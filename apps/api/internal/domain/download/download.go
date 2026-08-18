// Package download issues short-lived, object-scoped download links for photos
// that a customer reached through an eligible public Event.
//
// Scope-binding design (conservative, no persisted grant token):
//   - Authorization derives only from the opaque public Event token, the Event
//     download policy, and result scope. The service re-resolves the Event scope
//     on every request; it never trusts organization membership for this flow.
//   - A photo is downloadable only if it belongs to the resolved Event
//     (organization + event) and is in a downloadable (READY) state.
//   - The signed access returned to the browser is a single short-lived,
//     object-scoped URL. It expires and cannot be replayed for any other object,
//     Event, or tenant. No separate grant/JWT token is minted, stored, or
//     returned, which removes an entire class of replay surface.
package download

import (
	"errors"
	"regexp"
	"strings"
	"time"
)

// MaxBulkHardCap bounds the configured per-request photo count regardless of
// configuration, so a single bulk request can never be unbounded.
const MaxBulkHardCap = 200

var (
	// ErrInvalidRequest indicates a malformed request (empty, oversized, or
	// syntactically invalid identifiers). It maps to 400.
	ErrInvalidRequest = errors.New("invalid download request")
	// ErrNotAvailable is the uniform, non-enumerating rejection used for unknown
	// or ineligible Events, disabled downloads, and out-of-scope or
	// non-downloadable photos. It maps to 404 so these cases are indistinguishable.
	ErrNotAvailable = errors.New("download not available")
	// ErrUnavailable indicates a transient dependency failure. It maps to 503.
	ErrUnavailable = errors.New("download temporarily unavailable")
)

type Decision string

const (
	DecisionAllowed Decision = "allowed"
	DecisionDenied  Decision = "denied"
)

type Kind string

const (
	KindSingle Kind = "single"
	KindBulk   Kind = "bulk"
)

// Denial codes are safe, low-cardinality labels for audit records. They never
// contain identifiers, URLs, object paths, or tokens.
const (
	DenialDownloadsDisabled = "downloads_disabled"
	DenialScopeViolation    = "scope_violation"
)

// Scope is the trusted internal scope resolved from an eligible public token.
type Scope struct {
	OrganizationID   string
	EventID          string
	DownloadsEnabled bool
}

// DownloadableObject is the storage projection needed to sign one download. It
// intentionally carries only what the signer needs and never leaves the server.
type DownloadableObject struct {
	ObjectKey   string
	ContentType string
}

// Grant is a single signed download link returned to the browser.
type Grant struct {
	PhotoID   string
	URL       string
	ExpiresAt time.Time
}

// AuditEntry is a safe, decision-level audit record for the download flow. It
// excludes signed URLs, object paths, tokens, embeddings, and image bytes.
type AuditEntry struct {
	OrganizationID string
	EventID        string
	PhotoID        string
	Kind           Kind
	Decision       Decision
	DenialCode     string
}

var uuidPattern = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`)

// NormalizePhotoIDs validates and de-duplicates requested identifiers. It
// enforces the bounded count and rejects syntactically invalid identifiers
// before any scope resolution or storage access.
func NormalizePhotoIDs(photoIDs []string, maxBulk int) ([]string, error) {
	if maxBulk <= 0 {
		return nil, ErrInvalidRequest
	}
	if len(photoIDs) == 0 || len(photoIDs) > maxBulk {
		return nil, ErrInvalidRequest
	}
	seen := make(map[string]struct{}, len(photoIDs))
	normalized := make([]string, 0, len(photoIDs))
	for _, id := range photoIDs {
		id = strings.TrimSpace(id)
		if !uuidPattern.MatchString(id) {
			return nil, ErrInvalidRequest
		}
		id = strings.ToLower(id)
		if _, duplicate := seen[id]; duplicate {
			continue
		}
		seen[id] = struct{}{}
		normalized = append(normalized, id)
	}
	if len(normalized) == 0 {
		return nil, ErrInvalidRequest
	}
	return normalized, nil
}

// KindFor classifies a request by its resolved identifier count.
func KindFor(count int) Kind {
	if count > 1 {
		return KindBulk
	}
	return KindSingle
}

// ContentDisposition builds a safe attachment disposition using the opaque
// photo identifier and a whitelisted extension derived from the stored content
// type. It never uses client-supplied filenames.
func ContentDisposition(photoID, contentType string) string {
	extension := "bin"
	switch strings.ToLower(strings.TrimSpace(contentType)) {
	case "image/jpeg":
		extension = "jpg"
	case "image/png":
		extension = "png"
	case "image/webp":
		extension = "webp"
	}
	return `attachment; filename="` + photoID + "." + extension + `"`
}
