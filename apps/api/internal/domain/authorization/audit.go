package authorization

import (
	"context"
	"strings"
)

type AuditOutcome string

const (
	AuditSuccess AuditOutcome = "success"
	AuditDenied  AuditOutcome = "denied"
	AuditFailure AuditOutcome = "failure"
)

type AuditRecord struct {
	OrganizationID string
	ActorUserID    string
	Action         string
	ResourceType   string
	ResourceID     string
	Outcome        AuditOutcome
	RequestID      string
	Metadata       map[string]string
}

type Auditor interface {
	WriteAudit(context.Context, AuditRecord) error
}

func SafeAuditRecord(record AuditRecord) AuditRecord {
	record.Action = bounded(strings.TrimSpace(record.Action), 100)
	record.ResourceType = bounded(strings.TrimSpace(record.ResourceType), 100)
	record.RequestID = bounded(strings.TrimSpace(record.RequestID), 64)
	metadata := make(map[string]string)
	for key, value := range record.Metadata {
		key = bounded(strings.TrimSpace(key), 50)
		if key != "" && len(metadata) < 10 {
			metadata[key] = bounded(strings.TrimSpace(value), 200)
		}
	}
	record.Metadata = metadata
	return record
}

func bounded(value string, maximum int) string {
	if len(value) > maximum {
		return value[:maximum]
	}
	return value
}
