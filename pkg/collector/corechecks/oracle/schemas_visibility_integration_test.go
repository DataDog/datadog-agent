// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build oracle_test

package oracle

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestSchemaContainerDictionaryVisibilityAgainstDatabase(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()
	admin, err := getSysConnection(t)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, admin.Close()) })
	admin.SetMaxOpenConns(1)

	var conID int64
	var pdb string
	require.NoError(t, admin.QueryRowContext(ctx, `SELECT con_id, name FROM v$pdbs
WHERE con_id > 2 AND open_mode = 'READ WRITE' AND restricted = 'NO' ORDER BY con_id FETCH FIRST 1 ROW ONLY`).Scan(&conID, &pdb))
	quotedPDB := `"` + strings.ReplaceAll(pdb, `"`, `""`) + `"`
	const username = "C##DD_SCHEMA_VISIBILITY"
	const password = "dd_schema_visibility_test"
	exec := func(t *testing.T, query string) {
		t.Helper()
		_, err := admin.ExecContext(ctx, query)
		require.NoError(t, err)
	}
	exec(t, "CREATE USER "+username+" IDENTIFIED BY "+password+" CONTAINER=ALL")
	t.Cleanup(func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), time.Minute)
		defer cleanupCancel()
		_, err := admin.ExecContext(cleanupCtx, "ALTER SESSION SET CONTAINER=CDB$ROOT")
		require.NoError(t, err)
		_, err = admin.ExecContext(cleanupCtx, "DROP USER "+username+" CASCADE")
		require.NoError(t, err)
	})
	exec(t, "GRANT CREATE SESSION, SELECT_CATALOG_ROLE, UNLIMITED TABLESPACE TO "+username+" CONTAINER=ALL")
	exec(t, "ALTER USER "+username+" SET CONTAINER_DATA=ALL CONTAINER=CURRENT")
	exec(t, "ALTER SESSION SET CONTAINER="+quotedPDB)
	exec(t, "CREATE TABLE "+username+".ORDERS (ID NUMBER)")
	exec(t, "ALTER SESSION SET CONTAINER=CDB$ROOT")

	connection := getConnectData(t, useDefaultUser)
	connection.Username = username
	connection.Password = password
	c, sender := newTestCheck(t, connection, "", "")
	t.Cleanup(c.Teardown)
	var ready int
	require.NoError(t, getWrapper(&c, &ready, "SELECT 1 FROM dual"))
	require.NoError(t, c.init())
	c.config.Schemas.Enabled = true
	c.config.Schemas.IncludeDatabases = []string{"^" + regexp.QuoteMeta(pdb) + "$"}
	c.config.Schemas.IncludeSchemas = []string{"^" + username + "$"}
	c.config.Schemas.IncludeTables = []string{"^ORDERS$"}
	views := false
	c.config.Schemas.CollectViews = &views
	containerID := strconv.FormatInt(conID, 10)

	setUsersVisible := func(t *testing.T, visible bool) {
		t.Helper()
		containers := "(CDB$ROOT)"
		if visible {
			containers = "ALL"
		}
		exec(t, "ALTER USER "+username+" SET CONTAINER_DATA="+containers+" FOR SYS.CDB_USERS CONTAINER=CURRENT")
	}
	collect := func(t *testing.T) []schemaEvent {
		t.Helper()
		firstCall := len(sender.Calls)
		require.NoError(t, c.schemaCollection(ctx))
		var events []schemaEvent
		for _, call := range sender.Calls[firstCall:] {
			if call.Method != "EventPlatformEvent" || call.Arguments.String(1) != "dbm-metadata" {
				continue
			}
			var event schemaEvent
			require.NoError(t, json.Unmarshal(call.Arguments.Get(0).([]byte), &event))
			events = append(events, event)
		}
		return events
	}
	assertCollected := func(t *testing.T) {
		t.Helper()
		events := collect(t)
		require.NotEmpty(t, events)
		require.Equal(t, len(events), events[len(events)-1].CollectionPayloadsCount)
		var tables []string
		for _, event := range events {
			for _, container := range event.Metadata {
				require.Equal(t, containerID, container.ID)
				for _, schema := range container.Schemas {
					for _, table := range schema.Tables {
						tables = append(tables, table.Name)
					}
				}
			}
		}
		require.Equal(t, []string{"ORDERS"}, tables)
	}

	require.True(t, t.Run("visible", assertCollected))
	setUsersVisible(t, false)
	t.Run("hidden_at_discovery", func(t *testing.T) {
		var count int
		require.NoError(t, c.db.GetContext(ctx, &count, `SELECT COUNT(*) FROM v$containers
WHERE con_id = :1 AND open_mode IN ('READ WRITE', 'READ ONLY') AND NVL(restricted, 'NO') = 'NO'`, conID))
		require.Equal(t, 1, count, "the existing open-mode filters still admit this PDB")
		require.NoError(t, c.db.GetContext(ctx, &count, "SELECT COUNT(*) FROM cdb_users WHERE con_id = :1", conID))
		require.Zero(t, count, "owner discovery would mistake hidden users for an empty PDB")
		require.NoError(t, c.db.GetContext(ctx, &count,
			"SELECT COUNT(*) FROM cdb_tables WHERE con_id = :1 AND owner = :2 AND table_name = 'ORDERS'", conID, username))
		require.Equal(t, 1, count, "the fixture table still exists and is visible")
		require.Empty(t, collect(t), "an invisible PDB must not produce a completed empty snapshot")
		containers, err := c.containerNames(ctx)
		require.NoError(t, err)
		require.NotContains(t, containers, conID)
	})
	setUsersVisible(t, true)
	t.Run("hidden_before_completion", func(t *testing.T) {
		containers, err := c.containerNames(ctx)
		require.NoError(t, err)
		require.Contains(t, containers, conID)
		var events []schemaEvent
		coordinator := newSchemaSnapshotCoordinator(func(payload []byte) {
			var event schemaEvent
			require.NoError(t, json.Unmarshal(payload, &event))
			events = append(events, event)
		})
		coordinator.validate = func(id string) error { return c.validateSchemaContainer(ctx, id) }
		coordinator.add(schemaEvent{CollectionStartedAt: 1, Metadata: []containerObject{{ID: containerID, Name: pdb}}})
		t.Cleanup(func() { setUsersVisible(t, true) })
		setUsersVisible(t, false)
		require.ErrorContains(t, coordinator.complete(), fmt.Sprintf("container %d is no longer available", conID))
		require.Len(t, events, 1)
		require.Zero(t, events[0].CollectionPayloadsCount, "loss of visibility must prevent snapshot completion")
	})
	t.Run("visible_again", assertCollected)
}
