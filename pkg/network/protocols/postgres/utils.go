// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package postgres

import (
	"context"
	"database/sql"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/uptrace/bun"
	"github.com/uptrace/bun/dialect/pgdialect"
	"github.com/uptrace/bun/driver/pgdriver"
)

// DummyTable defines a table schema, so that `bun` module can generate queries for it.
type DummyTable struct {
	bun.BaseModel `bun:"table:dummy,alias:d"`

	ID  int64 `bun:",pk,autoincrement"`
	Foo string
}

var dummyModel = &DummyTable{ID: 1, Foo: "bar"}

// GetPGHandle returns a handle on the test Postgres DB. This does not initiate
// a connection
func GetPGHandle(t *testing.T, serverAddr string) *sql.DB {
	t.Helper()

	pg := sql.OpenDB(pgdriver.NewConnector(
		pgdriver.WithNetwork("tcp"),
		pgdriver.WithAddr(serverAddr),
		pgdriver.WithInsecure(true),
		pgdriver.WithUser("admin"),
		pgdriver.WithPassword("password"),
		pgdriver.WithDatabase("testdb"),
	))

	return pg
}

// ConnectAndGetDB initiates a connection to the database, get a handle on the
// test db, and register cleanup handlers for the test. Finally, it saves the db
// handle and task context in the provided extras map.
func ConnectAndGetDB(t *testing.T, serverAddr string, extras map[string]interface{}) {
	t.Helper()

	ctx := context.Background()

	pg := GetPGHandle(t, serverAddr)
	db := bun.NewDB(pg, pgdialect.New())

	if extras != nil {
		extras["ctx"] = ctx
		extras["db"] = db
	}
}

// The following are helpers around bun to quickly execute SQL query for use in
// protocol classification tests.

func getCtx(extras map[string]interface{}) (*bun.DB, context.Context) {
	db := extras["db"].(*bun.DB)
	taskCtx := extras["ctx"].(context.Context)

	return db, taskCtx
}

// RunAlterQuery runs ALTER query on the test DB.
func RunAlterQuery(t *testing.T, extras map[string]interface{}) {
	t.Helper()
	db, ctx := getCtx(extras)

	_, err := db.NewAddColumn().Model((*DummyTable)(nil)).ColumnExpr("new_column BOOL").Exec(ctx)
	require.NoError(t, err)
}

// RunCreateQuery creates a new table.
func RunCreateQuery(t *testing.T, extras map[string]interface{}) {
	t.Helper()
	db, ctx := getCtx(extras)

	_, err := db.NewCreateTable().Model((*DummyTable)(nil)).Exec(ctx)
	require.NoError(t, err)
}

// RunDeleteQuery run a deletion query on the test DB.
func RunDeleteQuery(t *testing.T, extras map[string]interface{}) {
	t.Helper()
	db, ctx := getCtx(extras)

	_, err := db.NewDelete().Model(dummyModel).WherePK().Exec(ctx)
	require.NoError(t, err)
}

// RunDropQuery drops a table.
func RunDropQuery(t *testing.T, extras map[string]interface{}) {
	t.Helper()
	db, ctx := getCtx(extras)

	_, err := db.NewDropTable().Model((*DummyTable)(nil)).IfExists().Exec(ctx)
	require.NoError(t, err)
}

// RunInsertQuery inserts a new row in the table.
func RunInsertQuery(t *testing.T, id int64, extras map[string]interface{}) {
	t.Helper()
	db, ctx := getCtx(extras)

	model := *dummyModel
	model.ID = id

	_, err := db.NewInsert().Model(&model).Exec(ctx)
	require.NoError(t, err)
}

// RunSelectQuery runs a SELECT query on the test DB.
func RunSelectQuery(t *testing.T, extras map[string]interface{}) {
	t.Helper()
	db, ctx := getCtx(extras)

	_, err := db.NewSelect().Model(dummyModel).Exec(ctx)
	require.NoError(t, err)
}

// RunUpdateQuery runs an UPDATE query on the test DB.
func RunUpdateQuery(t *testing.T, extras map[string]interface{}) {
	t.Helper()
	db, ctx := getCtx(extras)

	newModel := *dummyModel
	newModel.Foo = "baz"

	_, err := db.NewUpdate().Model(&newModel).WherePK().Exec(ctx)
	require.NoError(t, err)
}
