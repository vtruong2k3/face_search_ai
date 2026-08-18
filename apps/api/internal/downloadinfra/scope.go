// Package downloadinfra adapts platform services to the download domain ports,
// keeping the domain free of infrastructure and event-package types.
package downloadinfra

import (
	"context"
	"time"

	"github.com/face-search-ai/api/internal/domain/download"
	"github.com/face-search-ai/api/internal/domain/event"
)

type ScopeResolver struct{ events *event.Service }

func NewScopeResolver(events *event.Service) *ScopeResolver { return &ScopeResolver{events: events} }

func (r *ScopeResolver) FindPublicDownloadScope(ctx context.Context, token string, now time.Time) (download.Scope, error) {
	scope, err := r.events.FindPublicDownloadScope(ctx, token, now)
	if err != nil {
		return download.Scope{}, err
	}
	return download.Scope{OrganizationID: scope.OrganizationID, EventID: scope.EventID, DownloadsEnabled: scope.DownloadsEnabled}, nil
}

var _ download.ScopeResolver = (*ScopeResolver)(nil)
