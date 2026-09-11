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
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const schemaTestUser = "c##dd_schema_test"

func setupSchemaFixtures(t *testing.T) string {
	sysCheck, _ := newSysCheck(t, "", "")
	require.NoError(t, sysCheck.Run())
	t.Cleanup(sysCheck.Teardown)

	dropUser := fmt.Sprintf("drop user %s cascade", schemaTestUser)
	_, _ = sysCheck.db.Exec(dropUser)
	t.Cleanup(func() { _, _ = sysCheck.db.Exec(dropUser) })

	for _, stmt := range []string{
		fmt.Sprintf("create user %s identified by dd_schema_test container=all", schemaTestUser),
		fmt.Sprintf("grant unlimited tablespace to %s", schemaTestUser),
		// Materialized views need CREATE TABLE for their backing tables.
		fmt.Sprintf("grant create table, create materialized view, query rewrite to %s", schemaTestUser),
		fmt.Sprintf("grant read, write on directory data_pump_dir to %s", schemaTestUser),
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
		// Object tables appear only in CDB_OBJECT_TABLES.
		fmt.Sprintf("create or replace type %s.dd_address_t as object (street varchar2(100), city varchar2(100))", schemaTestUser),
		fmt.Sprintf("create table %s.dd_addresses of %s.dd_address_t", schemaTestUser, schemaTestUser),
		fmt.Sprintf("create table %s.dd_object_col (id number, addr %s.dd_address_t)", schemaTestUser, schemaTestUser),

		fmt.Sprintf(`create table %s.dd_types (
			num_plain number,
			num_prec number(10),
			num_prec_scale number(10,2),
			num_neg_scale number(10,-2),
			float_plain float,
			float_prec float(24),
			varchar_byte varchar2(50 byte),
			varchar_char varchar2(20 char),
			nvarchar2_col nvarchar2(20),
			char_col char(5),
			nchar_col nchar(5),
			clob_col clob,
			nclob_col nclob,
			blob_col blob,
			raw_col raw(16),
			date_col date,
			ts_col timestamp,
			ts_prec_col timestamp(3),
			tstz_col timestamp with time zone,
			tsltz_col timestamp with local time zone,
			iym_col interval year to month,
			ids_col interval day to second,
			bfloat_col binary_float,
			bdouble_col binary_double,
			rowid_col rowid,
			urowid_col urowid,
			xml_col xmltype,
			virt_col as (num_prec_scale * 2),
			invisible_col number invisible,
			not_null_col varchar2(10) not null,
			default_lit varchar2(10) default 'X',
			default_expr date default sysdate
		)`, schemaTestUser),
		fmt.Sprintf("create table %s.dd_long (id number, long_col long)", schemaTestUser),
		fmt.Sprintf("create table %s.dd_longraw (id number, longraw_col long raw)", schemaTestUser),

		fmt.Sprintf("create table %s.dd_index_test (a number, b varchar2(20), c varchar2(20))", schemaTestUser),
		fmt.Sprintf("create unique index %s.dd_idx_unique on %s.dd_index_test (c)", schemaTestUser, schemaTestUser),
		fmt.Sprintf("create index %s.dd_idx_composite on %s.dd_index_test (a, b)", schemaTestUser, schemaTestUser),
		fmt.Sprintf("create index %s.dd_idx_fbi on %s.dd_index_test (upper(b))", schemaTestUser, schemaTestUser),
		fmt.Sprintf("create index %s.dd_idx_fbi_composite on %s.dd_index_test (a, upper(c))", schemaTestUser, schemaTestUser),
		fmt.Sprintf(`create table %s.dd_fk_target (
			k1 number,
			k2 number,
			constraint dd_fk_target_pk primary key (k1, k2)
		)`, schemaTestUser),
		fmt.Sprintf(`create table %s.dd_fk_source (
			id number,
			ref_k1 number,
			ref_k2 number,
			code varchar2(10),
			constraint dd_fk_source_uk unique (code),
			constraint dd_fk_source_chk check (id > 0),
			constraint dd_fk_source_fk foreign key (ref_k1, ref_k2) references %s.dd_fk_target (k1, k2)
		)`, schemaTestUser, schemaTestUser),

		fmt.Sprintf("create view %s.dd_orders_view as select order_id, status from %s.dd_orders", schemaTestUser, schemaTestUser),
		fmt.Sprintf("comment on table %s.dd_orders_view is 'Schema collection view fixture'", schemaTestUser),
		// A nonmatching view verifies that exclude_tables removes only matching views.
		fmt.Sprintf("create view %s.dd_reports_view as select order_id from %s.dd_orders", schemaTestUser, schemaTestUser),
		fmt.Sprintf(`create materialized view %s.dd_orders_mv
			build immediate refresh complete on demand
			as select count(*) as cnt from %s.dd_index_test`, schemaTestUser, schemaTestUser),
		fmt.Sprintf(`create table %s.dd_ext_test (id number, name varchar2(20))
			organization external (
				type oracle_loader
				default directory data_pump_dir
				access parameters (
					records delimited by newline
					fields terminated by ','
				)
				location ('dd_ext_test.csv')
			)
			reject limit unlimited`, schemaTestUser),
		fmt.Sprintf(`create table %s.dd_iot_test (
			id number primary key,
			val varchar2(20)
		) organization index`, schemaTestUser),
		fmt.Sprintf(`create table %s.dd_part_test (
			id number,
			created_at date
		)
		partition by range (created_at) (
			partition p1 values less than (date '2020-01-01'),
			partition p2 values less than (date '2030-01-01')
		)`, schemaTestUser),
	} {
		_, err := sysCheck.db.Exec(stmt)
		require.NoErrorf(t, err, "fixture failed: %s", stmt)
	}

	return setupPDBFixture(t, sysCheck)
}

