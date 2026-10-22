package main

import (
	"github.com/mtariq99/retrievion/internal/ingestion"
	"github.com/mtariq99/retrievion/internal/notifications"
)

type Workers struct {
	Ingestion   *ingestion.Worker
	Invitations *notifications.InvitationWorker
}
