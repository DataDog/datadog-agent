// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build oracle_test

package oracle

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestSchemaLegacyColumnValuesAgainstDatabase(t *testing.T) {
	pdb := setupSchemaFixtures(t)
	require.NotEmpty(t, pdb, "this test requires a writable PDB")
	sys, _ := newSysCheck(t, "", "")
	defer sys.Teardown()
	var connected int
	require.NoError(t, getWrapper(&sys, &connected, "SELECT 1 FROM dual"))
	ctx := context.Background()
	admin, err := sys.db.Connx(ctx)
	require.NoError(t, err)
	defer admin.Close()
	defer func() {
		_, err := admin.ExecContext(context.Background(), schemaSwitchContainerSQL("CDB$ROOT"))
		require.NoError(t, err)
	}()
	keys := make(map[tableKey]struct{})
	columns := make(map[columnKey]struct{})
	defaults := make(map[tableKey]string)
	for _, container := range []string{"CDB$ROOT", pdb} {
		_, err := admin.ExecContext(ctx, schemaSwitchContainerSQL(container))
		require.NoError(t, err)
		var id int64
		var name string
		require.NoError(t, admin.QueryRowxContext(ctx, schemaSessionContainerQuery).Scan(&id, &name))
		value := fmt.Sprintf("'CONTAINER_%d'", id)
		for _, statement := range []string{
			fmt.Sprintf("CREATE TABLE %s.dd_legacy_values (id NUMBER, val VARCHAR2(40) DEFAULT %s)", schemaTestUser, value),
			fmt.Sprintf("CREATE INDEX %s.dd_legacy_normal ON %s.dd_legacy_values(id)", schemaTestUser, schemaTestUser),
			fmt.Sprintf("CREATE INDEX %s.dd_legacy_expression ON %s.dd_legacy_values(UPPER(val))", schemaTestUser, schemaTestUser),
		} {
			_, err := admin.ExecContext(ctx, statement)
			require.NoError(t, err)
		}
		key := tableKey{conID: id, owner: strings.ToUpper(schemaTestUser), table: "DD_LEGACY_VALUES"}
		keys[key] = struct{}{}
		defaults[key] = value
		for _, column := range []string{"ID", "VAL"} {
			columns[columnKey{tableKey: key, column: column}] = struct{}{}
		}
	}
	c, _ := newDefaultCheck(t, "collect_schemas:\n  enabled: true", "")
	defer c.Teardown()
	require.NoError(t, c.init())
	actualVersion := c.dbVersion
	// Also exercise the LONG fallback when CI runs a newer database.
	c.dbVersion = "12.1.0.2.0"
	c.db.SetMaxOpenConns(1)
	conn, err := c.db.Connx(ctx)
	require.NoError(t, err)
	defer conn.Close()
	c.schemaQueryer = conn
	details := c.tableDetailsForPage(ctx, keys, columns, keys)
	require.NoError(t, c.schemaContextError(ctx))
	for key, value := range defaults {
		detail := details[key]
		require.NotNil(t, detail, "database version %s, container %d", actualVersion, key.conID)
		require.Equal(t, value, strings.TrimSpace(detail.ColumnDefaults["VAL"]))
		normal := findIndex(detail.Indexes, "DD_LEGACY_NORMAL")
		require.NotNil(t, normal)
		require.Equal(t, []indexKeyPart{{Column: "ID"}}, normal.Columns)
		expression := findIndex(detail.Indexes, "DD_LEGACY_EXPRESSION")
		require.NotNil(t, expression)
		require.Len(t, expression.Columns, 1)
		require.Equal(t, `UPPER("VAL")`, strings.TrimSpace(expression.Columns[0].Expression))
	}
	var restoredID int64
	var restoredName string
	require.NoError(t, conn.QueryRowxContext(ctx, schemaSessionContainerQuery).Scan(&restoredID, &restoredName))
	require.Equal(t, "CDB$ROOT", restoredName)
}
