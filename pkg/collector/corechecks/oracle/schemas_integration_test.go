// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build oracle_test

package oracle

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const schemaTestUser = "c##dd_schema_test"

// setupSchemaFixtures creates a schema the collector will actually pick up. The fixtures in
// compose/initdb.d belong to SYS, which is oracle_maintained and therefore filtered out.
func setupSchemaFixtures(t *testing.T) {
	sysCheck, _ := newSysCheck(t, "", "")
	require.NoError(t, sysCheck.Run())
	t.Cleanup(sysCheck.Teardown)

	dropUser := fmt.Sprintf("drop user %s cascade", schemaTestUser)
	_, _ = sysCheck.db.Exec(dropUser)
	t.Cleanup(func() { _, _ = sysCheck.db.Exec(dropUser) })

	for _, stmt := range []string{
		fmt.Sprintf("create user %s identified by dd_schema_test container=all", schemaTestUser),
		fmt.Sprintf("grant unlimited tablespace to %s", schemaTestUser),
		fmt.Sprintf(`create table %s.dd_orders (
			order_id number(12,0) not null,
			status varchar2(20 char) default 'NEW',
			amount number(10,2),
			created_at timestamp,
			constraint dd_orders_pk primary key (order_id)
		)`, schemaTestUser),
		fmt.Sprintf("create index %s.dd_orders_status_idx on %s.dd_orders (status)", schemaTestUser, schemaTestUser),
		fmt.Sprintf("comment on table %s.dd_orders is 'Schema collection fixture'", schemaTestUser),
		fmt.Sprintf("comment on column %s.dd_orders.amount is 'Order total'", schemaTestUser),
		fmt.Sprintf("create global temporary table %s.dd_staging (batch_id number) on commit preserve rows", schemaTestUser),
		// Object tables (CREATE TABLE t OF type_t) only appear in cdb_object_tables, not
		// cdb_tables, so this fixture exercises that second branch of the schema query.
		fmt.Sprintf("create or replace type %s.dd_address_t as object (street varchar2(100), city varchar2(100))", schemaTestUser),
		fmt.Sprintf("create table %s.dd_addresses of %s.dd_address_t", schemaTestUser, schemaTestUser),
	} {
		_, err := sysCheck.db.Exec(stmt)
		require.NoErrorf(t, err, "fixture failed: %s", stmt)
	}
}

// collectSchemaEvents runs one schema collection and returns the dbm-metadata payloads.
func collectSchemaEvents(t *testing.T) []schemaEvent {
	c, sender := newDefaultCheck(t, "schemas:\n  enabled: true\n  collection_interval: 1", "")
	defer c.Teardown()
	require.NoError(t, c.Run())
	require.NoError(t, c.SchemaCollection())

	var events []schemaEvent
	for _, call := range sender.Calls {
		if call.Method != "EventPlatformEvent" {
			continue
		}
		if track, _ := call.Arguments.Get(1).(string); track != "dbm-metadata" {
			continue
		}
		var e schemaEvent
		if err := json.Unmarshal(call.Arguments.Get(0).([]byte), &e); err != nil {
			continue
		}
		if e.Kind == "oracle_databases" {
			events = append(events, e)
		}
	}
	return events
}

func findTable(events []schemaEvent, owner, name string) *schemaTable {
	for _, e := range events {
		for _, container := range e.Metadata {
			for _, schema := range container.Schemas {
				if !strings.EqualFold(schema.Name, owner) {
					continue
				}
				for _, table := range schema.Tables {
					if strings.EqualFold(table.Name, name) {
						return table
					}
				}
			}
		}
	}
	return nil
}

