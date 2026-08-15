package postgres

import (
	"context"
	"errors"
	"fmt"

	"github.com/face-search-ai/api/internal/store"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Store struct {
	pool            *pgxpool.Pool
	expectedVersion int64
}

func Open(ctx context.Context, databaseURL string, maxConnections int32, expectedVersion int64) (*Store, error) {
	cfg, err := pgxpool.ParseConfig(databaseURL)
	if err != nil {
		return nil, fmt.Errorf("%w: invalid database configuration", store.ErrInvalidState)
	}
	cfg.MaxConns = maxConnections
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		return nil, MapError(err)
	}
	return &Store{pool: pool, expectedVersion: expectedVersion}, nil
}

func (s *Store) Close() { s.pool.Close() }
func (s *Store) Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error) {
	return s.pool.Exec(ctx, sql, args...)
}
func (s *Store) Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error) {
	return s.pool.Query(ctx, sql, args...)
}
func (s *Store) QueryRow(ctx context.Context, sql string, args ...any) pgx.Row {
	return s.pool.QueryRow(ctx, sql, args...)
}
func (s *Store) DB() store.DBTX                 { return s.pool }
func (s *Store) Ping(ctx context.Context) error { return MapError(s.pool.Ping(ctx)) }

func (s *Store) CheckSchema(ctx context.Context) error {
	var version int64
	var dirty bool
	if err := s.pool.QueryRow(ctx, "SELECT version, dirty FROM schema_migrations LIMIT 1").Scan(&version, &dirty); err != nil {
		return MapError(err)
	}
	if dirty || version != s.expectedVersion {
		return fmt.Errorf("%w: database schema is not ready", store.ErrInvalidState)
	}
	return nil
}

func (s *Store) WithinTransaction(ctx context.Context, fn store.TransactionFunc) error {
	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return MapError(err)
	}
	if err := fn(ctx, tx); err != nil {
		if rollbackErr := tx.Rollback(ctx); rollbackErr != nil && !errors.Is(rollbackErr, pgx.ErrTxClosed) {
			return errors.Join(err, MapError(rollbackErr))
		}
		return err
	}
	if err := tx.Commit(ctx); err != nil {
		_ = tx.Rollback(ctx)
		return MapError(err)
	}
	return nil
}

func MapError(err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, pgx.ErrNoRows) {
		return store.ErrNotFound
	}
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		switch pgErr.Code {
		case "23505", "23503", "23514", "23P01":
			return store.ErrConflict
		}
	}
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return fmt.Errorf("%w: database request did not complete", store.ErrUnavailable)
	}
	return fmt.Errorf("%w: database operation failed", store.ErrUnavailable)
}
