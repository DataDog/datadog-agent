// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build oracle_test

package oracle

import (
	"context"
	"database/sql"
	"errors"
	"regexp"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/require"
)

func expectSchemaSession(mock sqlmock.Sqlmock, id int64, name string) {
	mock.ExpectQuery(regexp.QuoteMeta(schemaSessionContainerQuery)).WillReturnRows(
		sqlmock.NewRows([]string{"CON_ID", "CON_NAME"}).AddRow(id, name))
}

func expectSchemaSwitch(mock sqlmock.Sqlmock, id int64, name string) {
	expectSchemaSession(mock, 1, "CDB$ROOT")
	mock.ExpectQuery("SELECT name FROM v\\$containers").WithArgs(id).WillReturnRows(
		sqlmock.NewRows([]string{"NAME"}).AddRow(name))
	mock.ExpectExec(regexp.QuoteMeta(schemaSwitchContainerSQL(name))).WillReturnResult(sqlmock.NewResult(0, 0))
	expectSchemaSession(mock, id, name)
}

func expectSchemaRestore(mock sqlmock.Sqlmock) {
	mock.ExpectExec(regexp.QuoteMeta(schemaSwitchContainerSQL("CDB$ROOT"))).WillReturnResult(sqlmock.NewResult(0, 0))
	expectSchemaSession(mock, 1, "CDB$ROOT")
}

func TestSchemaContainerRestoredAfterCollection(t *testing.T) {
	for _, scenario := range []string{"success", "query error", "panic", "canceled"} {
		t.Run(scenario, func(t *testing.T) {
			c, pool, mock, cleanup := newSchemaCheck(t)
			defer cleanup()
			pool.SetMaxOpenConns(1)
			conn, err := pool.Connx(context.Background())
			require.NoError(t, err)
			defer conn.Close()
			c.schemaQueryer = conn
			expectSchemaSwitch(mock, 3, "PDB1")
			expectSchemaRestore(mock)
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			called := false
			run := func() error {
				return c.withSchemaContainer(ctx, 3, func() error {
					called = true
					switch scenario {
					case "query error":
						return errors.New("metadata unavailable")
					case "panic":
						panic("metadata panic")
					case "canceled":
						cancel()
						return ctx.Err()
					}
					return nil
				})
			}
			if scenario == "panic" {
				require.PanicsWithValue(t, "metadata panic", func() { _ = run() })
			} else if scenario == "success" {
				require.NoError(t, run())
			} else {
				require.Error(t, run())
			}
			require.True(t, called)
			require.Same(t, conn, c.schemaQueryer)
			require.NoError(t, mock.ExpectationsWereMet())
		})
	}
}

