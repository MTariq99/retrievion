package main

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/mtariq99/retrievion/internal/config"
	"github.com/mtariq99/retrievion/internal/storage"
	"golang.org/x/crypto/bcrypt"
)

const (
	defaultTenantName = "Retrievion Evaluation"
	defaultUserEmail  = "eval-bot@retrievion.local"
	defaultUserName   = "Retrievion Evaluation Runner"
	migrationTable    = "schema_migrations"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "evaluation setup failed: %v\n", err)
		os.Exit(1)
	}
}

func run() error {
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("load configuration: %w", err)
	}

	logger, err := storage.BuildLogger(cfg.App.Env)
	if err != nil {
		return fmt.Errorf("initialize logger: %w", err)
	}
	defer func() { _ = logger.Sync() }()

	db, err := storage.InitDB(cfg, logger)
	if err != nil {
		return fmt.Errorf("connect to database: %w", err)
	}
	defer db.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()

	if err := checkMigrationReadiness(ctx, db, cfg.Database.MigrationPath); err != nil {
		return err
	}

	tenantName := envOrDefault("EVAL_TENANT_NAME", defaultTenantName)
	userEmail := strings.ToLower(envOrDefault("EVAL_USER_EMAIL", defaultUserEmail))
	userName := envOrDefault("EVAL_USER_NAME", defaultUserName)

	tenantID, userID, err := bootstrapIdentity(ctx, db, tenantName, userEmail, userName)
	if err != nil {
		return err
	}
	if err := ensureEvalFixtures(ctx, db, cfg, logger, tenantID); err != nil {
		return fmt.Errorf("prepare evaluation fixtures: %w", err)
	}
	fmt.Println("Evaluation identity is ready.")
	fmt.Printf("Tenant ID: %s\n", tenantID)
	fmt.Printf("User ID:   %s\n", userID)
	fmt.Println("No migrations were applied by eval-setup.")
	return nil
}

func checkMigrationReadiness(ctx context.Context, db *sql.DB, migrationPath string) error {
	var version int64
	var dirty bool

	err := db.QueryRowContext(ctx, "SELECT version, dirty FROM "+migrationTable+" LIMIT 1").Scan(&version, &dirty)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return fmt.Errorf("database migrations have not been applied; run `make run-api` first, then retry `make eval-setup`")
		}
		if strings.Contains(strings.ToLower(err.Error()), "schema_migrations") && (strings.Contains(strings.ToLower(err.Error()), "does not exist") || strings.Contains(strings.ToLower(err.Error()), "undefined table")) {
			return fmt.Errorf("database migrations have not been applied; run `make run-api` first, then retry `make eval-setup`")
		}
		return fmt.Errorf("read migration state: %w", err)
	}

	if dirty {
		return fmt.Errorf("database migration state is dirty at version %d; resolve the failed migration before running `make eval-setup`", version)
	}

	latest, err := latestMigrationVersion(migrationPath)
	if err != nil {
		return fmt.Errorf("inspect local migration files: %w", err)
	}
	if version == 0 || version < latest {
		return fmt.Errorf("database migrations are behind the repository (database version %d, latest local version %d); run `make run-api` first, then retry `make eval-setup`", version, latest)
	}

	return nil
}

func latestMigrationVersion(migrationPath string) (int64, error) {
	entries, err := os.ReadDir(migrationPath)
	if err != nil {
		return 0, fmt.Errorf("read migration directory %q: %w", migrationPath, err)
	}

	var latest int64
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || !strings.HasSuffix(name, ".up.sql") {
			continue
		}

		prefix := strings.SplitN(name, "_", 2)[0]
		version, err := strconv.ParseInt(prefix, 10, 64)
		if err != nil {
			return 0, fmt.Errorf("invalid migration filename %q: %w", name, err)
		}
		if version > latest {
			latest = version
		}
	}
	if latest == 0 {
		return 0, fmt.Errorf("no .up.sql migration files found in %q", migrationPath)
	}
	return latest, nil
}

