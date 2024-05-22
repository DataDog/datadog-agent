// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build test

package postgres

import (
	"context"
	"crypto/tls"
	"errors"
	"net"
	"strconv"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

const (
	// TLSDisabled represents the case when TLS is disabled.
	TLSDisabled = false
	// TLSEnabled represents the case when TLS is enabled.
	TLSEnabled = true

	// DefaultQuery is the default query to run.
	DefaultQuery = "default"
	emptyOption  = ""
	NoLimit      = 0
)

type ConnectionOptions struct {
	EnableTLS     bool
	ServerAddress string
	CustomDialer  func(ctx context.Context, network, addr string) (net.Conn, error)
	Username      string
	Password      string
	DBName        string
}

type PGClient struct {
	DB *pgxpool.Pool
}

func optionOrDefault(option, emptyOption, fallback string) string {
	if option == emptyOption {
		return fallback
	}
	return option
}

// NewPGClient creates a new Postgres client for testing purposes.
func NewPGClient(opts ConnectionOptions) (*PGClient, error) {
	if opts.ServerAddress == "" {
		return nil, errors.New("server address is required")
	}
	host, portStr, err := net.SplitHostPort(opts.ServerAddress)
	if err != nil {
		return nil, err
	}
	port, err := strconv.ParseUint(portStr, 10, 64)
	if err != nil {
		return nil, err
	}
	if port == 0 || port > 65535 {
		return nil, errors.New("invalid port")
	}
	config, err := pgxpool.ParseConfig("")
	if err != nil {
		return nil, err
	}

	config.ConnConfig.Host = host
	config.ConnConfig.Port = uint16(port)
	config.ConnConfig.User = optionOrDefault(opts.Username, emptyOption, "admin")
	config.ConnConfig.Password = optionOrDefault(opts.Password, emptyOption, "password")
	config.ConnConfig.Database = optionOrDefault(opts.DBName, emptyOption, "testdb")

	if opts.EnableTLS {
		config.ConnConfig.TLSConfig = &tls.Config{
			InsecureSkipVerify: true,
		}
	}

	if opts.CustomDialer != nil {
		config.ConnConfig.DialFunc = opts.CustomDialer
	}

	pool, err := pgxpool.NewWithConfig(context.Background(), config)
	if err != nil {
		return nil, err
	}
	return &PGClient{DB: pool}, nil
}

// Connect initiates a connection to the database, get a handle on the
// test db, and register cleanup handlers for the test. Finally, it saves the db
// handle and task context in the provided extras map.
func (c *PGClient) Connect() error {
	if c.DB == nil {
		return errors.New("db handle is nil")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	return c.DB.Ping(ctx)
}

func (c *PGClient) Close() {
	c.DB.Close()
}

func (c *PGClient) RunQuery(query string, args ...any) error {
	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Second)
	defer cancel()
	res, err := c.DB.Query(ctx, query, args...)
	res.Close()
	return err
}