// ALTER SESSION and subsequent DDL must use the same physical connection.
func setupPDBFixture(t *testing.T, sysCheck Check) string {
	ctx := context.Background()

	var pdb string
	err := sysCheck.db.QueryRowContext(ctx,
		`SELECT name FROM v$pdbs WHERE open_mode = 'READ WRITE' AND name != 'PDB$SEED' ORDER BY con_id FETCH FIRST 1 ROWS ONLY`,
	).Scan(&pdb)
	if err != nil {
		t.Logf("no writable PDB on this instance, skipping multitenancy fixture: %s", err)
		return ""
	}

	conn, err := sysCheck.db.Conn(ctx)
	require.NoError(t, err)
	defer conn.Close()

	for _, stmt := range []string{
		fmt.Sprintf("alter session set container = %s", pdb),
		// Common-user tablespace grants must be repeated in each PDB.
		fmt.Sprintf("grant unlimited tablespace to %s", schemaTestUser),
		fmt.Sprintf(`create table %s.dd_pdb_orders (
			order_id number,
			region varchar2(20)
		)`, schemaTestUser),
		fmt.Sprintf("comment on table %s.dd_pdb_orders is 'PDB fixture'", schemaTestUser),
		"alter session set container = cdb$root",
	} {
		_, err := conn.ExecContext(ctx, stmt)
		require.NoErrorf(t, err, "PDB fixture failed: %s", stmt)
	}

	return pdb
}

func collectSchemaEvents(t *testing.T) []schemaEvent {
	return collectSchemaEventsWithConfig(t, "schemas:\n  enabled: true\n  collection_interval: 1")
}

func collectSchemaEventsWithConfig(t *testing.T, schemasConfig string) []schemaEvent {
	c, sender := newDefaultCheck(t, schemasConfig, "")
	defer c.Teardown()
	require.NoError(t, c.init())
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
		events = append(events, e)
	}
	return events
}

func tableEvents(events []schemaEvent) []schemaEvent {
	var out []schemaEvent
	for _, e := range events {
		if e.Kind == "oracle_databases" {
			out = append(out, e)
		}
	}
	return out
}

