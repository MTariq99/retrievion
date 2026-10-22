package auth

import (
	"context"
	"database/sql"
	"errors"
	"strings"

	"github.com/google/uuid"

	"github.com/mtariq99/retrievion/internal/errs"
	"github.com/mtariq99/retrievion/models"
)

type Repository struct {
	db *sql.DB
}

func NewRepository(db *sql.DB) *Repository {
	return &Repository{db: db}
}

func (r *Repository) CreateUser(ctx context.Context, u *models.User) error {
	const query = `
		INSERT INTO users (email, password_hash, name)
		VALUES ($1, $2, $3)
		RETURNING id, created_at, updated_at
	`
	err := r.db.QueryRowContext(ctx, query, u.Email, u.PasswordHash, u.Name).
		Scan(&u.ID, &u.CreatedAt, &u.UpdatedAt)
	if err != nil {
		if isUniqueViolation(err) {
			return errs.ErrAlreadyExists
		}
		return errs.RepositoryError(err)
	}
	return nil
}

func (r *Repository) FindUserByEmail(ctx context.Context, email string) (*models.User, error) {
	const query = `
		SELECT u.id, u.email, u.password_hash, u.name, u.created_at, u.updated_at,
		       (
		           SELECT tm.tenant_id
		           FROM tenant_members tm
		           WHERE tm.user_id = u.id
		           AND (SELECT COUNT(*) FROM tenant_members WHERE user_id = u.id) = 1
		           LIMIT 1
		       ) AS tenant_id
		FROM users u
		WHERE u.email = $1
	`
	var u models.User
	var tenantID uuid.NullUUID
	err := r.db.QueryRowContext(ctx, query, email).Scan(
		&u.ID, &u.Email, &u.PasswordHash, &u.Name, &u.CreatedAt, &u.UpdatedAt, &tenantID,
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, errs.ErrNotFound
		}
		return nil, errs.RepositoryError(err)
	}
	if tenantID.Valid {
		u.TenantID = tenantID.UUID
	}
	return &u, nil
}

func (r *Repository) FindUserByID(ctx context.Context, id uuid.UUID) (*models.User, error) {
	const query = `
		SELECT u.id, u.email, u.password_hash, u.name, u.created_at, u.updated_at,
		       (
		           SELECT tm.tenant_id
		           FROM tenant_members tm
		           WHERE tm.user_id = u.id
		           AND (SELECT COUNT(*) FROM tenant_members WHERE user_id = u.id) = 1
		           LIMIT 1
		       ) AS tenant_id
		FROM users u
		WHERE u.id = $1
	`
	var u models.User
	var tenantID uuid.NullUUID
	err := r.db.QueryRowContext(ctx, query, id).Scan(
		&u.ID, &u.Email, &u.PasswordHash, &u.Name, &u.CreatedAt, &u.UpdatedAt, &tenantID,
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, errs.ErrNotFound
		}
		return nil, errs.RepositoryError(err)
	}
	if tenantID.Valid {
		u.TenantID = tenantID.UUID
	}
	return &u, nil
}

func (r *Repository) CreateTenant(ctx context.Context, t *models.Tenant) error {
	const query = `
		INSERT INTO tenants (name)
		VALUES ($1)
		RETURNING id, created_at, updated_at
	`
	err := r.db.QueryRowContext(ctx, query, t.Name).Scan(&t.ID, &t.CreatedAt, &t.UpdatedAt)
	if err != nil {
		return errs.RepositoryError(err)
	}
	return nil
}

func (r *Repository) CreateMembership(ctx context.Context, m *models.TenantMember) error {
	const query = `
		INSERT INTO tenant_members (tenant_id, user_id, role)
		VALUES ($1, $2, $3)
		RETURNING created_at
	`
	err := r.db.QueryRowContext(ctx, query, m.TenantID, m.UserID, m.Role).Scan(&m.CreatedAt)
	if err != nil {
		return errs.RepositoryError(err)
	}
	return nil
}