// TestSchemaCollectionAgainstDatabase exercises the parts a mock cannot: that every query is
// valid SQL on this Oracle version, that the grants are sufficient, and that the dictionary
// returns what the renderer assumes.
func TestSchemaCollectionAgainstDatabase(t *testing.T) {
	setupSchemaFixtures(t)

	events := collectSchemaEvents(t)
	require.NotEmpty(t, events, "no oracle_databases payload was emitted")

	for _, e := range events {
		assert.Equal(t, "oracle", e.Dbms)
		assert.NotZero(t, e.CollectionStartedAt, "every payload carries the snapshot id")
		require.NotEmpty(t, e.Metadata)
		assert.NotEmpty(t, e.Metadata[0].ID, "container id")
	}

	orders := findTable(events, schemaTestUser, "dd_orders")
	require.NotNil(t, orders, "the fixture table was not collected")

	assert.Equal(t, "table", orders.TableType)
	assert.Equal(t, "Schema collection fixture", orders.Comment)
	assert.NotEmpty(t, orders.ID, "table id comes from cdb_objects.object_id")

	columns := map[string]schemaColumn{}
	for _, col := range orders.Columns {
		columns[col.Name] = col
	}
	require.Contains(t, columns, "ORDER_ID")
	assert.Equal(t, "NUMBER(12,0)", columns["ORDER_ID"].DataType)
	assert.False(t, columns["ORDER_ID"].Nullable)

	// DATA_LENGTH would report 80 on AL32UTF8; the declared length is 20 characters.
	require.Contains(t, columns, "STATUS")
	assert.Equal(t, "VARCHAR2(20 CHAR)", columns["STATUS"].DataType)

	require.Contains(t, columns, "AMOUNT")
	assert.Equal(t, "NUMBER(10,2)", columns["AMOUNT"].DataType)
	assert.Equal(t, "Order total", columns["AMOUNT"].Comment)

	require.Contains(t, columns, "CREATED_AT")
	assert.Equal(t, "TIMESTAMP(6)", columns["CREATED_AT"].DataType,
		"temporal precision is already inside DATA_TYPE and must not be appended twice")

	var pk *constraintInfo
	for _, con := range orders.Constraints {
		if con.Type == "primary_key" {
			pk = con
		}
	}
	require.NotNil(t, pk, "primary key was not collected")
	assert.Equal(t, []string{"ORDER_ID"}, pk.Columns)

	var statusIdx *indexInfo
	for _, idx := range orders.Indexes {
		if strings.EqualFold(idx.Name, "dd_orders_status_idx") {
			statusIdx = idx
		}
	}
	require.NotNil(t, statusIdx, "index was not collected")
	assert.Equal(t, []string{"STATUS"}, statusIdx.Columns)
	assert.False(t, statusIdx.Unique)

	staging := findTable(events, schemaTestUser, "dd_staging")
	require.NotNil(t, staging, "the temporary table was not collected")
	assert.Contains(t, staging.Properties, "temporary")
	require.NotNil(t, staging.Temporary)
	assert.Equal(t, "SYS$SESSION", staging.Temporary.Scope)

	addresses := findTable(events, schemaTestUser, "dd_addresses")
	require.NotNil(t, addresses, "the object table was not collected")
	assert.Equal(t, "table", addresses.TableType)
	assert.Contains(t, addresses.Properties, "object_table")
	require.NotNil(t, addresses.ObjectType)
	assert.Equal(t, strings.ToUpper(schemaTestUser), strings.ToUpper(addresses.ObjectType.TypeOwner))
	assert.Equal(t, "DD_ADDRESS_T", strings.ToUpper(addresses.ObjectType.TypeName))
	assert.NotEmpty(t, addresses.ID, "object tables get their id from cdb_objects like any other table")

	addressColumns := map[string]schemaColumn{}
	for _, col := range addresses.Columns {
		addressColumns[strings.ToUpper(col.Name)] = col
	}
	require.Contains(t, addressColumns, "STREET")
	require.Contains(t, addressColumns, "CITY")
	assert.NotContains(t, addressColumns, "SYS_NC_OID$",
		"the hidden object-identifier column must stay excluded, same as for ordinary tables")
	assert.NotContains(t, addressColumns, "SYS_NC_ROWINFO$")
}

// TestSchemaCollectionExcludesOracleMaintained guards the filter that keeps the catalog free of
// Oracle's own dozens of built-in schemas.
func TestSchemaCollectionExcludesOracleMaintained(t *testing.T) {
	setupSchemaFixtures(t)

	for _, e := range collectSchemaEvents(t) {
		for _, container := range e.Metadata {
			for _, schema := range container.Schemas {
				assert.NotContains(t, []string{"SYS", "SYSTEM", "XDB", "AUDSYS", "DBSNMP"},
					strings.ToUpper(schema.Name),
					"Oracle-maintained schema leaked into the payload")
			}
		}
	}
}
