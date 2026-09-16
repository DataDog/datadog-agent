// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build oracle_test

package oracle

import (
	"context"
	"database/sql"
	"fmt"
	"io/fs"
	"os"
	"strings"
	"time"

	go_ora "github.com/sijms/go-ora/v2"
)

func setupTestDatabase() error {
	timeout := 10 * time.Minute
	if raw := os.Getenv("ORACLE_TEST_READY_TIMEOUT"); raw != "" {
		parsed, err := time.ParseDuration(raw)
		if err != nil || parsed <= 0 {
			return fmt.Errorf("ORACLE_TEST_READY_TIMEOUT must be a positive duration, got %q", raw)
		}
		timeout = parsed
	}
	connection, err := getTestConnectionConfig(useSysUser)
	if err != nil {
		return err
	}
	databaseURL := go_ora.BuildUrl(connection.Server, connection.Port, connection.ServiceName, connection.Username, connection.Password,
		map[string]string{"CONNECT TIMEOUT": "5", "TIMEOUT": "20"})
	db, err := sql.Open("oracle", databaseURL)
	if err != nil {
		return err
	}
	defer db.Close()
	db.SetMaxOpenConns(1)

	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()
	if err := waitForTestDatabase(ctx, db, ticker.C); err != nil {
		return fmt.Errorf("waiting for %s:%d/%s: %w", connection.Server, connection.Port, connection.ServiceName, err)
	}
	initCtx, cancelInit := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancelInit()
	return initializeTestDatabase(initCtx, db, os.DirFS("compose/initdb.d"))
}

const testDatabaseReadyQuery = `SELECT COUNT(*) FROM v$database
WHERE open_mode = 'READ WRITE'
AND NOT EXISTS (SELECT 1 FROM v$pdbs WHERE name <> 'PDB$SEED' AND open_mode <> 'READ WRITE')`

func waitForTestDatabase(ctx context.Context, db *sql.DB, retry <-chan time.Time) error {
	start := time.Now()
	for {
		var ready int
		queryCtx, cancel := context.WithTimeout(ctx, 20*time.Second)
		err := db.QueryRowContext(queryCtx, testDatabaseReadyQuery).Scan(&ready)
		cancel()
		if err == nil && ready == 1 {
			fmt.Printf("Oracle database ready after %s\n", time.Since(start).Round(time.Second))
			return nil
		}
		if err == nil {
			err = fmt.Errorf("database or pluggable databases are not open READ WRITE")
		}
		fmt.Printf("Waiting for Oracle (%s elapsed): %s\n", time.Since(start).Round(time.Second), err)
		select {
		case <-ctx.Done():
			return fmt.Errorf("%w; last readiness error: %v", ctx.Err(), err)
		case <-retry:
		}
	}
}

func initializeTestDatabase(ctx context.Context, db *sql.DB, scripts fs.FS) error {
	files, err := fs.ReadDir(scripts, ".")
	if err != nil {
		return fmt.Errorf("reading initialization scripts: %w", err)
	}
	for _, file := range files {
		if file.IsDir() || !strings.HasSuffix(file.Name(), ".sql") {
			continue
		}
		filename := file.Name()
		content, err := fs.ReadFile(scripts, filename)
		if err != nil {
			return fmt.Errorf("reading %s: %w", filename, err)
		}
		fmt.Printf("Executing %s\n", filename)
		if strings.HasSuffix(filename, ".nosplit.sql") {
			if _, err := db.ExecContext(ctx, string(content)); err != nil {
				return fmt.Errorf("executing %s: %w", filename, err)
			}
			continue
		}
		// Ordinary scripts contain one statement per line; PL/SQL uses .nosplit.sql.
		for lineNumber, line := range strings.Split(string(content), "\n") {
			statement := strings.TrimSpace(line)
			if statement == "" || strings.HasPrefix(statement, "--") {
				continue
			}
			statement = strings.TrimSpace(strings.TrimSuffix(statement, ";"))
			if statement == "" {
				continue
			}
			if _, err := db.ExecContext(ctx, statement); err != nil {
				return fmt.Errorf("executing %s:%d: %w", filename, lineNumber+1, err)
			}
		}
	}
	return nil
}
