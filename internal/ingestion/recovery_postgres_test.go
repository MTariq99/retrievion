package ingestion

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	_ "github.com/lib/pq"

	"github.com/mtariq99/retrievion/internal/consts"
)

func openRecoveryTestDB(t *testing.T) (*sql.DB, context.Context) {
	t.Helper()

	dsn := os.Getenv("RETRIEVION_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("set RETRIEVION_TEST_DATABASE_URL to run PostgreSQL integration tests")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	t.Cleanup(cancel)

	db, err := sql.Open("postgres", dsn)
	if err != nil {
		t.Fatalf("open PostgreSQL: %v", err)
	}
	t.Cleanup(func() {
		if err := db.Close(); err != nil {
			t.Errorf("close PostgreSQL: %v", err)
		}
	})

	if err := db.PingContext(ctx); err != nil {
		t.Fatalf("ping PostgreSQL: %v", err)
	}

	return db, ctx
}

func createRecoveryFixture(t *testing.T, db *sql.DB, ctx context.Context) (
	tenantID, documentID, versionID uuid.UUID,
) {
	t.Helper()

	tenantID = uuid.New()
	documentID = uuid.New()
	versionID = uuid.New()

	if _, err := db.ExecContext(
		ctx,
		`INSERT INTO tenants (id, name) VALUES ($1, $2)`,
		tenantID, "recovery-test-"+tenantID.String(),
	); err != nil {
		t.Fatalf("create tenant: %v", err)
	}

	t.Cleanup(func() {
		cleanupCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		if _, err := db.ExecContext(
			cleanupCtx,
			`DELETE FROM tenants WHERE id = $1`, tenantID,
		); err != nil {
			t.Errorf("delete recovery-test tenant: %v", err)
		}
	})

	if _, err := db.ExecContext(
		ctx,
		`INSERT INTO documents (id, tenant_id, name, mime_type, status)
		 VALUES ($1, $2, $3, $4, 'pending')`,
		documentID, tenantID, "recovery-test.txt", "text/plain",
	); err != nil {
		t.Fatalf("create document: %v", err)
	}

	if _, err := db.ExecContext(
		ctx,
		`INSERT INTO document_versions
		 (id, document_id, version, content_hash, storage_key, status)
		 VALUES ($1, $2, 1, $3, $4, 'pending')`,
		versionID, documentID,
		"recovery-"+versionID.String(),
		"recovery-tests/"+versionID.String(),
	); err != nil {
		t.Fatalf("create document version: %v", err)
	}

	return tenantID, documentID, versionID
}

func createRecoveryJob(
	t *testing.T,
	db *sql.DB,
	ctx context.Context,
	tenantID, documentID, versionID uuid.UUID,
	age time.Duration,
) uuid.UUID {
	t.Helper()

	jobID := uuid.New()

	_, err := db.ExecContext(
		ctx,
		`INSERT INTO ingestion_jobs
		 (id, tenant_id, document_id, document_version_id,
		  status, attempts, started_at)
		 VALUES ($1, $2, $3, $4, $5, 1,
		         now() - ($6 * INTERVAL '1 second'))`,
		jobID, tenantID, documentID, versionID,
		consts.JobStatusProcessing, age.Seconds(),
	)
	if err != nil {
		t.Fatalf("create processing job: %v", err)
	}

	return jobID
}

func TestRecoverStaleJobsPostgres(t *testing.T) {
	db, ctx := openRecoveryTestDB(t)
	tenantID, documentID, versionID := createRecoveryFixture(t, db, ctx)
	repo := NewRepository(db)

	staleID := createRecoveryJob(
		t, db, ctx, tenantID, documentID, versionID, 10*time.Minute,
	)
	freshID := createRecoveryJob(
		t, db, ctx, tenantID, documentID, versionID, 30*time.Second,
	)

	recovered, err := repo.RecoverStaleJobs(ctx, 5*time.Minute, 25)
	if err != nil {
		t.Fatalf("recover stale jobs: %v", err)
	}
	if recovered != 1 {
		t.Fatalf("recovered = %d; want 1", recovered)
	}

	var status string
	var startedAt sql.NullTime
	var attempts int
	var jobError sql.NullString

	err = db.QueryRowContext(
		ctx,
		`SELECT status, started_at, attempts, error
		 FROM ingestion_jobs WHERE id = $1`,
		staleID,
	).Scan(&status, &startedAt, &attempts, &jobError)
	if err != nil {
		t.Fatalf("read recovered job: %v", err)
	}

	if status != string(consts.JobStatusQueued) {
		t.Fatalf("recovered status = %q; want queued", status)
	}
	if startedAt.Valid {
		t.Fatal("recovered job still has started_at")
	}
	if attempts != 1 {
		t.Fatalf("attempts = %d; recovery must not increment attempts", attempts)
	}
	if !jobError.Valid || jobError.String != "Recovered after processing timeout" {
		t.Fatalf("unexpected recovery error: %v", jobError)
	}

	var eventCount int
	var eventType string
	var aggregateID uuid.UUID
	var payloadJobID, payloadDocumentID, payloadVersionID uuid.UUID

	err = db.QueryRowContext(
		ctx,
		`SELECT count(*), min(event_type), min(aggregate_id::text)::uuid,
		        min(payload->>'job_id')::uuid,
		        min(payload->>'document_id')::uuid,
		        min(payload->>'version_id')::uuid
		 FROM ingestion_outbox WHERE aggregate_id = $1`,
		staleID,
	).Scan(
		&eventCount, &eventType, &aggregateID,
		&payloadJobID, &payloadDocumentID, &payloadVersionID,
	)
	if err != nil {
		t.Fatalf("read recovery outbox event: %v", err)
	}
	if eventCount != 1 {
		t.Fatalf("outbox event count = %d; want 1", eventCount)
	}
	if eventType != "ingestion.job.created" {
		t.Fatalf("event type = %q", eventType)
	}
	if aggregateID != staleID || payloadJobID != staleID ||
		payloadDocumentID != documentID || payloadVersionID != versionID {
		t.Fatal("outbox event does not identify the recovered job and its document version")
	}

	err = db.QueryRowContext(
		ctx,
		`SELECT status, started_at FROM ingestion_jobs WHERE id = $1`,
		freshID,
	).Scan(&status, &startedAt)
	if err != nil {
		t.Fatalf("read fresh job: %v", err)
	}
	if status != string(consts.JobStatusProcessing) || !startedAt.Valid {
		t.Fatalf("fresh job changed unexpectedly: status=%q started_at=%v", status, startedAt)
	}

	// A second recovery must not create a duplicate event for this job.
	recovered, err = repo.RecoverStaleJobs(ctx, 5*time.Minute, 25)
	if err != nil {
		t.Fatalf("repeat recovery: %v", err)
	}
	if recovered != 0 {
		t.Fatalf("second recovery recovered %d jobs; want 0", recovered)
	}
}

func TestRecoverStaleJobsPostgresConcurrent(t *testing.T) {
	db, ctx := openRecoveryTestDB(t)
	tenantID, documentID, versionID := createRecoveryFixture(t, db, ctx)
	repo := NewRepository(db)

	const jobCount = 8
	jobIDs := make([]uuid.UUID, 0, jobCount)
	for i := 0; i < jobCount; i++ {
		jobIDs = append(jobIDs, createRecoveryJob(
			t, db, ctx, tenantID, documentID, versionID, 10*time.Minute,
		))
	}

	start := make(chan struct{})
	results := make(chan int, 2)
	errs := make(chan error, 2)

	var wg sync.WaitGroup
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			count, err := repo.RecoverStaleJobs(ctx, 5*time.Minute, jobCount)
			if err != nil {
				errs <- err
				return
			}
			results <- count
		}()
	}

	close(start)
	wg.Wait()
	close(results)
	close(errs)

	for err := range errs {
		t.Errorf("concurrent recovery failed: %v", err)
	}

	totalRecovered := 0
	for count := range results {
		totalRecovered += count
	}
	if totalRecovered != jobCount {
		t.Fatalf("total recovered = %d; want %d", totalRecovered, jobCount)
	}

	for _, jobID := range jobIDs {
		var status string
		var eventCount int

		if err := db.QueryRowContext(
			ctx,
			`SELECT status FROM ingestion_jobs WHERE id = $1`,
			jobID,
		).Scan(&status); err != nil {
			t.Fatalf("read job %s: %v", jobID, err)
		}
		if status != string(consts.JobStatusQueued) {
			t.Errorf("job %s status = %q; want queued", jobID, status)
		}

		if err := db.QueryRowContext(
			ctx,
			`SELECT count(*) FROM ingestion_outbox WHERE aggregate_id = $1`,
			jobID,
		).Scan(&eventCount); err != nil {
			t.Fatalf("count events for job %s: %v", jobID, err)
		}
		if eventCount != 1 {
			t.Errorf("job %s has %d recovery events; want 1", jobID, eventCount)
		}
	}
}

