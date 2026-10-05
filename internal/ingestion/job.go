package ingestion

import (
	"encoding/json"
	"fmt"

	"github.com/google/uuid"

	"github.com/mtariq99/retrievion/models"
)

// ParseJob decodes a raw queue message body (see storage.MarshalIngestionJob)
// into a models.Job with parsed UUIDs.
func ParseJob(body []byte) (models.Job, error) {
	var p models.JobPayload
	if err := json.Unmarshal(body, &p); err != nil {
		return models.Job{}, fmt.Errorf("unmarshal ingestion job: %w", err)
	}

	jobID, err := uuid.Parse(p.JobID)
	if err != nil {
		return models.Job{}, fmt.Errorf("parse job_id: %w", err)
	}
	documentID, err := uuid.Parse(p.DocumentID)
	if err != nil {
		return models.Job{}, fmt.Errorf("parse document_id: %w", err)
	}
	versionID, err := uuid.Parse(p.DocumentVersionID)
	if err != nil {
		return models.Job{}, fmt.Errorf("parse version_id: %w", err)
	}

	return models.Job{JobID: jobID, DocumentID: documentID, DocumentVersionID: versionID}, nil
}
