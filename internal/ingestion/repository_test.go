package ingestion

import (
	"context"
	"errors"
	"regexp"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/google/uuid"

	"github.com/mtariq99/retrievion/internal/consts"
)

func newJobRepositoryMock(t *testing.T) (*Repository, sqlmock.Sqlmock) {
	t.Helper()

	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("create sqlmock database: %v", err)
	}
	t.Cleanup(func() {
		_ = db.Close()
	})

	return NewRepository(db), mock
}

func TestMarkJobProcessing(t *testing.T) {
	repo, mock := newJobRepositoryMock(t)
	jobID := uuid.New()

	mock.ExpectQuery(regexp.QuoteMeta(
		`UPDATE ingestion_jobs
		SET status = $2,
		    attempts = attempts + 1,
		    started_at = now(),
		    completed_at = NULL,
		    error = NULL
		WHERE id = $1 AND status = $3
		RETURNING attempts`,
	)).
		WithArgs(jobID, consts.JobStatusProcessing, consts.JobStatusQueued).
		WillReturnRows(sqlmock.NewRows([]string{"attempts"}).AddRow(1))

	attempt, err := repo.MarkJobProcessing(context.Background(), jobID)
	if err != nil {
		t.Fatalf("MarkJobProcessing returned error: %v", err)
	}
	if attempt != 1 {
		t.Fatalf("expected attempt 1, got %d", attempt)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet SQL expectations: %v", err)
	}
}

func TestMarkJobProcessingRejectsUnexpectedState(t *testing.T) {
	repo, mock := newJobRepositoryMock(t)
	jobID := uuid.New()

	mock.ExpectQuery(regexp.QuoteMeta(
		`UPDATE ingestion_jobs
		SET status = $2,
		    attempts = attempts + 1,
		    started_at = now(),
		    completed_at = NULL,
		    error = NULL
		WHERE id = $1 AND status = $3
		RETURNING attempts`,
	)).
		WithArgs(jobID, consts.JobStatusProcessing, consts.JobStatusQueued).
		WillReturnRows(sqlmock.NewRows([]string{"attempts"}))

	_, err := repo.MarkJobProcessing(context.Background(), jobID)
	if err == nil {
		t.Fatal("expected an error when job is not queued")
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet SQL expectations: %v", err)
	}
}

func TestMarkJobCompleted(t *testing.T) {
	repo, mock := newJobRepositoryMock(t)
	jobID := uuid.New()
	attempt := 1

	mock.ExpectExec(regexp.QuoteMeta(
		`UPDATE ingestion_jobs
		SET status = $2, completed_at = now(), error = NULL
		WHERE id = $1 AND status = $3 AND attempts = $4`,
	)).
		WithArgs(jobID, consts.JobStatusCompleted, consts.JobStatusProcessing, attempt).
		WillReturnResult(sqlmock.NewResult(0, 1))

	if err := repo.MarkJobCompleted(context.Background(), jobID, attempt); err != nil {
		t.Fatalf("MarkJobCompleted returned error: %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet SQL expectations: %v", err)
	}
}

func TestMarkJobFailedRejectsNilCause(t *testing.T) {
	repo, mock := newJobRepositoryMock(t)

	err := repo.MarkJobFailed(context.Background(), uuid.New(), 1, nil)
	if err == nil {
		t.Fatal("expected an error for nil failure cause")
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unexpected SQL operation: %v", err)
	}
}

func TestMarkJobFailedRejectsUnexpectedState(t *testing.T) {
	repo, mock := newJobRepositoryMock(t)
	jobID := uuid.New()
	attempt := 1
	cause := errors.New("temporary storage failure")

	mock.ExpectExec(regexp.QuoteMeta(
		`UPDATE ingestion_jobs
		SET status = $2, error = $3
		WHERE id = $1 AND status = $4 AND attempts = $5`,
	)).
		WithArgs(
			jobID,
			consts.JobStatusFailed,
			cause.Error(),
			consts.JobStatusProcessing,
			attempt,
		).
		WillReturnResult(sqlmock.NewResult(0, 0))

	err := repo.MarkJobFailed(context.Background(), jobID, attempt, cause)
	if err == nil {
		t.Fatal("expected an error when no matching processing attempt was updated")
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet SQL expectations: %v", err)
	}
}

func TestRecoverStaleJobs(t *testing.T) {
	repo, mock := newJobRepositoryMock(t)
	ctx := context.Background()

	mock.ExpectQuery(regexp.QuoteMeta(`
		WITH stale_jobs AS MATERIALIZED (
			SELECT id, document_id, document_version_id
			FROM ingestion_jobs
			WHERE status = $1
			  AND started_at IS NOT NULL
			  AND started_at < now() - ($2 * INTERVAL '1 second')
			ORDER BY started_at
			LIMIT $3
			FOR UPDATE SKIP LOCKED
		),
		requeued AS (
			UPDATE ingestion_jobs AS j
			SET status = $4,
			    started_at = NULL,
			    error = 'Recovered after processing timeout'
			FROM stale_jobs AS s
			WHERE j.id = s.id
			  AND j.status = $1
			RETURNING j.id, j.document_id, j.document_version_id
		),
		inserted_events AS (
			INSERT INTO ingestion_outbox (
				id,
				event_type,
				aggregate_id,
				payload
			)
			SELECT
				gen_random_uuid(),
				$5,
				r.id,
				jsonb_build_object(
					'job_id', r.id,
					'document_id', r.document_id,
					'version_id', r.document_version_id
				)
			FROM requeued AS r
			RETURNING id
		)
		SELECT count(*) FROM inserted_events
	`)).
		WithArgs(
			consts.JobStatusProcessing,
			60.0,
			10,
			consts.JobStatusQueued,
			"ingestion.job.created",
		).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(2))

	recovered, err := repo.RecoverStaleJobs(ctx, time.Minute, 10)
	if err != nil {
		t.Fatalf("RecoverStaleJobs returned error: %v", err)
	}
	if recovered != 2 {
		t.Fatalf("expected 2 recovered jobs, got %d", recovered)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet SQL expectations: %v", err)
	}
}

func TestRecoverStaleJobsRejectsInvalidArguments(t *testing.T) {
	repo, mock := newJobRepositoryMock(t)

	tests := []struct {
		name       string
		staleAfter time.Duration
		limit      int
	}{
		{name: "zero timeout", staleAfter: 0, limit: 10},
		{name: "negative timeout", staleAfter: -time.Second, limit: 10},
		{name: "zero limit", staleAfter: time.Minute, limit: 0},
		{name: "negative limit", staleAfter: time.Minute, limit: -1},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := repo.RecoverStaleJobs(
				context.Background(),
				tt.staleAfter,
				tt.limit,
			)
			if err == nil {
				t.Fatal("expected invalid arguments to be rejected")
			}
		})
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unexpected SQL operation: %v", err)
	}
}

func TestRecoverStaleJobsDatabaseError(t *testing.T) {
	repo, mock := newJobRepositoryMock(t)

	mock.ExpectQuery("WITH stale_jobs").
		WithArgs(
			consts.JobStatusProcessing,
			60.0,
			10,
			consts.JobStatusQueued,
			"ingestion.job.created",
		).
		WillReturnError(errors.New("database unavailable"))

	_, err := repo.RecoverStaleJobs(
		context.Background(),
		time.Minute,
		10,
	)
	if err == nil {
		t.Fatal("expected database error")
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet SQL expectations: %v", err)
	}
}