func TestRecoverStaleJobsPostgresOutboxFailureRollsBack(t *testing.T) {
	db, ctx := openRecoveryTestDB(t)
	tenantID, documentID, versionID := createRecoveryFixture(t, db, ctx)
	repo := NewRepository(db)

	jobID := createRecoveryJob(
		t, db, ctx, tenantID, documentID, versionID, 10*time.Minute,
	)

	// Fail outbox insertion only for this fixture's job.
	functionName := "test_recovery_outbox_failure_" + jobID.String()[:8]
	triggerName := "test_recovery_outbox_trigger_" + jobID.String()[:8]

	_, err := db.ExecContext(ctx, fmt.Sprintf(`
		CREATE FUNCTION %s() RETURNS trigger AS $$
		BEGIN
			IF NEW.aggregate_id = '%s'::uuid THEN
				RAISE EXCEPTION 'intentional recovery integration-test failure';
			END IF;
			RETURN NEW;
		END;
		$$ LANGUAGE plpgsql
	`, functionName, jobID))
	if err != nil {
		t.Fatalf("create test trigger function: %v", err)
	}

	_, err = db.ExecContext(ctx, fmt.Sprintf(`
		CREATE TRIGGER %s
		BEFORE INSERT ON ingestion_outbox
		FOR EACH ROW EXECUTE FUNCTION %s()
	`, triggerName, functionName))
	if err != nil {
		_, _ = db.ExecContext(ctx, fmt.Sprintf("DROP FUNCTION %s()", functionName))
		t.Fatalf("create test trigger: %v", err)
	}

	t.Cleanup(func() {
		_, _ = db.ExecContext(context.Background(),
			fmt.Sprintf("DROP TRIGGER IF EXISTS %s ON ingestion_outbox", triggerName))
		_, _ = db.ExecContext(context.Background(),
			fmt.Sprintf("DROP FUNCTION IF EXISTS %s()", functionName))
	})

	_, err = repo.RecoverStaleJobs(ctx, 5*time.Minute, 25)
	if err == nil {
		t.Fatal("expected recovery to fail when outbox insertion fails")
	}

	var status string
	var startedAt sql.NullTime
	if err := db.QueryRowContext(
		ctx,
		`SELECT status, started_at FROM ingestion_jobs WHERE id = $1`,
		jobID,
	).Scan(&status, &startedAt); err != nil {
		t.Fatalf("read job after rollback: %v", err)
	}
	if status != string(consts.JobStatusProcessing) || !startedAt.Valid {
		t.Fatalf("job update was not rolled back: status=%q started_at=%v", status, startedAt)
	}

	var eventCount int
	if err := db.QueryRowContext(
		ctx,
		`SELECT count(*) FROM ingestion_outbox WHERE aggregate_id = $1`,
		jobID,
	).Scan(&eventCount); err != nil {
		t.Fatalf("count outbox events: %v", err)
	}
	if eventCount != 0 {
		t.Fatalf("outbox event count = %d after failed recovery; want 0", eventCount)
	}
}

