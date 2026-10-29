package main

import (
	"github.com/mtariq99/retrievion/internal/document"
	"github.com/mtariq99/retrievion/internal/ingestion"
	"github.com/mtariq99/retrievion/internal/notifications"
	"github.com/mtariq99/retrievion/internal/tenants"
)

type Workers struct {
	Ingestion       *ingestion.Worker
	Invitations     *notifications.InvitationWorker
	OutboxPublisher *document.OutboxPublisher
	Recovery        *ingestion.RecoveryWorker
	OutboxRelay     *tenants.OutboxRelay
}
