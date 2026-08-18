package searchinfra

import (
	"context"
	"time"

	"github.com/face-search-ai/api/internal/domain/event"
)

type ScopeResolver struct{ events *event.Service }

func NewScopeResolver(events *event.Service) *ScopeResolver { return &ScopeResolver{events: events} }

func (r *ScopeResolver) FindPublicSearchScope(ctx context.Context, token string, now time.Time) (event.PublicSearchScope, error) {
	return r.events.FindPublicSearchScope(ctx, token, now)
}
