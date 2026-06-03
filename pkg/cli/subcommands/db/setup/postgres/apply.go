// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package postgres

import (
	"context"
	"fmt"
	"net/url"
	"strings"

	"github.com/jackc/pgx/v5"

	"github.com/DataDog/datadog-agent/pkg/cli/subcommands/db/setup"
)

// Apply executes the plan in order. It stops on first failure and marks
// subsequent operations as pending. Returns a populated SetupResult.
func Apply(ctx context.Context, baseConn *pgx.Conn, baseConnConfig *pgx.ConnConfig, ops []*setup.Operation, state *setup.SetupState) *setup.SetupResult {
	// Per-database connections opened lazily, keyed by database name.
	dbConns := map[string]*pgx.Conn{}
	defer func() {
		for _, c := range dbConns {
			_ = c.Close(ctx)
		}
	}()

	failed := false
	for _, op := range ops {
		if op.Status == setup.StatusSkipped || op.Kind == setup.KindSkip {
			op.Status = setup.StatusSkipped
			continue
		}
		if op.Kind == setup.KindManualStep {
			op.Status = setup.StatusManual
			continue
		}
		if failed {
			op.Status = setup.StatusPending
			continue
		}

		conn, err := connForOp(ctx, op, baseConn, baseConnConfig, dbConns)
		if err != nil {
			op.Status = setup.StatusFailed
			op.Error = fmt.Errorf("open connection to db %q: %w", op.Database, err)
			failed = true
			continue
		}

		if err := executeOp(ctx, conn, op); err != nil {
			op.Status = setup.StatusFailed
			op.Error = err
			failed = true
			continue
		}

		// Reload-only settings: call pg_reload_conf() immediately after.
		if op.Kind == setup.KindReload {
			if _, err := baseConn.Exec(ctx, sqlReloadConf); err != nil {
				op.Status = setup.StatusFailed
				op.Error = fmt.Errorf("pg_reload_conf: %w", err)
				failed = true
				continue
			}
		}

		op.Status = setup.StatusCompleted
	}

	return buildResult(ops, state, failed)
}

func connForOp(ctx context.Context, op *setup.Operation, baseConn *pgx.Conn, baseConnConfig *pgx.ConnConfig, dbConns map[string]*pgx.Conn) (*pgx.Conn, error) {
	if op.Database == "" {
		return baseConn, nil
	}
	if c, ok := dbConns[op.Database]; ok {
		return c, nil
	}
	// Open a new connection to the target database.
	cfg := baseConnConfig.Copy()
	cfg.Database = op.Database
	c, err := pgx.ConnectConfig(ctx, cfg)
	if err != nil {
		return nil, err
	}
	dbConns[op.Database] = c
	return c, nil
}

func executeOp(ctx context.Context, conn *pgx.Conn, op *setup.Operation) error {
	if op.SQL == "" {
		return nil
	}
	_, err := conn.Exec(ctx, op.SQL, op.Args...)
	return err
}

func buildResult(ops []*setup.Operation, state *setup.SetupState, failed bool) *setup.SetupResult {
	result := &setup.SetupResult{
		Operations: ops,
		Flavor:     state.Flavor,
		PGVersion:  state.PGVersion,
	}

	for _, op := range ops {
		if op.Kind == setup.KindManualStep {
			result.ManualSteps = true
		}
		if op.RequiresRestart && op.Status == setup.StatusCompleted {
			result.RestartNeeded = true
		}
	}

	// Also flag restart if any restart was already pending before this run.
	if len(state.PendingRestart) > 0 {
		result.RestartNeeded = true
	}

	switch {
	case failed:
		result.Outcome = "failure"
	case result.ManualSteps || result.RestartNeeded:
		result.Outcome = "success_with_manual_steps"
	default:
		result.Outcome = "success"
	}

	return result
}

// ConnectWithConfig opens a connection using the provided ConnConfig.
func ConnectWithConfig(ctx context.Context, cfg *pgx.ConnConfig) (*pgx.Conn, error) {
	return pgx.ConnectConfig(ctx, cfg)
}

// ParseConnURI parses a postgres connection URI and returns the ConnConfig
// and the default database name from the URI.
func ParseConnURI(uri string) (*pgx.ConnConfig, string, error) {
	cfg, err := pgx.ParseConfig(uri)
	if err != nil {
		return nil, "", fmt.Errorf("invalid connection URI: %w", err)
	}
	db := cfg.Database
	if db == "" {
		u, err2 := url.Parse(uri)
		if err2 == nil {
			db = strings.TrimPrefix(u.Path, "/")
		}
	}
	if db == "" {
		db = "postgres"
	}
	return cfg, db, nil
}