func viewEvents(events []schemaEvent) []schemaEvent {
	var out []schemaEvent
	for _, e := range events {
		if e.Kind == "oracle_views" {
			out = append(out, e)
		}
	}
	return out
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

func findTableInContainer(events []schemaEvent, nameSubstr, owner, name string) *schemaTable {
	for _, e := range events {
		for _, container := range e.Metadata {
			if !strings.Contains(strings.ToUpper(container.Name), strings.ToUpper(nameSubstr)) {
				continue
			}
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

func findView(events []schemaEvent, owner, name string) *viewObject {
	for _, e := range events {
		for _, container := range e.Metadata {
			for _, schema := range container.Schemas {
				if !strings.EqualFold(schema.Name, owner) {
					continue
				}
				for _, view := range schema.Views {
					if strings.EqualFold(view.Name, name) {
						return view
					}
				}
			}
		}
	}
	return nil
}

func columnMap(cols []schemaColumn) map[string]schemaColumn {
	m := map[string]schemaColumn{}
	for _, col := range cols {
		m[strings.ToUpper(col.Name)] = col
	}
	return m
}

func findIndex(indexes []*indexInfo, name string) *indexInfo {
	for _, idx := range indexes {
		if strings.EqualFold(idx.Name, name) {
			return idx
		}
	}
	return nil
}

func findConstraint(constraints []*constraintInfo, name string) *constraintInfo {
	for _, con := range constraints {
		if strings.EqualFold(con.Name, name) {
			return con
		}
	}
	return nil
}

func TestSchemaCollectionAgainstDatabase(t *testing.T) {
	setupSchemaFixtures(t)

	events := tableEvents(collectSchemaEvents(t))
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

	columns := columnMap(orders.Columns)
	require.Contains(t, columns, "ORDER_ID")
	assert.Equal(t, "NUMBER(12,0)", columns["ORDER_ID"].DataType)
	assert.False(t, columns["ORDER_ID"].Nullable)

	// Character semantics use CHAR_LENGTH, not byte-oriented DATA_LENGTH.
	require.Contains(t, columns, "STATUS")
	assert.Equal(t, "VARCHAR2(20 CHAR)", columns["STATUS"].DataType)

	require.Contains(t, columns, "AMOUNT")
	assert.Equal(t, "NUMBER(10,2)", columns["AMOUNT"].DataType)
	assert.Equal(t, "Order total", columns["AMOUNT"].Comment)

	require.Contains(t, columns, "CREATED_AT")
	assert.Equal(t, "TIMESTAMP(6)", columns["CREATED_AT"].DataType,
		"temporal precision is already inside DATA_TYPE and must not be appended twice")

	pk := findConstraint(orders.Constraints, "dd_orders_pk")
	require.NotNil(t, pk, "primary key was not collected")
	assert.Equal(t, "primary_key", pk.Type)
	assert.Equal(t, []string{"ORDER_ID"}, pk.Columns)

	statusIdx := findIndex(orders.Indexes, "dd_orders_status_idx")
	require.NotNil(t, statusIdx, "index was not collected")
	assert.Equal(t, columnParts("STATUS"), statusIdx.Columns)
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

	addressColumns := columnMap(addresses.Columns)
	require.Contains(t, addressColumns, "STREET")
	require.Contains(t, addressColumns, "CITY")
	assert.NotContains(t, addressColumns, "SYS_NC_OID$",
		"the hidden object-identifier column must stay excluded, same as for ordinary tables")
	assert.NotContains(t, addressColumns, "SYS_NC_ROWINFO$")
}

func TestSchemaCollectionColumnDataTypesAndAttributes(t *testing.T) {
	setupSchemaFixtures(t)
	events := tableEvents(collectSchemaEvents(t))

	types := findTable(events, schemaTestUser, "dd_types")
	require.NotNil(t, types, "the dd_types fixture table was not collected")
	columns := columnMap(types.Columns)

	dataTypes := map[string]string{
		"NUM_PLAIN":      "NUMBER",
		"NUM_PREC":       "NUMBER(10,0)",
		"NUM_PREC_SCALE": "NUMBER(10,2)",
		"NUM_NEG_SCALE":  "NUMBER(10,-2)",
		// Oracle reports bare FLOAT as DATA_PRECISION=126.
		"FLOAT_PLAIN":   "FLOAT(126)",
		"FLOAT_PREC":    "FLOAT(24)",
		"VARCHAR_BYTE":  "VARCHAR2(50 BYTE)",
		"VARCHAR_CHAR":  "VARCHAR2(20 CHAR)",
		"NVARCHAR2_COL": "NVARCHAR2(20)",
		"CHAR_COL":      "CHAR(5 BYTE)",
		"NCHAR_COL":     "NCHAR(5)",
		"CLOB_COL":      "CLOB",
		"NCLOB_COL":     "NCLOB",
		"BLOB_COL":      "BLOB",
		"RAW_COL":       "RAW(16)",
		"DATE_COL":      "DATE",
		"TS_COL":        "TIMESTAMP(6)",
		"TS_PREC_COL":   "TIMESTAMP(3)",
		"TSTZ_COL":      "TIMESTAMP(6) WITH TIME ZONE",
		"TSLTZ_COL":     "TIMESTAMP(6) WITH LOCAL TIME ZONE",
		"IYM_COL":       "INTERVAL YEAR(2) TO MONTH",
		"IDS_COL":       "INTERVAL DAY(2) TO SECOND(6)",
		"BFLOAT_COL":    "BINARY_FLOAT",
		"BDOUBLE_COL":   "BINARY_DOUBLE",
		"ROWID_COL":     "ROWID",
		"UROWID_COL":    "UROWID",
		"XML_COL":       "XMLTYPE",
		"VIRT_COL":      "NUMBER",
		"INVISIBLE_COL": "NUMBER",
		"NOT_NULL_COL":  "VARCHAR2(10 BYTE)",
		"DEFAULT_LIT":   "VARCHAR2(10 BYTE)",
		"DEFAULT_EXPR":  "DATE",
	}
	for name, want := range dataTypes {
		require.Contains(t, columns, name, "column %s was not collected", name)
		assert.Equal(t, want, columns[name].DataType, "column %s", name)
	}

	assert.True(t, columns["VIRT_COL"].Virtual, "a virtual column must be flagged as such")
	assert.True(t, columns["INVISIBLE_COL"].Invisible, "an invisible column must be flagged as such")
	assert.False(t, columns["NOT_NULL_COL"].Nullable)
	assert.True(t, columns["NUM_PLAIN"].Nullable)

	assert.Equal(t, "'X'", strings.TrimSpace(columns["DEFAULT_LIT"].Default))
	assert.Equal(t, "sysdate", strings.TrimSpace(columns["DEFAULT_EXPR"].Default))

	objectCol := findTable(events, schemaTestUser, "dd_object_col")
	require.NotNil(t, objectCol, "the object-type-column fixture table was not collected")
	objectColColumns := columnMap(objectCol.Columns)
	require.Contains(t, objectColColumns, "ADDR")
	assert.Equal(t, strings.ToUpper(schemaTestUser)+".DD_ADDRESS_T", strings.ToUpper(objectColColumns["ADDR"].DataType),
		"a user-defined object type column must be qualified with its owner")

	long := findTable(events, schemaTestUser, "dd_long")
	require.NotNil(t, long, "the LONG fixture table was not collected")
	longColumns := columnMap(long.Columns)
	require.Contains(t, longColumns, "LONG_COL")
	assert.Equal(t, "LONG", longColumns["LONG_COL"].DataType)

	longRaw := findTable(events, schemaTestUser, "dd_longraw")
	require.NotNil(t, longRaw, "the LONG RAW fixture table was not collected")
	longRawColumns := columnMap(longRaw.Columns)
	require.Contains(t, longRawColumns, "LONGRAW_COL")
	assert.Equal(t, "LONG RAW", longRawColumns["LONGRAW_COL"].DataType)
}

func TestSchemaCollectionIndexesAndConstraints(t *testing.T) {
	setupSchemaFixtures(t)
	events := tableEvents(collectSchemaEvents(t))

	indexTable := findTable(events, schemaTestUser, "dd_index_test")
	require.NotNil(t, indexTable, "the dd_index_test fixture table was not collected")

	unique := findIndex(indexTable.Indexes, "dd_idx_unique")
	require.NotNil(t, unique, "unique index was not collected")
	assert.True(t, unique.Unique)
	assert.Equal(t, columnParts("C"), unique.Columns)

	composite := findIndex(indexTable.Indexes, "dd_idx_composite")
	require.NotNil(t, composite, "composite index was not collected")
	assert.False(t, composite.Unique)
	assert.Equal(t, columnParts("A", "B"), composite.Columns)

	source := findTable(events, schemaTestUser, "dd_fk_source")
	require.NotNil(t, source, "the dd_fk_source fixture table was not collected")

	uk := findConstraint(source.Constraints, "dd_fk_source_uk")
	require.NotNil(t, uk, "unique constraint was not collected")
	assert.Equal(t, "unique", uk.Type)
	assert.Equal(t, []string{"CODE"}, uk.Columns)

	chk := findConstraint(source.Constraints, "dd_fk_source_chk")
	require.NotNil(t, chk, "check constraint was not collected")
	assert.Equal(t, "check", chk.Type)
	assert.Contains(t, strings.ToUpper(chk.Condition), "ID")
	assert.Contains(t, chk.Condition, ">")

	fk := findConstraint(source.Constraints, "dd_fk_source_fk")
	require.NotNil(t, fk, "two-column foreign key was not collected")
	assert.Equal(t, "foreign_key", fk.Type)
	assert.Equal(t, []string{"REF_K1", "REF_K2"}, fk.Columns)
	assert.Equal(t, "DD_FK_TARGET", strings.ToUpper(fk.ReferencedTable))
	assert.Equal(t, []string{"K1", "K2"}, fk.ReferencedColumns)
	assert.Equal(t, strings.ToUpper(schemaTestUser), strings.ToUpper(fk.ReferencedOwner))
}

func TestSchemaCollectionFunctionBasedIndexColumnsReportExpression(t *testing.T) {
	setupSchemaFixtures(t)
	events := tableEvents(collectSchemaEvents(t))

	indexTable := findTable(events, schemaTestUser, "dd_index_test")
	require.NotNil(t, indexTable, "the dd_index_test fixture table was not collected")

	singleFBI := findIndex(indexTable.Indexes, "dd_idx_fbi")
	require.NotNil(t, singleFBI, "a single-column function-based index must not be dropped")
	require.Len(t, singleFBI.Columns, 1)
	assert.Empty(t, singleFBI.Columns[0].Column, "an expression entry must not also carry a column name")
	assert.Equal(t, `UPPER("B")`, strings.ToUpper(singleFBI.Columns[0].Expression))

	compositeFBI := findIndex(indexTable.Indexes, "dd_idx_fbi_composite")
	require.NotNil(t, compositeFBI)
	require.Len(t, compositeFBI.Columns, 2, "the plain column and the expression column must both survive")
	assert.Equal(t, "A", strings.ToUpper(compositeFBI.Columns[0].Column))
	assert.Empty(t, compositeFBI.Columns[0].Expression)
	assert.Empty(t, compositeFBI.Columns[1].Column, "an expression entry must not also carry a column name")
	assert.Equal(t, `UPPER("C")`, strings.ToUpper(compositeFBI.Columns[1].Expression))
}

func TestSchemaCollectionRelationKinds(t *testing.T) {
	setupSchemaFixtures(t)
	events := tableEvents(collectSchemaEvents(t))

	mv := findTable(events, schemaTestUser, "dd_orders_mv")
	require.NotNil(t, mv, "the materialized view's backing table was not collected")
	require.NotNil(t, mv.Mview, "materialized view details were not collected")
	assert.Equal(t, "COMPLETE", mv.Mview.RefreshMethod)

	ext := findTable(events, schemaTestUser, "dd_ext_test")
	require.NotNil(t, ext, "the external table was not collected")
	assert.Equal(t, "external", ext.TableType)
	require.NotNil(t, ext.External)
	assert.Equal(t, "DATA_PUMP_DIR", strings.ToUpper(ext.External.Directory))
	require.Len(t, ext.External.Locations, 1)
	assert.Equal(t, "DATA_PUMP_DIR:DD_EXT_TEST.CSV", strings.ToUpper(ext.External.Locations[0]))

	iot := findTable(events, schemaTestUser, "dd_iot_test")
	require.NotNil(t, iot, "the index-organized table was not collected")
	assert.Contains(t, iot.Properties, "index_organized")

	part := findTable(events, schemaTestUser, "dd_part_test")
	require.NotNil(t, part, "the partitioned table was not collected")
	assert.Contains(t, part.Properties, "partitioned")
	require.NotNil(t, part.Partitioned)
	assert.Equal(t, "RANGE", part.Partitioned.PartitioningType)
	assert.EqualValues(t, 2, part.Partitioned.NumPartitions)
	assert.Equal(t, "RANGE (CREATED_AT)", strings.ToUpper(part.Partitioned.PartitionKey))
}

func TestSchemaCollectionViews(t *testing.T) {
	setupSchemaFixtures(t)
	all := collectSchemaEvents(t)

	views := viewEvents(all)
	require.NotEmpty(t, views, "no oracle_views payload was emitted")

	view := findView(views, schemaTestUser, "dd_orders_view")
	require.NotNil(t, view, "the fixture view was not collected")
	assert.Equal(t, "Schema collection view fixture", view.Comment)
	assert.Contains(t, strings.ToUpper(view.Definition), "DD_ORDERS")
	assert.NotEmpty(t, view.ID)

	columns := columnMap(view.Columns)
	require.Contains(t, columns, "ORDER_ID")
	require.Contains(t, columns, "STATUS")
}

func TestSchemaCollectionViewsRespectTableFilters(t *testing.T) {
	setupSchemaFixtures(t)

	events := collectSchemaEventsWithConfig(t,
		"schemas:\n  enabled: true\n  collection_interval: 1\n  exclude_tables:\n    - \"^DD_ORDERS_VIEW$\"\n")

	views := viewEvents(events)
	require.NotEmpty(t, views, "no oracle_views payload was emitted")

	assert.Nil(t, findView(views, schemaTestUser, "dd_orders_view"),
		"a view matching exclude_tables must be filtered out of the oracle_views payload")
	assert.NotNil(t, findView(views, schemaTestUser, "dd_reports_view"),
		"a view not matching exclude_tables must still be collected")

	for _, e := range views {
		assert.False(t, e.Truncated,
			"a view removed by exclude_tables must not be reported as a max_views truncation")
	}
}

func TestSchemaCollectionMultitenancy(t *testing.T) {
	pdb := setupSchemaFixtures(t)
	if pdb == "" {
		t.Skip("no writable PDB available on this instance")
	}
	events := tableEvents(collectSchemaEvents(t))

	pdbOrders := findTableInContainer(events, pdb, schemaTestUser, "dd_pdb_orders")
	require.NotNil(t, pdbOrders, "the PDB fixture table was not collected under its container")
	assert.Equal(t, "PDB fixture", pdbOrders.Comment)

	root := findTable(events, schemaTestUser, "dd_orders")
	require.NotNil(t, root, "the root-container fixture table must still be collected alongside the PDB one")
}

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
