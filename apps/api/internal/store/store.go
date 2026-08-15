package store

import (
	"context"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

type DBTX interface {
	Exec(context.Context, string, ...any) (pgconn.CommandTag, error)
	Query(context.Context, string, ...any) (pgx.Rows, error)
	QueryRow(context.Context, string, ...any) pgx.Row
}

type TransactionFunc func(context.Context, DBTX) error

type Transactor interface {
	WithinTransaction(context.Context, TransactionFunc) error
}

type SchemaChecker interface {
	CheckSchema(context.Context) error
}
