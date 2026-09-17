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
	"os"
	"testing"
	"time"

	go_ora "github.com/sijms/go-ora/v2"
	"github.com/stretchr/testify/require"
)

func setupTestDatabase() error {
	timeout, err := testDatabaseReadyTimeout()
	if err != nil {
		return err
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

func TestInitializeTestDatabaseIsRepeatable(t *testing.T) {
	db, err := getSysConnection(t)
	require.NoError(t, err)
	defer db.Close()
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	for range 2 {
		require.NoError(t, initializeTestDatabase(ctx, db, os.DirFS("compose/initdb.d")))
	}
	var rows int
	require.NoError(t, db.QueryRowContext(ctx, "SELECT COUNT(*) FROM t WHERE n = 18446744073709551615").Scan(&rows))
	require.Equal(t, 1, rows)
}
