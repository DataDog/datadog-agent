// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package postgres

import (
	"context"
	"database/sql"
	"runtime"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/uptrace/bun"
	"github.com/uptrace/bun/dialect/pgdialect"
	"github.com/uptrace/bun/driver/pgdriver"
)

// Define a table schema, so that Bun can generates queries for it.
type DummyTable struct {
	bun.BaseModel `bun:"table:dummy,alias:d"`

	ID  int64 `bun:",pk,autoincrement"`
	Foo string
}

func PostgresTestsSupported() (bool, string) {
	if runtime.GOOS != "linux" {
		return true, "Postgres tests supported only on Linux"
	}

	return false, ""
}

func ConnectAndGetDB(t *testing.T, serverAddr string) (*bun.DB, context.Context) {
	t.Helper()

	time.Sleep(5 * time.Second)

	ctx := context.Background()

	pg := sql.OpenDB(pgdriver.NewConnector(
		pgdriver.WithNetwork("tcp"),
		pgdriver.WithAddr(serverAddr),
		pgdriver.WithInsecure(true),
		pgdriver.WithUser("admin"),
		pgdriver.WithPassword("password"),
		pgdriver.WithDatabase("testdb"),
	))
	require.NoError(t, pg.Ping())

	db := bun.NewDB(pg, pgdialect.New())

	// Cleanup test tables
	t.Cleanup(func() {
		_, _ = db.NewDropTable().Model((*DummyTable)(nil)).Exec(ctx)
	})

	return db, ctx
}