func TestActivateVersionRejectsRecoveredStaleAttemptPostgres(t *testing.T) {
	db, ctx := openRecoveryTestDB(t)
	tenantID, documentID, versionID := createRecoveryFixture(t, db, ctx)
	repo := NewRepository(db)

	jobID := createRecoveryJob(
		t, db, ctx, tenantID, documentID, versionID, 10*time.Minute,
	)

	// Recovery invalidates the original processing attempt by requeueing the job.
	recovered, err := repo.RecoverStaleJobs(ctx, 5*time.Minute, 25)
	if err != nil {
		t.Fatalf("recover stale job: %v", err)
	}
	if recovered != 1 {
		t.Fatalf("recovered = %d; want 1", recovered)
	}

	// The old worker must not activate a version after its attempt was reclaimed.
	err = repo.ActivateVersion(ctx, jobID, documentID, versionID, 1)
	if err == nil {
		t.Fatal("expected stale activation attempt to be rejected")
	}

	var jobStatus string
	var attempts int
	if err := db.QueryRowContext(
		ctx,
		`SELECT status, attempts FROM ingestion_jobs WHERE id = $1`,
		jobID,
	).Scan(&jobStatus, &attempts); err != nil {
		t.Fatalf("read job after stale activation: %v", err)
	}

	if jobStatus != string(consts.JobStatusQueued) {
		t.Fatalf("job status = %q; want queued", jobStatus)
	}
	if attempts != 1 {
		t.Fatalf("attempts = %d; want 1", attempts)
	}

	var versionStatus string
	if err := db.QueryRowContext(
		ctx,
		`SELECT status FROM document_versions WHERE id = $1`,
		versionID,
	).Scan(&versionStatus); err != nil {
		t.Fatalf("read version after stale activation: %v", err)
	}

	if versionStatus != string(consts.DocumentStatusPending) {
		t.Fatalf(
			"version status = %q; want pending after rejected stale activation",
			versionStatus,
		)
	}
}
