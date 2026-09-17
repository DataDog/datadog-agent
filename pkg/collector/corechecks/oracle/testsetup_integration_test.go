// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build oracle_test

package oracle

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

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