func bootstrapIdentity(ctx context.Context, db *sql.DB, tenantName, userEmail, userName string) (string, string, error) {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return "", "", fmt.Errorf("begin identity transaction: %w", err)
	}
	defer tx.Rollback()

	if _, err := tx.ExecContext(ctx, `SELECT pg_advisory_xact_lock(hashtext('retrievion-eval-setup'))`); err != nil {
		return "", "", fmt.Errorf("lock evaluation setup: %w", err)
	}

	tenantID, err := findOrCreateTenant(ctx, tx, tenantName)
	if err != nil {
		return "", "", err
	}

	userID, err := findOrCreateUser(ctx, tx, userEmail, userName)
	if err != nil {
		return "", "", err
	}

	var membershipRole string
	err = tx.QueryRowContext(ctx, `
		SELECT role
		FROM tenant_members
		WHERE tenant_id = $1 AND user_id = $2
	`, tenantID, userID).Scan(&membershipRole)

	switch {
	case errors.Is(err, sql.ErrNoRows):
		_, err = tx.ExecContext(ctx, `
			INSERT INTO tenant_members (tenant_id, user_id, role)
			VALUES ($1, $2, 'owner')
		`, tenantID, userID)
		if err != nil {
			return "", "", fmt.Errorf("create evaluation membership: %w", err)
		}
	case err != nil:
		return "", "", fmt.Errorf("validate evaluation membership: %w", err)
	case strings.TrimSpace(membershipRole) == "":
		return "", "", fmt.Errorf("evaluation membership has an empty role")
	}
	if err := tx.Commit(); err != nil {
		return "", "", fmt.Errorf("commit evaluation identity: %w", err)
	}
	return tenantID, userID, nil
}

func findOrCreateTenant(ctx context.Context, tx *sql.Tx, name string) (string, error) {
	var id string
	err := tx.QueryRowContext(ctx, `
		SELECT id::text FROM tenants WHERE name = $1
	`, name).Scan(&id)

	if err == nil {
		var count int
		if err := tx.QueryRowContext(
			ctx,
			`SELECT COUNT(*) FROM tenants WHERE name = $1`, name,
		).Scan(&count); err != nil {
			return "", fmt.Errorf("validate evaluation tenant: %w", err)
		}
		if count != 1 {
			return "", fmt.Errorf(
				"found multiple tenants named %q; resolve duplicates or set EVAL_TENANT_NAME to a unique name",
				name,
			)
		}
		return id, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return "", fmt.Errorf("look up evaluation tenant: %w", err)
	}

	err = tx.QueryRowContext(ctx, `
		INSERT INTO tenants (name)
		VALUES ($1)
		RETURNING id::text
	`, name).Scan(&id)
	if err != nil {
		return "", fmt.Errorf("create evaluation tenant: %w", err)
	}
	return id, nil
}

func findOrCreateUser(ctx context.Context, tx *sql.Tx, email, name string) (string, error) {
	var id string
	var deletedAt sql.NullTime

	err := tx.QueryRowContext(ctx, `
		SELECT id::text, deleted_at
		FROM users
		WHERE lower(email) = lower($1)
	`, email).Scan(&id, &deletedAt)

	if err == nil {
		if deletedAt.Valid {
			return "", fmt.Errorf(
				"evaluation user %q is soft-deleted; restore it or configure a different EVAL_USER_EMAIL",
				email,
			)
		}
		return id, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return "", fmt.Errorf("look up evaluation user: %w", err)
	}

	randomPassword := make([]byte, 32)
	if _, err := rand.Read(randomPassword); err != nil {
		return "", fmt.Errorf("generate evaluation credential: %w", err)
	}
	password := hex.EncodeToString(randomPassword)
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return "", fmt.Errorf("hash evaluation credential: %w", err)
	}

	err = tx.QueryRowContext(ctx, `
		INSERT INTO users (email, password_hash, name, role)
		VALUES ($1, $2, $3, 'user')
		RETURNING id::text
	`, email, string(hash), name).Scan(&id)
	if err != nil {
		return "", fmt.Errorf("create evaluation user: %w", err)
	}
	return id, nil
}

func envOrDefault(key, fallback string) string {
	if value := strings.TrimSpace(os.Getenv(key)); value != "" {
		return value
	}
	return fallback
}

var _ = filepath.Clean
