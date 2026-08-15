package authorization

import (
	"strings"
	"testing"
)

func TestSafeAuditRecordBoundsMetadata(t *testing.T) {
	record := SafeAuditRecord(AuditRecord{
		Action:       "  organization.membership.read  ",
		ResourceType: " organization_membership ",
		RequestID:    strings.Repeat("r", 100),
		Metadata: map[string]string{
			" outcome_reason ": strings.Repeat("v", 300),
			"":                 "ignored",
		},
	})
	if record.Action != "organization.membership.read" || record.ResourceType != "organization_membership" {
		t.Fatalf("record=%#v", record)
	}
	if len(record.RequestID) != 64 || len(record.Metadata["outcome_reason"]) != 200 {
		t.Fatalf("unbounded safe fields=%#v", record)
	}
	if _, exists := record.Metadata[""]; exists {
		t.Fatal("empty metadata key was retained")
	}
}
