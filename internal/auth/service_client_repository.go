package auth

import (
	"context"
	"database/sql"

	"github.com/google/uuid"
	"github.com/mtariq99/retrievion/models"
)

type ServiceClientRepository struct {
	db *sql.DB
}

func NewClientServiceRepository(db *sql.DB) *ServiceClientRepository {
	return &ServiceClientRepository{db: db}
}

func (r *ServiceClientRepository) GetByClientID(ctx context.Context, clientID uuid.UUID) (*models.ServiceClient, error) {
	var sc models.ServiceClient
	err := r.db.QueryRowContext(
		ctx,
		`SELECT id, client_id, secret_hash, tenant_id, name, scopes, revoked_at, created_at
		 FROM service_clients WHERE client_id = $1`, clientID,
	).Scan(&sc.ID, &sc.ClientID, &sc.SecretHash, &sc.TenantID, &sc.Name, &sc.Scopes, &sc.RevokedAt, &sc.CreatedAt)
	if err != nil {
		return nil, err
	}
	return &sc, nil
}

func (r *ServiceClientRepository) Create(ctx context.Context, sc *models.ServiceClient) error {
	_, err := r.db.ExecContext(
		ctx,
		`INSERT INTO service_clients (client_id, secret_hash, tenant_id, name, scopes)
		 VALUES ($1, $2, $3, $4, $5)`,
		sc.ClientID, sc.SecretHash, sc.TenantID, sc.Name, sc.Scopes,
	)
	return err
}
