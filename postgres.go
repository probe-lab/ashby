package main

import (
	"context"
	"fmt"
	"sync"

	"github.com/jackc/pgx/v5/pgxpool"
)

type PgDataSource struct {
	connstr  string
	poolOnce sync.Once
	err      error
	pool     *pgxpool.Pool
}

func NewPgDataSource(connstr string) *PgDataSource {
	return &PgDataSource{
		connstr: connstr,
	}
}

func (p *PgDataSource) GetDataSet(ctx context.Context, query string, params ...any) (DataSet, error) {
	p.poolOnce.Do(func() {
		conf, err := pgxpool.ParseConfig(p.connstr)
		if err != nil {
			p.err = fmt.Errorf("unable to parse connection string: %w", err)
			return
		}
		pool, err := pgxpool.NewWithConfig(context.Background(), conf)
		if err != nil {
			p.err = fmt.Errorf("unable to connect to database: %w", err)
			return
		}
		p.pool = pool
	})

	if p.err != nil {
		return nil, p.err
	}

	conn, err := p.pool.Acquire(ctx)
	if err != nil {
		p.err = fmt.Errorf("unable to connect to database: %w", err)
		return nil, fmt.Errorf("execute query: %w", err)
	}
	defer conn.Release()

	rows, err := conn.Query(ctx, query, params...)
	if err != nil {
		return nil, fmt.Errorf("execute query: %w", err)
	}

	data := make(map[string][]any)
	fds := rows.FieldDescriptions()
	for rows.Next() {
		vals, err := rows.Values()
		if err != nil {
			return nil, fmt.Errorf("read row values: %w", err)
		}

		for i, fd := range fds {
			data[fd.Name] = append(data[fd.Name], vals[i])
		}
	}
	if rows.Err() != nil {
		return nil, fmt.Errorf("collect rows: %w", rows.Err())
	}

	return NewStaticDataSet(data), nil
}