func TestSchemaContainerDiscardedWhenRestoreFails(t *testing.T) {
	c, pool, mock, cleanup := newSchemaCheck(t)
	defer cleanup()
	conn, err := pool.Connx(context.Background())
	require.NoError(t, err)
	defer conn.Close()
	c.schemaQueryer = conn
	expectSchemaSwitch(mock, 3, "PDB1")
	mock.ExpectExec(regexp.QuoteMeta(schemaSwitchContainerSQL("CDB$ROOT"))).WillReturnError(errors.New("connection lost"))
	mock.ExpectClose()
	err = c.withSchemaContainer(context.Background(), 3, func() error { return nil })
	require.ErrorContains(t, err, "restore schema connection")
	require.Error(t, c.schemaQueryError)
	require.ErrorIs(t, conn.Raw(func(any) error { return nil }), sql.ErrConnDone)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestSchemaContainerRejectsChangedIdentity(t *testing.T) {
	c, _, mock, cleanup := newSchemaCheck(t)
	defer cleanup()
	expectSchemaSession(mock, 1, "CDB$ROOT")
	mock.ExpectQuery("SELECT name FROM v\\$containers").WithArgs(int64(3)).WillReturnRows(sqlmock.NewRows([]string{"NAME"}).AddRow("PDB1"))
	mock.ExpectExec(regexp.QuoteMeta(schemaSwitchContainerSQL("PDB1"))).WillReturnResult(sqlmock.NewResult(0, 0))
	expectSchemaSession(mock, 4, "PDB1")
	expectSchemaRestore(mock)
	err := c.withSchemaContainer(context.Background(), 3, func() error {
		t.Fatal("must not collect from a replacement PDB")
		return nil
	})
	require.ErrorContains(t, err, "expected 3, got 4")
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestSchemaLegacyColumnValuesUseCorrectContainer(t *testing.T) {
	for _, version := range []string{"12.1.0.2.0", "12.2.0.1.0", "19.0.0.0.0", "21.3.0.0.0"} {
		t.Run(version, func(t *testing.T) {
			c, _, mock, cleanup := newSchemaCheck(t)
			defer cleanup()
			c.dbVersion = version
			columns := make(map[columnKey]struct{})
			details := make(map[tableKey]*tableDetails)
			for _, id := range []int64{3, 4} {
				key := tableKey{conID: id, owner: "APP", table: "T"}
				columns[columnKey{tableKey: key, column: "C"}] = struct{}{}
				details[key] = &tableDetails{Indexes: []*indexInfo{{Name: "IDX", Columns: []indexKeyPart{{Column: "C"}, {Column: "SYS_NC00002$"}}}}}
				expectSchemaSwitch(mock, id, "PDB")
				mock.ExpectQuery(`(?s)SELECT TO_NUMBER.*c\.data_default.*FROM dba_tab_cols`).WithArgs(schemaColumnArgs(id, "APP", "T", "C", "SYS_NC00002$")...).WillReturnRows(
					sqlmock.NewRows([]string{"CON_ID", "OWNER", "TABLE_NAME", "COLUMN_NAME", "DATA_DEFAULT"}).
						AddRow(id, "APP", "T", "C", "'DEFAULT'").AddRow(id, "APP", "T", "SYS_NC00002$", "UPPER(C)"))
				expectSchemaRestore(mock)
			}
			c.enrichSchemaColumnValues(context.Background(), details, columns)
			for _, detail := range details {
				require.Equal(t, "'DEFAULT'", detail.ColumnDefaults["C"])
				require.Equal(t, []indexKeyPart{{Column: "C"}, {Expression: "UPPER(C)"}}, detail.Indexes[0].Columns)
			}
			require.NoError(t, mock.ExpectationsWereMet())
		})
	}
}

func TestSchemaExpressionFailurePreservesIndexes(t *testing.T) {
	for _, version := range []string{"12.1.0.2.0", "23.0.0.0.0"} {
		t.Run(version, func(t *testing.T) {
			c, _, mock, cleanup := newSchemaCheck(t)
			defer cleanup()
			c.dbVersion = version
			key := tableKey{conID: 3, owner: "APP", table: "T"}
			mock.MatchExpectationsInOrder(false)
			mock.ExpectQuery("FROM cdb_indexes").WillReturnRows(sqlmock.NewRows([]string{"CON_ID", "TABLE_OWNER", "TABLE_NAME", "OWNER", "INDEX_NAME", "UNIQUENESS", "INDEX_TYPE", "COLUMN_NAME"}).
				AddRow(3, "APP", "T", "APP", "NORMAL", "NONUNIQUE", "NORMAL", "C").
				AddRow(3, "APP", "T", "APP", "FUNCTION", "NONUNIQUE", "FUNCTION-BASED NORMAL", "SYS_NC00002$"))
			if version == "12.1.0.2.0" {
				expectSchemaSession(mock, 3, "PDB1")
			}
			mock.ExpectQuery("tab_cols").WillReturnError(errors.New("ORA-00904: invalid identifier"))
			details := c.tableDetails(context.Background(), map[tableKey]struct{}{key: {}}, nil)
			require.Contains(t, details, key)
			require.Len(t, details[key].Indexes, 2)
			require.Equal(t, "C", details[key].Indexes[0].Columns[0].Column)
			require.NoError(t, mock.ExpectationsWereMet())
		})
	}
	require.NotContains(t, indexesQuery, "tab_cols")
	require.NotContains(t, indexesQuery, "data_default")
}

func TestSchemaContainerNameIsQuoted(t *testing.T) {
	require.Equal(t, `ALTER SESSION SET CONTAINER = "PDB""1"`, schemaSwitchContainerSQL(`PDB"1`))
}

func TestSchemaLocalColumnValuesKeepStableBoundSQL(t *testing.T) {
	build := func(id int64, owner, table, column string) schemaSQL {
		keys := map[columnKey]struct{}{{tableKey: tableKey{conID: id, owner: owner, table: table}, column: column}: {}}
		filters := columnFilterChunks(keys, relationColumnNames{conID: schemaCurrentContainerID, owner: "c.owner", relation: "c.table_name"}, "c.column_name")
		require.Len(t, filters, 1)
		return filters[0]
	}
	first := build(3, "APP", "T1", "COL1")
	second := build(4, "O'WNER", "T'2", "C'OL2")
	require.Equal(t, first.text, second.text)
	require.NotContains(t, second.text, "O'WNER")
	require.NotContains(t, second.text, "T'2")
	require.NotContains(t, second.text, "C'OL2")
	require.Equal(t, sql.Named("col0con", int64(4)), second.args[0])
	require.Equal(t, sql.Named("col0owner", "O'WNER"), second.args[1])
	require.Equal(t, sql.Named("col0table", "T'2"), second.args[2])
	require.Equal(t, sql.Named("col0name0", sql.NullString{String: "C'OL2", Valid: true}), second.args[3])
	for _, arg := range second.args[4:] {
		require.Equal(t, sql.NullString{}, arg.(sql.NamedArg).Value)
	}
}
