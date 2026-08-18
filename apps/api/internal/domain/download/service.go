package download

import (
	"context"
	"time"
)

// ScopeResolver resolves the trusted Event scope and download policy from an
// opaque public token. Implementations must apply the same eligibility rules as
// the public Event surface (public, active, unexpired) so unknown and ineligible
// Events are indistinguishable.
type ScopeResolver interface {
	FindPublicDownloadScope(context.Context, string, time.Time) (Scope, error)
}

// PhotoResolver returns the storage projection for a photo only when it belongs
// to the given organization and Event and is in a downloadable (READY) state.
// It must return an error for any other case so result scope cannot be bypassed.
type PhotoResolver interface {
	FindDownloadable(context.Context, string, string, string) (DownloadableObject, error)
}

// ObjectURLSigner signs a short-lived GET for exactly one stored object. The
// returned URL must be scoped to that object and expire at the returned time.
type ObjectURLSigner interface {
	PresignGet(context.Context, string, string, time.Duration) (string, time.Time, error)
}

// Recorder persists safe, decision-level audit records. Implementations must not
// receive or store signed URLs, object paths, tokens, or biometric data.
type Recorder interface {
	Record(context.Context, AuditEntry) error
}

// Result carries the outcome of an issuance attempt. It always reports the
// resolved scope, kind, and decision when a scope was resolved, so the transport
// layer can emit request-correlated audit records even on denial.
type Result struct {
	OrganizationID string
	EventID        string
	Kind           Kind
	Decision       Decision
	DenialCode     string
	Grants         []Grant
}

type Service struct {
	scopes   ScopeResolver
	photos   PhotoResolver
	signer   ObjectURLSigner
	recorder Recorder
	urlTTL   time.Duration
	maxBulk  int
	now      func() time.Time
}

func NewService(scopes ScopeResolver, photos PhotoResolver, signer ObjectURLSigner, recorder Recorder, urlTTL time.Duration, maxBulk int) (*Service, error) {
	if scopes == nil || photos == nil || signer == nil || recorder == nil {
		return nil, ErrInvalidRequest
	}
	if urlTTL <= 0 || maxBulk <= 0 || maxBulk > MaxBulkHardCap {
		return nil, ErrInvalidRequest
	}
	return &Service{scopes: scopes, photos: photos, signer: signer, recorder: recorder, urlTTL: urlTTL, maxBulk: maxBulk, now: time.Now}, nil
}

// Issue authorizes and signs downloads for the requested photos. It fails closed:
// any policy or scope problem yields a uniform ErrNotAvailable, and no link is
// returned unless every requested photo is in scope and downloadable.
func (s *Service) Issue(ctx context.Context, token string, photoIDs []string) (Result, error) {
	ids, err := NormalizePhotoIDs(photoIDs, s.maxBulk)
	if err != nil {
		return Result{}, ErrInvalidRequest
	}
	kind := KindFor(len(ids))

	scope, err := s.scopes.FindPublicDownloadScope(ctx, token, s.now().UTC())
	if err != nil {
		// No trusted scope resolved: unknown or ineligible Event. Nothing safe to
		// audit against a tenant; reject uniformly.
		return Result{Kind: kind, Decision: DecisionDenied}, ErrNotAvailable
	}

	if !scope.DownloadsEnabled {
		result := Result{OrganizationID: scope.OrganizationID, EventID: scope.EventID, Kind: kind, Decision: DecisionDenied, DenialCode: DenialDownloadsDisabled}
		s.record(ctx, AuditEntry{OrganizationID: scope.OrganizationID, EventID: scope.EventID, PhotoID: singlePhotoID(ids, kind), Kind: kind, Decision: DecisionDenied, DenialCode: DenialDownloadsDisabled})
		return result, ErrNotAvailable
	}

	grants := make([]Grant, 0, len(ids))
	for _, id := range ids {
		object, err := s.photos.FindDownloadable(ctx, scope.OrganizationID, scope.EventID, id)
		if err != nil {
			// Photo is not part of this Event or is not downloadable. Reject the
			// whole request uniformly and record the scope violation.
			result := Result{OrganizationID: scope.OrganizationID, EventID: scope.EventID, Kind: kind, Decision: DecisionDenied, DenialCode: DenialScopeViolation}
			s.record(ctx, AuditEntry{OrganizationID: scope.OrganizationID, EventID: scope.EventID, PhotoID: id, Kind: kind, Decision: DecisionDenied, DenialCode: DenialScopeViolation})
			return result, ErrNotAvailable
		}
		url, expiresAt, err := s.signer.PresignGet(ctx, object.ObjectKey, ContentDisposition(id, object.ContentType), s.urlTTL)
		if err != nil {
			return Result{OrganizationID: scope.OrganizationID, EventID: scope.EventID, Kind: kind, Decision: DecisionDenied}, ErrUnavailable
		}
		grants = append(grants, Grant{PhotoID: id, URL: url, ExpiresAt: expiresAt})
	}

	for _, grant := range grants {
		s.record(ctx, AuditEntry{OrganizationID: scope.OrganizationID, EventID: scope.EventID, PhotoID: grant.PhotoID, Kind: kind, Decision: DecisionAllowed})
	}
	return Result{OrganizationID: scope.OrganizationID, EventID: scope.EventID, Kind: kind, Decision: DecisionAllowed, Grants: grants}, nil
}

// singlePhotoID returns the sole requested identifier for a single-kind request,
// which lets a request-level denial (for example downloads disabled) still record
// a photo-scoped audit row. Bulk denials are recorded at the request level.
func singlePhotoID(ids []string, kind Kind) string {
	if kind == KindSingle && len(ids) == 1 {
		return ids[0]
	}
	return ""
}

// record persists an audit entry best-effort, mirroring the authorization audit
// convention. A failed audit write must not surface storage internals or block
// the caller, but the decision-level record excludes all sensitive fields.
func (s *Service) record(ctx context.Context, entry AuditEntry) {
	_ = s.recorder.Record(ctx, entry)
}
