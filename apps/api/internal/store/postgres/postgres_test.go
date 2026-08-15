package postgres

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/face-search-ai/api/internal/store"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

func TestMapError(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want error
	}{
		{name: "missing row", err: pgx.ErrNoRows, want: store.ErrNotFound},
		{name: "unique violation", err: &pgconn.PgError{Code: "23505", Detail: "secret row value"}, want: store.ErrConflict},
		{name: "canceled", err: context.Canceled, want: store.ErrUnavailable},
		{name: "unknown", err: errors.New("postgres://user:secret@db/private"), want: store.ErrUnavailable},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := MapError(tt.err)
			if !errors.Is(got, tt.want) {
				t.Fatalf("MapError() = %v, want %v", got, tt.want)
			}
			if strings.Contains(got.Error(), "secret") {
				t.Fatalf("mapped error leaked sensitive detail: %v", got)
			}
		})
	}
}