func (r *Repository) GetMembership(ctx context.Context, tenantID, userID uuid.UUID) (*models.TenantMember, error) {
	const query = `
		SELECT
			tm.tenant_id,
			t.name,
			tm.user_id,
			tm.role,
			tm.created_at
		FROM tenant_members tm
		INNER JOIN tenants t ON t.id = tm.tenant_id
		WHERE tm.tenant_id = $1 AND tm.user_id = $2
	`

	var m models.TenantMember

	err := r.db.QueryRowContext(ctx, query, tenantID, userID).Scan(
		&m.TenantID,
		&m.TenantName,
		&m.UserID,
		&m.Role,
		&m.CreatedAt,
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, errs.ErrNotFound
		}

		return nil, errs.RepositoryError(err)
	}

	return &m, nil
}

func (r *Repository) GetUserTenants(ctx context.Context, userID uuid.UUID) ([]models.Tenant, error) {
	const query = `
		SELECT t.id, t.name, t.created_at, t.updated_at
		FROM tenants t
		JOIN tenant_members tm ON tm.tenant_id = t.id
		WHERE tm.user_id = $1
		ORDER BY t.created_at ASC
	`
	rows, err := r.db.QueryContext(ctx, query, userID)
	if err != nil {
		return nil, errs.RepositoryError(err)
	}
	defer rows.Close()

	var tenants []models.Tenant
	for rows.Next() {
		var t models.Tenant
		if err := rows.Scan(&t.ID, &t.Name, &t.CreatedAt, &t.UpdatedAt); err != nil {
			return nil, errs.RepositoryError(err)
		}
		tenants = append(tenants, t)
	}
	if err := rows.Err(); err != nil {
		return nil, errs.RepositoryError(err)
	}
	return tenants, nil
}

func (r *Repository) GetUserMemberships(ctx context.Context, userID uuid.UUID) ([]models.TenantMember, error) {
	const query = `
		SELECT tenant_id, user_id, role, created_at
		FROM tenant_members
		WHERE user_id = $1
		ORDER BY created_at ASC
	`
	rows, err := r.db.QueryContext(ctx, query, userID)
	if err != nil {
		return nil, errs.RepositoryError(err)
	}
	defer rows.Close()

	var memberships []models.TenantMember
	for rows.Next() {
		var m models.TenantMember
		if err := rows.Scan(&m.TenantID, &m.UserID, &m.Role, &m.CreatedAt); err != nil {
			return nil, errs.RepositoryError(err)
		}
		memberships = append(memberships, m)
	}
	if err := rows.Err(); err != nil {
		return nil, errs.RepositoryError(err)
	}
	return memberships, nil
}

func (r *Repository) CreateUserWithTenant(ctx context.Context, u *models.User, tenantName string) (*models.Tenant, error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, errs.RepositoryError(err)
	}
	defer tx.Rollback()

	const userQuery = `
		INSERT INTO users (email, password_hash, name,role)
		VALUES ($1, $2, $3,$4)
		RETURNING id,role, created_at, updated_at
	`
	if err := tx.QueryRowContext(ctx, userQuery, u.Email, u.PasswordHash, u.Name, u.Role).
		Scan(&u.ID, &u.Role, &u.CreatedAt, &u.UpdatedAt); err != nil {
		if isUniqueViolation(err) {
			return nil, errs.ErrAlreadyExists
		}
		return nil, errs.RepositoryError(err)
	}

	tenant := &models.Tenant{Name: tenantName}
	const tenantQuery = `
		INSERT INTO tenants (name)
		VALUES ($1)
		RETURNING id, created_at, updated_at
	`
	if err := tx.QueryRowContext(ctx, tenantQuery, tenant.Name).
		Scan(&tenant.ID, &tenant.CreatedAt, &tenant.UpdatedAt); err != nil {
		return nil, errs.RepositoryError(err)
	}

	const memberQuery = `
		INSERT INTO tenant_members (tenant_id, user_id, role)
		VALUES ($1, $2, $3)
	`
	if _, err := tx.ExecContext(ctx, memberQuery, tenant.ID, u.ID, models.RoleOwner); err != nil {
		return nil, errs.RepositoryError(err)
	}

	if err := tx.Commit(); err != nil {
		return nil, errs.RepositoryError(err)
	}

	return tenant, nil
}

func isUniqueViolation(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	return strings.Contains(msg, "duplicate key value") || strings.Contains(msg, "SQLSTATE 23505")
}
