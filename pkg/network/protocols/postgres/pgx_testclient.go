// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build test

package postgres

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// PGXClient represents a Postgres client using the pgx library.
type PGXClient struct {
	DB *pgxpool.Pool
}

// NewPGXClient creates a new Postgres client for testing purposes.
func NewPGXClient(opts ConnectionOptions) (*PGXClient, error) {
	config, err := pgxpool.ParseConfig(buildDSN(opts))
	if err != nil {
		return nil, err
	}

	pool, err := pgxpool.NewWithConfig(context.Background(), config)
	if err != nil {
		return nil, err
	}
	return &PGXClient{DB: pool}, nil
}

// Ping checks if the connection to the database is alive.
func (c *PGXClient) Ping() error {
	if c.DB == nil {
		return errors.New("db handle is nil")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	return c.DB.Ping(ctx)
}

// Close closes the connection to the database.
func (c *PGXClient) Close() {
	c.DB.Close()
}

// RunQuery runs a query on the database.
func (c *PGXClient) RunQuery(query string, args ...any) error {
	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Second)
	defer cancel()
	res, err := c.DB.Query(ctx, query, args...)
	res.Close()
	return err
}
