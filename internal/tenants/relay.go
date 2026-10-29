package tenants

import (
	"context"
	"database/sql"
	"log"
	"time"

	"github.com/mtariq99/retrievion/models"
)

type OutboxRelay struct {
	db        *sql.DB
	publisher InvitationPublisher
	repo      *TenantRepository
	cfg       *models.Config
	batch     int
}

func NewOutboxRelay(config *models.Config, db *sql.DB, repo *TenantRepository, publisher InvitationPublisher) *OutboxRelay {
	batch := 0
	if config == nil {
		batch = 50
	} else {
		batch = config.App.Batch
	}
	return &OutboxRelay{
		db:        db,
		publisher: publisher,
		repo:      repo,
		cfg:       config,
		batch:     batch,
	}
}

func (r *OutboxRelay) Run(ctx context.Context) {
	t := time.NewTicker(2 * time.Second)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			if err := r.drain(ctx); err != nil {
				log.Printf("outbox drain: %v", err)
			}
		}
	}
}

func (r *OutboxRelay) drain(ctx context.Context) error {
	tx, err := r.repo.BeginOutboxTx(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	batch, err := tx.Claim(ctx, r.batch)
	if err != nil {
		return err
	}

	for _, item := range batch {
		if err := r.publisher.PublishInvitation(ctx, item.ID, item.Payload); err != nil {
			secs := 1 << min(item.Attempts, 8)

			if err = tx.RecordFailure(ctx, item.ID, err.Error(), time.Duration(secs)*time.Second); err != nil {
				return err
			}

			continue
		}

		if err := tx.Delete(ctx, item.ID); err != nil {
			return err
		}
	}

	return tx.Commit()
}
