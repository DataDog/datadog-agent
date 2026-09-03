// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build oracle_test

package oracle

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/benbjohnson/clock"
	"github.com/jmoiron/sqlx"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

// newSchemaCheck returns a check wired to a mock DB with schema collection enabled.
func newSchemaCheck(t *testing.T) (Check, *sqlx.DB, sqlmock.Sqlmock, func()) {
	db, dbMock, err := sqlmock.New()
	require.NoError(t, err)

	c, _ := newDbDoesNotExistCheck(t, "", "")
	sqlxDB := sqlx.NewDb(db, "sqlmock")
	c.db = sqlxDB
	c.clock = clock.NewMock()
	c.dbVersion = "23.0.0.0.0"
	c.multitenant = true
	c.config.Schemas.Enabled = true
	c.config.Schemas.CollectionInterval = 600
	c.config.Schemas.PayloadChunkSize = 2

	return c, sqlxDB, dbMock, func() { db.Close() }
}

// columnParts builds the ordinary, all-real-columns case of an index's key for assertions.
func columnParts(names ...string) []indexKeyPart {
	parts := make([]indexKeyPart, len(names))
	for i, n := range names {
		parts[i] = indexKeyPart{Column: n}
	}
	return parts
}

func TestSchemaCollectionNoOwnersSkips(t *testing.T) {
	c, _, dbMock, closeDB := newSchemaCheck(t)
	defer closeDB()

	dbMock.ExpectQuery(`v\$containers`).WillReturnRows(sqlmock.NewRows([]string{"CON_ID", "NAME"}))
	dbMock.ExpectQuery("cdb_users").WillReturnRows(sqlmock.NewRows([]string{"CON_ID", "USERNAME", "USER_ID"}))

	require.NoError(t, c.SchemaCollection())
	assert.NoError(t, dbMock.ExpectationsWereMet())
}

func TestDataTypeRendering(t *testing.T) {
	cases := []struct {
		name string
		row  schemaRowDB
		want string
	}{
		{
			name: "char semantics use CHAR_LENGTH not DATA_LENGTH",
			row: schemaRowDB{
				DataType:   sql.NullString{String: "VARCHAR2", Valid: true},
				DataLength: sql.NullInt64{Int64: 80, Valid: true},
				CharLength: sql.NullInt64{Int64: 20, Valid: true},
				CharUsed:   "C",
			},
			want: "VARCHAR2(20 CHAR)",
		},
		{
			name: "byte semantics use DATA_LENGTH",
			row: schemaRowDB{
				DataType:   sql.NullString{String: "VARCHAR2", Valid: true},
				DataLength: sql.NullInt64{Int64: 50, Valid: true},
				CharLength: sql.NullInt64{Int64: 50, Valid: true},
				CharUsed:   "B",
			},
			want: "VARCHAR2(50 BYTE)",
		},
		{
			name: "national types carry no qualifier",
			row: schemaRowDB{
				DataType:   sql.NullString{String: "NVARCHAR2", Valid: true},
				DataLength: sql.NullInt64{Int64: 40, Valid: true},
				CharLength: sql.NullInt64{Int64: 20, Valid: true},
				CharUsed:   "C",
			},
			want: "NVARCHAR2(20)",
		},
		{
			name: "number with precision and scale",
			row: schemaRowDB{
				DataType:      sql.NullString{String: "NUMBER", Valid: true},
				DataPrecision: sql.NullInt64{Int64: 12, Valid: true},
				DataScale:     sql.NullInt64{Int64: 2, Valid: true},
			},
			want: "NUMBER(12,2)",
		},
		{
			name: "unconstrained number keeps no precision",
			row:  schemaRowDB{DataType: sql.NullString{String: "NUMBER", Valid: true}},
			want: "NUMBER",
		},
		{
			name: "timestamp precision already lives in DATA_TYPE",
			row: schemaRowDB{
				DataType:   sql.NullString{String: "TIMESTAMP(6)", Valid: true},
				DataLength: sql.NullInt64{Int64: 11, Valid: true},
			},
			want: "TIMESTAMP(6)",
		},
		{
			name: "LOB length is a locator size and must not be rendered",
			row: schemaRowDB{
				DataType:   sql.NullString{String: "CLOB", Valid: true},
				DataLength: sql.NullInt64{Int64: 4000, Valid: true},
			},
			want: "CLOB",
		},
		{
			name: "user defined type is owner qualified",
			row: schemaRowDB{
				DataType:      sql.NullString{String: "ADDRESS_T", Valid: true},
				DataTypeOwner: sql.NullString{String: "DEMO_APP", Valid: true},
			},
			want: "DEMO_APP.ADDRESS_T",
		},
		{
			name: "SYS owned types are not qualified",
			row: schemaRowDB{
				DataType:      sql.NullString{String: "XMLTYPE", Valid: true},
				DataTypeOwner: sql.NullString{String: "SYS", Valid: true},
			},
			want: "XMLTYPE",
		},
		{
			name: "REF columns keep their modifier",
			row: schemaRowDB{
				DataType:      sql.NullString{String: "EMPLOYEE_T", Valid: true},
				DataTypeOwner: sql.NullString{String: "HR", Valid: true},
				DataTypeMod:   sql.NullString{String: "REF", Valid: true},
			},
			want: "REF HR.EMPLOYEE_T",
		},
		{
			name: "FLOAT with precision",
			row: schemaRowDB{
				DataType:      sql.NullString{String: "FLOAT", Valid: true},
				DataPrecision: sql.NullInt64{Int64: 126, Valid: true},
			},
			want: "FLOAT(126)",
		},
		{
			name: "unconstrained FLOAT keeps no precision",
			row:  schemaRowDB{DataType: sql.NullString{String: "FLOAT", Valid: true}},
			want: "FLOAT",
		},
		{
			name: "RAW uses DATA_LENGTH, there is no character semantic for it",
			row: schemaRowDB{
				DataType:   sql.NullString{String: "RAW", Valid: true},
				DataLength: sql.NullInt64{Int64: 16, Valid: true},
			},
			want: "RAW(16)",
		},
		{
			name: "CHAR follows the same char/byte semantics as VARCHAR2",
			row: schemaRowDB{
				DataType:   sql.NullString{String: "CHAR", Valid: true},
				DataLength: sql.NullInt64{Int64: 4, Valid: true},
				CharLength: sql.NullInt64{Int64: 1, Valid: true},
				CharUsed:   "C",
			},
			want: "CHAR(1 CHAR)",
		},
		{
			// CHAR_USED is NVL-defaulted to '-' for columns where it does not apply (e.g.
			// non-character types), but VARCHAR2/CHAR can themselves carry that default when
			// the column predates length-semantics tracking; the renderer must still fall back
			// to the DATA_LENGTH/byte branch rather than mishandling it as CHAR semantics.
			name: "CHAR_USED default '-' on a character column falls back to byte semantics",
			row: schemaRowDB{
				DataType:   sql.NullString{String: "VARCHAR2", Valid: true},
				DataLength: sql.NullInt64{Int64: 30, Valid: true},
				CharLength: sql.NullInt64{Int64: 30, Valid: true},
				CharUsed:   "-",
			},
			want: "VARCHAR2(30 BYTE)",
		},
		{
			name: "LONG carries no length and renders as a bare type",
			row: schemaRowDB{
				DataType:   sql.NullString{String: "LONG", Valid: true},
				DataLength: sql.NullInt64{Int64: 4000, Valid: true},
			},
			want: "LONG",
		},
		{
			name: "LONG RAW carries no length and renders as a bare type",
			row: schemaRowDB{
				DataType:   sql.NullString{String: "LONG RAW", Valid: true},
				DataLength: sql.NullInt64{Int64: 4000, Valid: true},
			},
			want: "LONG RAW",
		},
		{
			name: "NUMBER with negative scale rounds, it is not a decimal count",
			row: schemaRowDB{
				DataType:      sql.NullString{String: "NUMBER", Valid: true},
				DataPrecision: sql.NullInt64{Int64: 10, Valid: true},
				DataScale:     sql.NullInt64{Int64: -5, Valid: true},
			},
			want: "NUMBER(10,-5)",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, dataType(tc.row))
		})
	}
}

func TestTableTypeAndProperties(t *testing.T) {
	external := schemaRowDB{External: "YES", IotType: "-", ClusterName: "-"}
	assert.Equal(t, "external", tableType(external))

	heap := schemaRowDB{External: "NO", IotType: "-", ClusterName: "-"}
	assert.Equal(t, "table", tableType(heap))

	compound := schemaRowDB{
		External: "NO", Temporary: "Y", Partitioned: "YES", IotType: "IOT",
		ClusterName: "ORD_CLUSTER", Clustering: "YES", ReadOnly: "YES",
	}
	assert.Equal(t,
		[]string{"temporary", "partitioned", "index_organized", "clustered", "attribute_clustered", "read_only"},
		tableProperties(compound),
		"Oracle attributes compound, so every applicable flag must be present")

	assert.Empty(t, tableProperties(heap))

	objectTable := schemaRowDB{
		External: "NO", IotType: "-", ClusterName: "-",
		ObjectTypeOwner: "DEMO_APP", ObjectType: "ADDRESS_T",
	}
	assert.Equal(t, "table", tableType(objectTable),
		"an object table is still a table, distinguished only by its properties")
	assert.Equal(t, []string{"object_table"}, tableProperties(objectTable))
}

func TestDefaultValueColumnIsVersionGated(t *testing.T) {
	c, _, _, closeDB := newSchemaCheck(t)
	defer closeDB()

	c.dbVersion = "23.26.2.0.0"
	assert.Equal(t, "c.data_default_vc", c.defaultValueColumn())

	// DATA_DEFAULT_VC does not exist before 23ai; every earlier version falls back to the LONG
	// column, truncated in Go rather than in SQL (see truncateLongValue).
	for _, v := range []string{"19.21.0.0.0", "21.3.0.0.0", "12.2.0.1.0"} {
		c.dbVersion = v
		assert.Equal(t, "c.data_default", c.defaultValueColumn(), "version %s", v)
	}
}

func TestSnapshotChunking(t *testing.T) {
	c, _, _, closeDB := newSchemaCheck(t)
	defer closeDB()
	c.config.Schemas.PayloadChunkSize = 2

	var payloads []schemaEvent
	collector := newSchemaCollector(&c, func(b []byte) {
		var e schemaEvent
		require.NoError(t, json.Unmarshal(b, &e))
		payloads = append(payloads, e)
	}, map[tableKey]*tableDetails{}, map[ownerKey]string{}, map[int64]string{})

	// Five tables in one container: chunk size 2 means 2 + 2 + 1.
	for _, name := range []string{"T1", "T2", "T3", "T4", "T5"} {
		collector.add(schemaRowDB{
			ConID: 3, Owner: "APP", TableName: name, Temporary: "N", External: "NO",
			IotType: "-", ClusterName: "-", Partitioned: "NO",
			ColumnName: "C1", DataType: sql.NullString{String: "NUMBER", Valid: true},
			Nullable: "Y",
		})
	}
	collector.finish()

	require.Len(t, payloads, 3, "5 tables at chunk size 2 must split into 3 payloads")

	for i, p := range payloads {
		assert.Equal(t, payloads[0].CollectionStartedAt, p.CollectionStartedAt,
			"payload %d must carry the snapshot id", i)
	}
	assert.Zero(t, payloads[0].CollectionPayloadsCount, "only the last payload is terminating")
	assert.Zero(t, payloads[1].CollectionPayloadsCount)
	assert.Equal(t, 3, payloads[2].CollectionPayloadsCount,
		"the terminating payload must declare how many payloads the snapshot has")
}

func TestSnapshotPerContainer(t *testing.T) {
	c, _, _, closeDB := newSchemaCheck(t)
	defer closeDB()
	c.config.Schemas.PayloadChunkSize = 100

	var payloads []schemaEvent
	collector := newSchemaCollector(&c, func(b []byte) {
		var e schemaEvent
		require.NoError(t, json.Unmarshal(b, &e))
		payloads = append(payloads, e)
	}, map[tableKey]*tableDetails{}, map[ownerKey]string{}, map[int64]string{})

	for _, conID := range []int64{1, 1, 3} {
		collector.add(schemaRowDB{
			ConID: conID, Owner: "APP", TableName: "T", Temporary: "N", External: "NO",
			IotType: "-", ClusterName: "-", Partitioned: "NO",
			ColumnName: "C1", DataType: sql.NullString{String: "NUMBER", Valid: true},
			Nullable: "Y",
		})
	}
	collector.finish()

	require.Len(t, payloads, 2, "each container is its own snapshot")
	assert.Equal(t, "1", payloads[0].Metadata[0].ID)
	assert.Equal(t, "3", payloads[1].Metadata[0].ID)
	for _, p := range payloads {
		assert.Equal(t, 1, p.CollectionPayloadsCount, "each container terminates its own snapshot")
	}

	// The mock clock never advances, which is what a real instance does when two small
	// containers are collected inside the same millisecond.
	assert.NotEqual(t, payloads[0].CollectionStartedAt, payloads[1].CollectionStartedAt,
		"containers must not share a snapshot identifier")
}

func TestRowCountEstimateCombinesStatsAndDeltas(t *testing.T) {
	c, _, _, closeDB := newSchemaCheck(t)
	defer closeDB()

	details := map[tableKey]*tableDetails{
		{conID: 1, owner: "APP", table: "T"}: {
			Modifications: &modificationsDetail{Inserts: 30, Deletes: 5},
		},
	}

	var payloads []schemaEvent
	collector := newSchemaCollector(&c, func(b []byte) {
		var e schemaEvent
		require.NoError(t, json.Unmarshal(b, &e))
		payloads = append(payloads, e)
	}, details, map[ownerKey]string{}, map[int64]string{})

	collector.add(schemaRowDB{
		ConID: 1, Owner: "APP", TableName: "T", Temporary: "N", External: "NO",
		IotType: "-", ClusterName: "-", Partitioned: "NO",
		NumRows:    sql.NullInt64{Int64: 100, Valid: true},
		ColumnName: "C1", DataType: sql.NullString{String: "NUMBER", Valid: true}, Nullable: "Y",
	})
	collector.finish()

	require.Len(t, payloads, 1)
	table := payloads[0].Metadata[0].Schemas[0].Tables[0]
	require.NotNil(t, table.RowCount)
	assert.Equal(t, int64(125), *table.RowCount, "NUM_ROWS plus inserts minus deletes")
}

// TestObjectTableDetailIsSurfaced covers a row coming from the cdb_object_tables branch of the
// query: the object type backing the table must show up both as a property flag and as a typed
// detail on the table.
func TestObjectTableDetailIsSurfaced(t *testing.T) {
	c, _, _, closeDB := newSchemaCheck(t)
	defer closeDB()

	var payloads []schemaEvent
	collector := newSchemaCollector(&c, func(b []byte) {
		var e schemaEvent
		require.NoError(t, json.Unmarshal(b, &e))
		payloads = append(payloads, e)
	}, map[tableKey]*tableDetails{}, map[ownerKey]string{}, map[int64]string{})

	collector.add(schemaRowDB{
		ConID: 3, Owner: "DEMO_APP", TableName: "ADDRESSES", Temporary: "N", External: "NO",
		IotType: "-", ClusterName: "-", Partitioned: "NO",
		ObjectTypeOwner: "DEMO_APP", ObjectType: "ADDRESS_T",
		ColumnName: "STREET", DataType: sql.NullString{String: "VARCHAR2", Valid: true},
		DataLength: sql.NullInt64{Int64: 100, Valid: true}, CharLength: sql.NullInt64{Int64: 100, Valid: true},
		CharUsed: "C", Nullable: "Y",
	})
	collector.finish()

	require.Len(t, payloads, 1)
	table := payloads[0].Metadata[0].Schemas[0].Tables[0]
	assert.Equal(t, "table", table.TableType)
	assert.Contains(t, table.Properties, "object_table")
	require.NotNil(t, table.ObjectType)
	assert.Equal(t, "DEMO_APP", table.ObjectType.TypeOwner)
	assert.Equal(t, "ADDRESS_T", table.ObjectType.TypeName)
}

// TestSchemaCollectionEmitsOnDbmMetadata covers the wiring SchemaCollection owns: the owners
// query, the main query, and the event actually reaching the sender on the dbm-metadata track.
// Detail queries are left unprimed on purpose -- sqlmock rejects them, which is the same shape
// as a missing grant, and collection must still produce a payload.
func TestSchemaCollectionEmitsOnDbmMetadata(t *testing.T) {
	db, dbMock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()
	dbMock.MatchExpectationsInOrder(false)

	c, sender := newDbDoesNotExistCheck(t, "", "")
	c.db = sqlx.NewDb(db, "sqlmock")
	mockClock := clock.NewMock()
	mockClock.Set(time.Unix(1787000000, 0))
	c.clock = mockClock
	c.dbVersion = "23.26.2.0.0"
	c.config.Schemas.Enabled = true
	c.config.Schemas.CollectionInterval = 600
	c.config.Schemas.PayloadChunkSize = 100

	dbMock.ExpectQuery(`v\$containers`).WillReturnRows(
		sqlmock.NewRows([]string{"CON_ID", "NAME"}).AddRow(3, "APP_PDB"))
	dbMock.ExpectQuery("cdb_users").WillReturnRows(
		sqlmock.NewRows([]string{"CON_ID", "USERNAME", "USER_ID"}).AddRow(3, "APP", 104))

	mainRows := sqlmock.NewRows([]string{
		"CON_ID", "OWNER", "TABLE_NAME", "TEMPORARY", "DURATION", "EXTERNAL", "IOT_TYPE",
		"PARTITIONED", "CLUSTER_NAME", "CLUSTERING", "READ_ONLY", "NUM_ROWS", "LAST_ANALYZED",
		"OBJECT_TYPE_OWNER", "OBJECT_TYPE",
		"COLUMN_NAME", "COLUMN_ID", "INTERNAL_COLUMN_ID", "VIRTUAL_COLUMN", "HIDDEN_COLUMN", "DATA_TYPE",
		"DATA_TYPE_OWNER", "DATA_TYPE_MOD", "DATA_LENGTH", "CHAR_LENGTH", "DATA_PRECISION",
		"DATA_SCALE", "CHAR_USED", "NULLABLE", "DATA_DEFAULT_VC",
	}).AddRow(
		3, "APP", "ORDERS", "N", "-", "NO", "-", "NO", "-", "NO", "NO", nil, nil, "-", "-",
		"ORDER_ID", 1, 1, "NO", "NO", "NUMBER", nil, nil, 22, nil, 12, 0, "-", "N", nil,
	)
	dbMock.ExpectQuery("cdb_tab_cols").WillReturnRows(mainRows)

	require.NoError(t, c.SchemaCollection())

	sender.AssertNumberOfCalls(t, "EventPlatformEvent", 1)
	sender.AssertCalled(t, "EventPlatformEvent", mock.Anything, "dbm-metadata")
	sender.AssertNumberOfCalls(t, "Commit", 1)

	// The emitted payload must be a complete, self-describing snapshot.
	call := sender.Calls[0]
	for _, c := range sender.Calls {
		if c.Method == "EventPlatformEvent" {
			call = c
			break
		}
	}
	var event schemaEvent
	require.NoError(t, json.Unmarshal(call.Arguments.Get(0).([]byte), &event))
	assert.Equal(t, "oracle_databases", event.Kind)
	assert.Equal(t, "oracle", event.Dbms)
	assert.Equal(t, int64(1787000000000), event.CollectionStartedAt,
		"the snapshot id is the collection start in epoch milliseconds")
	assert.Equal(t, 1, event.CollectionPayloadsCount)
	require.Len(t, event.Metadata, 1)
	assert.Equal(t, "3", event.Metadata[0].ID)
	require.Len(t, event.Metadata[0].Schemas, 1)
	assert.Equal(t, "104", event.Metadata[0].Schemas[0].ID)
	require.Len(t, event.Metadata[0].Schemas[0].Tables, 1)
	table := event.Metadata[0].Schemas[0].Tables[0]
	assert.Equal(t, "ORDERS", table.Name)
	assert.Equal(t, "table", table.TableType)
	require.Len(t, table.Columns, 1)
	assert.Equal(t, "NUMBER(12,0)", table.Columns[0].DataType)
}

// TestContainerNamesUsePdbName pins the container naming: v$containers supplies the PDB name,
// and only a container missing from that map falls back to the con_id.
func TestContainerNamesUsePdbName(t *testing.T) {
	c, _, _, closeDB := newSchemaCheck(t)
	defer closeDB()
	c.cdbName = "free"
	c.config.Schemas.PayloadChunkSize = 100

	var payloads []schemaEvent
	collector := newSchemaCollector(&c, func(b []byte) {
		var e schemaEvent
		require.NoError(t, json.Unmarshal(b, &e))
		payloads = append(payloads, e)
	}, map[tableKey]*tableDetails{}, map[ownerKey]string{}, map[int64]string{1: "CDB$ROOT", 3: "FREEPDB1"})

	for _, conID := range []int64{1, 3, 7} {
		collector.add(schemaRowDB{
			ConID: conID, Owner: "APP", TableName: "T", Temporary: "N", External: "NO",
			IotType: "-", ClusterName: "-", Partitioned: "NO",
			ColumnName: "C1", DataType: sql.NullString{String: "NUMBER", Valid: true}, Nullable: "Y",
		})
	}
	collector.finish()

	require.Len(t, payloads, 3)
	assert.Equal(t, "free.CDB$ROOT", payloads[0].Metadata[0].Name)
	assert.Equal(t, "free.FREEPDB1", payloads[1].Metadata[0].Name)
	assert.Equal(t, "free.7", payloads[2].Metadata[0].Name, "unknown container falls back to con_id")
}

// TestViewCollectionEmitsSeparateKind covers the view path: views travel as their own kind so
// the backend can treat them separately, and they must not be mixed into the tables payload.
func TestViewCollectionEmitsSeparateKind(t *testing.T) {
	db, dbMock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()
	dbMock.MatchExpectationsInOrder(false)

	c, sender := newDbDoesNotExistCheck(t, "", "")
	c.db = sqlx.NewDb(db, "sqlmock")
	mockClock := clock.NewMock()
	mockClock.Set(time.Unix(1787000000, 0))
	c.clock = mockClock
	c.dbVersion = "23.26.2.0.0"
	c.config.Schemas.Enabled = true
	c.config.Schemas.CollectionInterval = 600
	c.config.Schemas.PayloadChunkSize = 100

	tableRelationColumns := []string{
		"CON_ID", "OWNER", "TABLE_NAME", "TEMPORARY", "DURATION", "EXTERNAL", "IOT_TYPE",
		"PARTITIONED", "CLUSTER_NAME", "CLUSTERING", "READ_ONLY", "NUM_ROWS", "LAST_ANALYZED",
		"OBJECT_TYPE_OWNER", "OBJECT_TYPE",
		"COLUMN_NAME", "COLUMN_ID", "INTERNAL_COLUMN_ID", "VIRTUAL_COLUMN", "HIDDEN_COLUMN", "DATA_TYPE",
		"DATA_TYPE_OWNER", "DATA_TYPE_MOD", "DATA_LENGTH", "CHAR_LENGTH", "DATA_PRECISION",
		"DATA_SCALE", "CHAR_USED", "NULLABLE", "DATA_DEFAULT_VC",
	}
	// The views query does not have an object-table branch, so it keeps the original shape.
	viewRelationColumns := []string{
		"CON_ID", "OWNER", "TABLE_NAME", "TEMPORARY", "DURATION", "EXTERNAL", "IOT_TYPE",
		"PARTITIONED", "CLUSTER_NAME", "CLUSTERING", "READ_ONLY", "NUM_ROWS", "LAST_ANALYZED",
		"COLUMN_NAME", "COLUMN_ID", "VIRTUAL_COLUMN", "HIDDEN_COLUMN", "DATA_TYPE",
		"DATA_TYPE_OWNER", "DATA_TYPE_MOD", "DATA_LENGTH", "CHAR_LENGTH", "DATA_PRECISION",
		"DATA_SCALE", "CHAR_USED", "NULLABLE", "DATA_DEFAULT_VC",
	}

	dbMock.ExpectQuery(`v\$containers`).WillReturnRows(
		sqlmock.NewRows([]string{"CON_ID", "NAME"}).AddRow(3, "APP_PDB"))
	dbMock.ExpectQuery("cdb_users").WillReturnRows(
		sqlmock.NewRows([]string{"CON_ID", "USERNAME", "USER_ID"}).AddRow(3, "APP", 104))
	dbMock.ExpectQuery("cdb_tab_cols").WillReturnRows(
		sqlmock.NewRows(tableRelationColumns).AddRow(
			3, "APP", "ORDERS", "N", "-", "NO", "-", "NO", "-", "NO", "NO", nil, nil, "-", "-",
			"ORDER_ID", 1, 1, "NO", "NO", "NUMBER", nil, nil, 22, nil, 12, 0, "-", "N", nil,
		))
	dbMock.ExpectQuery("cdb_views").WillReturnRows(
		sqlmock.NewRows(viewRelationColumns).AddRow(
			3, "APP", "V_ORDERS", "N", "-", "NO", "-", "NO", "-", "NO", "NO", nil, nil,
			"ORDER_ID", 1, "NO", "NO", "NUMBER", nil, nil, 22, nil, 12, 0, "-", "N", nil,
		))

	require.NoError(t, c.SchemaCollection())

	byKind := map[string]schemaEvent{}
	for _, call := range sender.Calls {
		if call.Method != "EventPlatformEvent" {
			continue
		}
		var e schemaEvent
		require.NoError(t, json.Unmarshal(call.Arguments.Get(0).([]byte), &e))
		byKind[e.Kind] = e
	}

	require.Contains(t, byKind, "oracle_databases")
	require.Contains(t, byKind, "oracle_views")

	tables := byKind["oracle_databases"]
	require.Len(t, tables.Metadata[0].Schemas, 1)
	assert.Len(t, tables.Metadata[0].Schemas[0].Tables, 1)
	assert.Empty(t, tables.Metadata[0].Schemas[0].Views, "views must not ride along with tables")

	views := byKind["oracle_views"]
	require.Len(t, views.Metadata[0].Schemas, 1)
	require.Len(t, views.Metadata[0].Schemas[0].Views, 1)
	assert.Equal(t, "V_ORDERS", views.Metadata[0].Schemas[0].Views[0].Name)
	assert.Empty(t, views.Metadata[0].Schemas[0].Tables, "a views payload carries no tables")
	assert.NotEqual(t, tables.CollectionStartedAt, views.CollectionStartedAt,
		"tables and views are independent snapshots")
	assert.Equal(t, 1, views.CollectionPayloadsCount)
}

// TestViewCollectionFailureKeepsTables guards the grant that view collection needs and table
// collection does not: losing views must not discard the tables already emitted.
func TestViewCollectionFailureKeepsTables(t *testing.T) {
	db, dbMock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()
	dbMock.MatchExpectationsInOrder(false)

	c, sender := newDbDoesNotExistCheck(t, "", "")
	c.db = sqlx.NewDb(db, "sqlmock")
	c.clock = clock.NewMock()
	c.dbVersion = "23.26.2.0.0"
	c.config.Schemas.Enabled = true
	c.config.Schemas.CollectionInterval = 600
	c.config.Schemas.PayloadChunkSize = 100

	dbMock.ExpectQuery(`v\$containers`).WillReturnRows(
		sqlmock.NewRows([]string{"CON_ID", "NAME"}).AddRow(3, "APP_PDB"))
	dbMock.ExpectQuery("cdb_users").WillReturnRows(
		sqlmock.NewRows([]string{"CON_ID", "USERNAME", "USER_ID"}).AddRow(3, "APP", 104))
	dbMock.ExpectQuery("cdb_tab_cols").WillReturnRows(
		sqlmock.NewRows([]string{
			"CON_ID", "OWNER", "TABLE_NAME", "TEMPORARY", "DURATION", "EXTERNAL", "IOT_TYPE",
			"PARTITIONED", "CLUSTER_NAME", "CLUSTERING", "READ_ONLY", "NUM_ROWS", "LAST_ANALYZED",
			"OBJECT_TYPE_OWNER", "OBJECT_TYPE",
			"COLUMN_NAME", "COLUMN_ID", "INTERNAL_COLUMN_ID", "VIRTUAL_COLUMN", "HIDDEN_COLUMN", "DATA_TYPE",
			"DATA_TYPE_OWNER", "DATA_TYPE_MOD", "DATA_LENGTH", "CHAR_LENGTH", "DATA_PRECISION",
			"DATA_SCALE", "CHAR_USED", "NULLABLE", "DATA_DEFAULT_VC",
		}).AddRow(
			3, "APP", "ORDERS", "N", "-", "NO", "-", "NO", "-", "NO", "NO", nil, nil, "-", "-",
			"ORDER_ID", 1, 1, "NO", "NO", "NUMBER", nil, nil, 22, nil, 12, 0, "-", "N", nil,
		))
	// The views query is left unprimed, which sqlmock rejects the same way a missing grant does.

	require.NoError(t, c.SchemaCollection(), "a missing views grant must not fail collection")

	var kinds []string
	for _, call := range sender.Calls {
		if call.Method != "EventPlatformEvent" {
			continue
		}
		var e schemaEvent
		require.NoError(t, json.Unmarshal(call.Arguments.Get(0).([]byte), &e))
		kinds = append(kinds, e.Kind)
	}
	assert.Equal(t, []string{"oracle_databases"}, kinds)
	sender.AssertNumberOfCalls(t, "Commit", 1)
}

// TestTableDetailsIndexesGroupByName covers the scan closure that appends columns onto the
// last-seen index rather than creating a new indexInfo per row: cdb_ind_columns returns one row
// per index column, ordered by position, and a composite index's columns must land on a single
// indexInfo in that order.
func TestTableDetailsIndexesGroupByName(t *testing.T) {
	c, _, dbMock, closeDB := newSchemaCheck(t)
	defer closeDB()

	dbMock.MatchExpectationsInOrder(false)
	dbMock.ExpectQuery("cdb_indexes").WillReturnRows(
		sqlmock.NewRows([]string{"CON_ID", "TABLE_OWNER", "TABLE_NAME", "INDEX_NAME", "UNIQUENESS", "INDEX_TYPE", "COLUMN_NAME", "COLUMN_EXPRESSION"}).
			AddRow(3, "APP", "ORDERS", "ORDERS_COMPOSITE_IDX", "UNIQUE", "NORMAL", "STATUS", nil).
			AddRow(3, "APP", "ORDERS", "ORDERS_COMPOSITE_IDX", "UNIQUE", "NORMAL", "CREATED_AT", nil).
			AddRow(3, "APP", "ORDERS", "ORDERS_STATUS_IDX", "NONUNIQUE", "NORMAL", "STATUS", nil))

	allowed := map[tableKey]struct{}{{conID: 3, owner: "APP", table: "ORDERS"}: {}}
	details := c.tableDetails(context.Background(), []string{"'APP'"}, allowed)

	d := details[tableKey{conID: 3, owner: "APP", table: "ORDERS"}]
	require.NotNil(t, d)
	require.Len(t, d.Indexes, 2, "two distinct index names must produce two indexInfo entries")

	composite := d.Indexes[0]
	assert.Equal(t, "ORDERS_COMPOSITE_IDX", composite.Name)
	assert.True(t, composite.Unique)
	assert.Equal(t, columnParts("STATUS", "CREATED_AT"), composite.Columns,
		"a composite index's columns must accumulate onto the same indexInfo, in position order")

	single := d.Indexes[1]
	assert.Equal(t, "ORDERS_STATUS_IDX", single.Name)
	assert.False(t, single.Unique)
	assert.Equal(t, columnParts("STATUS"), single.Columns)
}

// TestTableDetailsIndexesFunctionBasedSubstitutesExpression covers the fix for the SYS_NC%
// hidden-column bug: indexesQuery's join to cdb_tab_cols supplies the expression for a
// function-based index's hidden key column, and that expression must be substituted for the
// meaningless generated column name -- for a single-column FBI (which used to vanish entirely)
// and for the function-based half of a composite index (which used to be silently dropped).
func TestTableDetailsIndexesFunctionBasedSubstitutesExpression(t *testing.T) {
	c, _, dbMock, closeDB := newSchemaCheck(t)
	defer closeDB()

	dbMock.MatchExpectationsInOrder(false)
	dbMock.ExpectQuery("cdb_indexes").WillReturnRows(
		sqlmock.NewRows([]string{"CON_ID", "TABLE_OWNER", "TABLE_NAME", "INDEX_NAME", "UNIQUENESS", "INDEX_TYPE", "COLUMN_NAME", "COLUMN_EXPRESSION"}).
			AddRow(3, "APP", "ORDERS", "ORDERS_FBI_IDX", "NONUNIQUE", "FUNCTION-BASED NORMAL", "SYS_NC00004$", `UPPER("STATUS")`).
			AddRow(3, "APP", "ORDERS", "ORDERS_FBI_COMPOSITE_IDX", "NONUNIQUE", "FUNCTION-BASED NORMAL", "CUSTOMER_ID", nil).
			AddRow(3, "APP", "ORDERS", "ORDERS_FBI_COMPOSITE_IDX", "NONUNIQUE", "FUNCTION-BASED NORMAL", "SYS_NC00005$", `UPPER("STATUS")`))

	allowed := map[tableKey]struct{}{{conID: 3, owner: "APP", table: "ORDERS"}: {}}
	details := c.tableDetails(context.Background(), []string{"'APP'"}, allowed)

	d := details[tableKey{conID: 3, owner: "APP", table: "ORDERS"}]
	require.NotNil(t, d)
	require.Len(t, d.Indexes, 2)

	single := d.Indexes[0]
	assert.Equal(t, "ORDERS_FBI_IDX", single.Name)
	assert.Equal(t, []indexKeyPart{{Expression: `UPPER("STATUS")`}}, single.Columns,
		"a single-column FBI must report its expression, not be dropped for having zero columns")

	composite := d.Indexes[1]
	assert.Equal(t, "ORDERS_FBI_COMPOSITE_IDX", composite.Name)
	assert.Equal(t, []indexKeyPart{{Column: "CUSTOMER_ID"}, {Expression: `UPPER("STATUS")`}}, composite.Columns,
		"the plain column and the expression column must both survive, in position order")
}

// TestTableDetailsConstraintsResolveForeignKey covers the two-pass FK resolution: a foreign key
// names the constraint it references, not the table, and that constraint's table/columns are
// only known once every table's constraints have been scanned.
func TestTableDetailsConstraintsResolveForeignKey(t *testing.T) {
	c, _, dbMock, closeDB := newSchemaCheck(t)
	defer closeDB()

	dbMock.MatchExpectationsInOrder(false)
	dbMock.ExpectQuery("cdb_constraints").WillReturnRows(
		sqlmock.NewRows([]string{
			"CON_ID", "OWNER", "TABLE_NAME", "CONSTRAINT_NAME", "CONSTRAINT_TYPE",
			"R_OWNER", "R_CONSTRAINT_NAME", "COLUMN_NAME", "SEARCH_CONDITION",
		}).
			// Primary key of the referenced table, ORDERS.
			AddRow(3, "APP", "ORDERS", "ORDERS_PK", "P", "-", "-", "ORDER_ID", nil).
			// A two-column composite foreign key on ORDER_ITEMS, in column-position order.
			AddRow(3, "APP", "ORDER_ITEMS", "ITEMS_FK", "R", "APP", "ORDERS_PK", "ORDER_ID", nil).
			AddRow(3, "APP", "ORDER_ITEMS", "ITEMS_FK", "R", "APP", "ORDERS_PK", "LINE_NO", nil))

	allowed := map[tableKey]struct{}{
		{conID: 3, owner: "APP", table: "ORDERS"}:      {},
		{conID: 3, owner: "APP", table: "ORDER_ITEMS"}: {},
	}
	details := c.tableDetails(context.Background(), []string{"'APP'"}, allowed)

	items := details[tableKey{conID: 3, owner: "APP", table: "ORDER_ITEMS"}]
	require.NotNil(t, items)
	require.Len(t, items.Constraints, 1)
	fk := items.Constraints[0]
	assert.Equal(t, "foreign_key", fk.Type)
	assert.Equal(t, []string{"ORDER_ID", "LINE_NO"}, fk.Columns,
		"a composite FK's own columns must accumulate in position order")
	assert.Equal(t, "ORDERS", fk.ReferencedTable, "resolved from the second pass over primaryKeys")
	assert.Equal(t, []string{"ORDER_ID"}, fk.ReferencedColumns)
	assert.Empty(t, fk.ReferencedConstraint, "a resolved FK does not need the constraint-name fallback")
}

// TestTableDetailsUnresolvedForeignKeyFallsBackToConstraintName covers the case where the
// referenced owner was never collected (excluded, or filtered out): the FK cannot be resolved
// to a table/columns, so it must fall back to naming the constraint rather than reporting an
// empty (and misleadingly "resolved") referenced table.
func TestTableDetailsUnresolvedForeignKeyFallsBackToConstraintName(t *testing.T) {
	c, _, dbMock, closeDB := newSchemaCheck(t)
	defer closeDB()

	dbMock.MatchExpectationsInOrder(false)
	dbMock.ExpectQuery("cdb_constraints").WillReturnRows(
		sqlmock.NewRows([]string{
			"CON_ID", "OWNER", "TABLE_NAME", "CONSTRAINT_NAME", "CONSTRAINT_TYPE",
			"R_OWNER", "R_CONSTRAINT_NAME", "COLUMN_NAME", "SEARCH_CONDITION",
		}).
			AddRow(3, "APP", "ORDER_ITEMS", "ITEMS_FK", "R", "OTHER_APP", "ORDERS_PK", "ORDER_ID", nil))

	allowed := map[tableKey]struct{}{{conID: 3, owner: "APP", table: "ORDER_ITEMS"}: {}}
	details := c.tableDetails(context.Background(), []string{"'APP'"}, allowed)

	fk := details[tableKey{conID: 3, owner: "APP", table: "ORDER_ITEMS"}].Constraints[0]
	assert.Empty(t, fk.ReferencedTable, "the referencing owner was never scanned, so the table cannot be resolved")
	assert.Equal(t, "ORDERS_PK", fk.ReferencedConstraint, "falls back to naming the constraint instead of looking corrupt")
}

// TestTableDetailsCheckConstraint covers the constraint_type = 'C' branch: a user-defined CHECK
// constraint must render as "check" (not the raw Oracle code) and carry its condition text.
func TestTableDetailsCheckConstraint(t *testing.T) {
	c, _, dbMock, closeDB := newSchemaCheck(t)
	defer closeDB()

	dbMock.MatchExpectationsInOrder(false)
	dbMock.ExpectQuery("cdb_constraints").WillReturnRows(
		sqlmock.NewRows([]string{
			"CON_ID", "OWNER", "TABLE_NAME", "CONSTRAINT_NAME", "CONSTRAINT_TYPE",
			"R_OWNER", "R_CONSTRAINT_NAME", "COLUMN_NAME", "SEARCH_CONDITION",
		}).
			AddRow(3, "APP", "ORDERS", "ORDERS_STATUS_CHK", "C", "-", "-", "STATUS", "status IN ('NEW','SHIPPED')"))

	allowed := map[tableKey]struct{}{{conID: 3, owner: "APP", table: "ORDERS"}: {}}
	details := c.tableDetails(context.Background(), []string{"'APP'"}, allowed)

	con := details[tableKey{conID: 3, owner: "APP", table: "ORDERS"}].Constraints[0]
	assert.Equal(t, "check", con.Type, "constraintType must map C to \"check\", not pass through the raw code")
	assert.Equal(t, "status IN ('NEW','SHIPPED')", con.Condition)
	assert.Empty(t, con.ReferencedTable, "a check constraint has no referenced table")
}

// TestTableDetailsPartitionKeyJoin covers the second pass that stitches cdb_part_key_columns
// (one row per key column, ordered by position) onto the partitionDetail already built from
// cdb_part_tables.
func TestTableDetailsPartitionKeyJoin(t *testing.T) {
	c, _, dbMock, closeDB := newSchemaCheck(t)
	defer closeDB()

	dbMock.MatchExpectationsInOrder(false)
	dbMock.ExpectQuery("cdb_part_tables").WillReturnRows(
		sqlmock.NewRows([]string{"CON_ID", "OWNER", "TABLE_NAME", "PARTITIONING_TYPE", "SUBPARTITIONING_TYPE", "PARTITION_COUNT"}).
			AddRow(3, "APP", "EVENTS", "RANGE", "NONE", 4))
	dbMock.ExpectQuery("cdb_part_key_columns").WillReturnRows(
		sqlmock.NewRows([]string{"CON_ID", "OWNER", "NAME", "COLUMN_NAME"}).
			AddRow(3, "APP", "EVENTS", "EVENT_DATE"))

	allowed := map[tableKey]struct{}{{conID: 3, owner: "APP", table: "EVENTS"}: {}}
	details := c.tableDetails(context.Background(), []string{"'APP'"}, allowed)

	p := details[tableKey{conID: 3, owner: "APP", table: "EVENTS"}].Partitioned
	require.NotNil(t, p)
	assert.Equal(t, int64(4), p.NumPartitions)
	assert.Empty(t, p.SubpartitionsType, "NONE must not surface as a subpartitioning type")
	assert.Equal(t, "RANGE (EVENT_DATE)", p.PartitionKey)
}

// TestTableDetailsExternalLocationsConcat covers the directory:location concatenation, and that
// a NULL directory_name (falling back to the table's default_directory_name) is rendered as a
// bare location instead of a leading colon.
func TestTableDetailsExternalLocationsConcat(t *testing.T) {
	c, _, dbMock, closeDB := newSchemaCheck(t)
	defer closeDB()

	dbMock.MatchExpectationsInOrder(false)
	dbMock.ExpectQuery("cdb_external_tables").WillReturnRows(
		sqlmock.NewRows([]string{"CON_ID", "OWNER", "TABLE_NAME", "TYPE_NAME", "NVL(default_directory_name, '-')"}).
			AddRow(3, "APP", "EXT_ORDERS", "ORACLE_LOADER", "DEFAULT_DIR"))
	dbMock.ExpectQuery("cdb_external_locations").WillReturnRows(
		sqlmock.NewRows([]string{"CON_ID", "OWNER", "TABLE_NAME", "NVL(directory_name, '-')", "LOCATION"}).
			AddRow(3, "APP", "EXT_ORDERS", "LOAD_DIR", "orders_2024.csv").
			AddRow(3, "APP", "EXT_ORDERS", "-", "orders_fallback.csv"))

	allowed := map[tableKey]struct{}{{conID: 3, owner: "APP", table: "EXT_ORDERS"}: {}}
	details := c.tableDetails(context.Background(), []string{"'APP'"}, allowed)

	ext := details[tableKey{conID: 3, owner: "APP", table: "EXT_ORDERS"}].External
	require.NotNil(t, ext)
	assert.Equal(t, "ORACLE_LOADER", ext.AccessDriver)
	assert.Equal(t, "DEFAULT_DIR", ext.Directory)
	require.Len(t, ext.Locations, 2)
	assert.Equal(t, "LOAD_DIR:orders_2024.csv", ext.Locations[0])
	assert.Equal(t, "orders_fallback.csv", ext.Locations[1],
		"a NULL directory_name must not be concatenated as a literal '-:' prefix")
}

// TestTableDetailsBlockchainAndImmutableRetention covers scanRetention's withHash branch (used
// for blockchain tables, which additionally carry a hash algorithm and version) versus the
// plain branch (immutable tables, which do not).
func TestTableDetailsBlockchainAndImmutableRetention(t *testing.T) {
	c, _, dbMock, closeDB := newSchemaCheck(t)
	defer closeDB()

	dbMock.MatchExpectationsInOrder(false)
	dbMock.ExpectQuery("cdb_blockchain_tables").WillReturnRows(
		sqlmock.NewRows([]string{
			"CON_ID", "SCHEMA_NAME", "TABLE_NAME", "ROW_RETENTION", "ROW_RETENTION_LOCKED",
			"TABLE_INACTIVITY_RETENTION", "HASH_ALGORITHM", "TABLE_VERSION",
		}).AddRow(3, "APP", "LEDGER", 90, "YES", 30, "SHA2_512", "v2"))
	dbMock.ExpectQuery("cdb_immutable_tables").WillReturnRows(
		sqlmock.NewRows([]string{
			"CON_ID", "SCHEMA_NAME", "TABLE_NAME", "ROW_RETENTION", "ROW_RETENTION_LOCKED",
			"TABLE_INACTIVITY_RETENTION",
		}).AddRow(3, "APP", "AUDIT_LOG", 365, "NO", nil))

	allowed := map[tableKey]struct{}{
		{conID: 3, owner: "APP", table: "LEDGER"}:    {},
		{conID: 3, owner: "APP", table: "AUDIT_LOG"}: {},
	}
	details := c.tableDetails(context.Background(), []string{"'APP'"}, allowed)

	ledger := details[tableKey{conID: 3, owner: "APP", table: "LEDGER"}].Blockchain
	require.NotNil(t, ledger)
	require.NotNil(t, ledger.RowRetentionDays)
	assert.Equal(t, int64(90), *ledger.RowRetentionDays)
	assert.True(t, ledger.RowRetentionLocked)
	assert.Equal(t, "SHA2_512", ledger.HashAlgorithm)
	assert.Equal(t, "v2", ledger.TableVersion)

	audit := details[tableKey{conID: 3, owner: "APP", table: "AUDIT_LOG"}].Immutable
	require.NotNil(t, audit)
	assert.False(t, audit.RowRetentionLocked)
	assert.Nil(t, audit.InactivityRetentionDays, "a NULL inactivity retention must stay nil, not zero")
	assert.Empty(t, audit.HashAlgorithm, "immutable tables carry no hash algorithm")
}

// TestSnapshotChunkingAtRealisticTableBoundary is the multi-column companion to
// TestSnapshotChunking: with one-column tables, a naive "flush every N rows" chunker happens to
// also flush at table boundaries, hiding a bug where a table's columns get split across two
// payloads. Three-column tables at chunk size 2 exercise that boundary for real.
func TestSnapshotChunkingAtRealisticTableBoundary(t *testing.T) {
	c, _, _, closeDB := newSchemaCheck(t)
	defer closeDB()
	c.config.Schemas.PayloadChunkSize = 2

	var payloads []schemaEvent
	collector := newSchemaCollector(&c, func(b []byte) {
		var e schemaEvent
		require.NoError(t, json.Unmarshal(b, &e))
		payloads = append(payloads, e)
	}, map[tableKey]*tableDetails{}, map[ownerKey]string{}, map[int64]string{})

	for _, name := range []string{"T1", "T2", "T3"} {
		for _, col := range []string{"C1", "C2", "C3"} {
			collector.add(schemaRowDB{
				ConID: 3, Owner: "APP", TableName: name, Temporary: "N", External: "NO",
				IotType: "-", ClusterName: "-", Partitioned: "NO",
				ColumnName: col, DataType: sql.NullString{String: "NUMBER", Valid: true},
				Nullable: "Y",
			})
		}
	}
	collector.finish()

	require.Len(t, payloads, 2, "3 tables at chunk size 2 must split into 2 payloads, not one per column row")

	var seenTables []string
	for _, p := range payloads {
		for _, table := range p.Metadata[0].Schemas[0].Tables {
			seenTables = append(seenTables, table.Name)
			assert.Len(t, table.Columns, 3, "table %s must keep all 3 of its columns in one payload", table.Name)
		}
	}
	assert.Equal(t, []string{"T1", "T2", "T3"}, seenTables, "no table must be split or dropped across the chunk boundary")
	assert.Equal(t, 2, payloads[1].CollectionPayloadsCount)
}

// TestSchemaCollectionScanErrorEmitsNoPayload guards the failure path: fetchMetadataRows
// buffers every row before any collector is created, so a scan error must return an error from
// SchemaCollection before anything is emitted -- no partial, uncounted payload for a snapshot
// the backend would otherwise never know is incomplete.
func TestSchemaCollectionScanErrorEmitsNoPayload(t *testing.T) {
	c, _, dbMock, closeDB := newSchemaCheck(t)
	defer closeDB()
	dbMock.MatchExpectationsInOrder(false)

	dbMock.ExpectQuery(`v\$containers`).WillReturnRows(
		sqlmock.NewRows([]string{"CON_ID", "NAME"}).AddRow(3, "APP_PDB"))
	dbMock.ExpectQuery("cdb_users").WillReturnRows(
		sqlmock.NewRows([]string{"CON_ID", "USERNAME", "USER_ID"}).AddRow(3, "APP", 104))

	// A row with a NULL where the scan target is non-nullable (TABLE_NAME) forces
	// StructScan to fail partway through the result set.
	mainRows := sqlmock.NewRows([]string{
		"CON_ID", "OWNER", "TABLE_NAME", "TEMPORARY", "DURATION", "EXTERNAL", "IOT_TYPE",
		"PARTITIONED", "CLUSTER_NAME", "CLUSTERING", "READ_ONLY", "NUM_ROWS", "LAST_ANALYZED",
		"OBJECT_TYPE_OWNER", "OBJECT_TYPE",
		"COLUMN_NAME", "COLUMN_ID", "INTERNAL_COLUMN_ID", "VIRTUAL_COLUMN", "HIDDEN_COLUMN", "DATA_TYPE",
		"DATA_TYPE_OWNER", "DATA_TYPE_MOD", "DATA_LENGTH", "CHAR_LENGTH", "DATA_PRECISION",
		"DATA_SCALE", "CHAR_USED", "NULLABLE", "DATA_DEFAULT_VC",
	}).AddRow(
		3, "APP", nil, "N", "-", "NO", "-", "NO", "-", "NO", "NO", nil, nil, "-", "-",
		"ORDER_ID", 1, 1, "NO", "NO", "NUMBER", nil, nil, 22, nil, 12, 0, "-", "N", nil,
	)
	dbMock.ExpectQuery("cdb_tab_cols").WillReturnRows(mainRows)

	c2, sender := newDbDoesNotExistCheck(t, "", "")
	c2.db = c.db
	c2.clock = c.clock
	c2.dbVersion = c.dbVersion
	c2.config.Schemas = c.config.Schemas

	require.Error(t, c2.SchemaCollection(), "a NULL into a non-nullable column must surface as a scan error")
	sender.AssertNotCalled(t, "EventPlatformEvent", mock.Anything, mock.Anything)
}

// TestMaxTablesTruncationFlag pins the truncated flag to TOTAL_TABLES vs max_tables: it must be
// set from the very first row of a capped container (the window total rides on every row), and
// must stay false when a container legitimately has fewer tables than the cap.
func TestMaxTablesTruncationFlag(t *testing.T) {
	c, _, _, closeDB := newSchemaCheck(t)
	defer closeDB()
	c.config.Schemas.PayloadChunkSize = 100
	c.config.Schemas.MaxTables = 1

	var payloads []schemaEvent
	collector := newSchemaCollector(&c, func(b []byte) {
		var e schemaEvent
		require.NoError(t, json.Unmarshal(b, &e))
		payloads = append(payloads, e)
	}, map[tableKey]*tableDetails{}, map[ownerKey]string{}, map[int64]string{})

	// TOTAL_TABLES=2 with max_tables=1: only the single ranked_tables row made it through the
	// SQL cap, but the total tells the collector two tables exist for this container.
	collector.add(schemaRowDB{
		ConID: 3, Owner: "APP", TableName: "T1", Temporary: "N", External: "NO",
		IotType: "-", ClusterName: "-", Partitioned: "NO",
		TotalTables: sql.NullInt64{Int64: 2, Valid: true},
		ColumnName:  "C1", DataType: sql.NullString{String: "NUMBER", Valid: true}, Nullable: "Y",
	})
	collector.finish()

	require.Len(t, payloads, 1)
	assert.True(t, payloads[0].Truncated, "TOTAL_TABLES exceeding max_tables must mark the payload truncated")
}

func TestMaxTablesNotTruncatedWhenUnderCap(t *testing.T) {
	c, _, _, closeDB := newSchemaCheck(t)
	defer closeDB()
	c.config.Schemas.PayloadChunkSize = 100
	c.config.Schemas.MaxTables = 300

	var payloads []schemaEvent
	collector := newSchemaCollector(&c, func(b []byte) {
		var e schemaEvent
		require.NoError(t, json.Unmarshal(b, &e))
		payloads = append(payloads, e)
	}, map[tableKey]*tableDetails{}, map[ownerKey]string{}, map[int64]string{})

	collector.add(schemaRowDB{
		ConID: 3, Owner: "APP", TableName: "T1", Temporary: "N", External: "NO",
		IotType: "-", ClusterName: "-", Partitioned: "NO",
		TotalTables: sql.NullInt64{Int64: 1, Valid: true},
		ColumnName:  "C1", DataType: sql.NullString{String: "NUMBER", Valid: true}, Nullable: "Y",
	})
	collector.finish()

	require.Len(t, payloads, 1)
	assert.False(t, payloads[0].Truncated, "a container with fewer tables than max_tables must not be marked truncated")
}

// TestMaxColumnsCapsColumnsAndFlagsTruncation covers the per-table column cap: max_columns is
// now enforced server-side (ranked_columns caps rows to col_rn <= max_columns), so the collector
// only ever sees the already-capped rows. It must still surface the drop via truncated, using
// TOTAL_COLUMNS -- the ranked_columns window total computed before the cap was applied.
func TestMaxColumnsCapsColumnsAndFlagsTruncation(t *testing.T) {
	c, _, _, closeDB := newSchemaCheck(t)
	defer closeDB()
	c.config.Schemas.PayloadChunkSize = 100
	c.config.Schemas.MaxColumns = 2

	var payloads []schemaEvent
	collector := newSchemaCollector(&c, func(b []byte) {
		var e schemaEvent
		require.NoError(t, json.Unmarshal(b, &e))
		payloads = append(payloads, e)
	}, map[tableKey]*tableDetails{}, map[ownerKey]string{}, map[int64]string{})

	// TOTAL_COLUMNS=4 with max_columns=2: only the first two ranked_columns rows made it through
	// the SQL cap, but the total tells the collector four columns exist on this table.
	for _, col := range []string{"C1", "C2"} {
		collector.add(schemaRowDB{
			ConID: 3, Owner: "APP", TableName: "WIDE", Temporary: "N", External: "NO",
			IotType: "-", ClusterName: "-", Partitioned: "NO",
			TotalColumns: sql.NullInt64{Int64: 4, Valid: true},
			ColumnName:   col, DataType: sql.NullString{String: "NUMBER", Valid: true}, Nullable: "Y",
		})
	}
	collector.finish()

	require.Len(t, payloads, 1)
	table := payloads[0].Metadata[0].Schemas[0].Tables[0]
	assert.Len(t, table.Columns, 2, "max_columns=2 must cap the table at 2 columns")
	assert.True(t, payloads[0].Truncated, "TOTAL_COLUMNS exceeding max_columns must mark the payload truncated")
}

func TestMaxColumnsNotTruncatedWhenUnderCap(t *testing.T) {
	c, _, _, closeDB := newSchemaCheck(t)
	defer closeDB()
	c.config.Schemas.PayloadChunkSize = 100
	c.config.Schemas.MaxColumns = 50

	var payloads []schemaEvent
	collector := newSchemaCollector(&c, func(b []byte) {
		var e schemaEvent
		require.NoError(t, json.Unmarshal(b, &e))
		payloads = append(payloads, e)
	}, map[tableKey]*tableDetails{}, map[ownerKey]string{}, map[int64]string{})

	collector.add(schemaRowDB{
		ConID: 3, Owner: "APP", TableName: "T", Temporary: "N", External: "NO",
		IotType: "-", ClusterName: "-", Partitioned: "NO",
		TotalColumns: sql.NullInt64{Int64: 1, Valid: true},
		ColumnName:   "C1", DataType: sql.NullString{String: "NUMBER", Valid: true}, Nullable: "Y",
	})
	collector.finish()

	require.Len(t, payloads, 1)
	assert.False(t, payloads[0].Truncated, "a table with fewer columns than max_columns must not be marked truncated")
}

// TestMaxColumnsNotTruncatedWhenExactlyAtCap guards the boundary: a table with exactly
// max_columns columns must not be reported as truncated, since none were actually dropped.
func TestMaxColumnsNotTruncatedWhenExactlyAtCap(t *testing.T) {
	c, _, _, closeDB := newSchemaCheck(t)
	defer closeDB()
	c.config.Schemas.PayloadChunkSize = 100
	c.config.Schemas.MaxColumns = 2

	var payloads []schemaEvent
	collector := newSchemaCollector(&c, func(b []byte) {
		var e schemaEvent
		require.NoError(t, json.Unmarshal(b, &e))
		payloads = append(payloads, e)
	}, map[tableKey]*tableDetails{}, map[ownerKey]string{}, map[int64]string{})

	for _, col := range []string{"C1", "C2"} {
		collector.add(schemaRowDB{
			ConID: 3, Owner: "APP", TableName: "T", Temporary: "N", External: "NO",
			IotType: "-", ClusterName: "-", Partitioned: "NO",
			TotalColumns: sql.NullInt64{Int64: 2, Valid: true},
			ColumnName:   col, DataType: sql.NullString{String: "NUMBER", Valid: true}, Nullable: "Y",
		})
	}
	collector.finish()

	require.Len(t, payloads, 1)
	table := payloads[0].Metadata[0].Schemas[0].Tables[0]
	assert.Len(t, table.Columns, 2)
	assert.False(t, payloads[0].Truncated, "TOTAL_COLUMNS equal to max_columns must not mark the payload truncated")
}

// TestSchemaOwnersRejectsUnexpectedCharacters guards schemaOwnerPattern: a schema name outside
// [A-Z0-9_$#] (defensive against something exotic making it into cdb_users) must be dropped
// rather than quoted verbatim into a later IN-list.
func TestSchemaOwnersRejectsUnexpectedCharacters(t *testing.T) {
	c, _, dbMock, closeDB := newSchemaCheck(t)
	defer closeDB()

	dbMock.ExpectQuery("cdb_users").WillReturnRows(
		sqlmock.NewRows([]string{"CON_ID", "USERNAME", "USER_ID"}).
			AddRow(3, "APP", 104).
			AddRow(3, "BOGUS'; DROP", 105))

	owners, names, err := c.schemaOwners(context.Background(), map[int64]string{3: "APP_PDB"})
	require.NoError(t, err)
	assert.Equal(t, []string{"APP"}, names)
	assert.Contains(t, owners, ownerKey{conID: 3, owner: "APP"})
	assert.NotContains(t, owners, ownerKey{conID: 3, owner: "BOGUS'; DROP"})
}

// TestSchemaOwnersBatchesBeyondMaxSchemaOwners guards ORA-01795: an owner list beyond 1000
// entries must still all be collected, split into IN-list batches, not truncated to the first
// 1000 alphabetically.
func TestSchemaOwnersBatchesBeyondMaxSchemaOwners(t *testing.T) {
	c, _, dbMock, closeDB := newSchemaCheck(t)
	defer closeDB()

	rows := sqlmock.NewRows([]string{"CON_ID", "USERNAME", "USER_ID"})
	const total = maxSchemaOwners + 10
	for i := 0; i < total; i++ {
		rows.AddRow(3, fmt.Sprintf("APP%04d", i), 1000+i)
	}
	dbMock.ExpectQuery("cdb_users").WillReturnRows(rows)

	owners, names, err := c.schemaOwners(context.Background(), map[int64]string{})
	require.NoError(t, err)
	assert.Len(t, names, total, "every owner beyond the 1000-item IN-list cap must still be returned")
	assert.Len(t, owners, total)

	chunks := ownerListChunks(names)
	require.Len(t, chunks, 2, "1010 owners must split into two IN-list batches")
	assert.Equal(t, "APP0999", strings.Trim(strings.Split(chunks[0], ", ")[len(strings.Split(chunks[0], ", "))-1], "'"))
}

// TestFetchMetadataRowsDropsRowsNotInOwners guards the owner-membership filter: a metadata row
// for a con_id/owner pair absent from the owners map (e.g. a schema excluded, or one only
// visible in a container that was itself filtered out) must be dropped rather than surfaced.
func TestFetchMetadataRowsDropsRowsNotInOwners(t *testing.T) {
	c, _, dbMock, closeDB := newSchemaCheck(t)
	defer closeDB()

	dbMock.ExpectQuery("cdb_tab_cols").WillReturnRows(
		sqlmock.NewRows([]string{
			"CON_ID", "OWNER", "TABLE_NAME", "TEMPORARY", "DURATION", "EXTERNAL", "IOT_TYPE",
			"PARTITIONED", "CLUSTER_NAME", "CLUSTERING", "READ_ONLY", "NUM_ROWS", "LAST_ANALYZED",
			"OBJECT_TYPE_OWNER", "OBJECT_TYPE", "TOTAL_TABLES",
			"COLUMN_NAME", "COLUMN_ID", "INTERNAL_COLUMN_ID", "VIRTUAL_COLUMN", "HIDDEN_COLUMN", "DATA_TYPE",
			"DATA_TYPE_OWNER", "DATA_TYPE_MOD", "DATA_LENGTH", "CHAR_LENGTH", "DATA_PRECISION",
			"DATA_SCALE", "CHAR_USED", "NULLABLE", "DATA_DEFAULT_VC",
		}).
			AddRow(3, "APP", "ORDERS", "N", "-", "NO", "-", "NO", "-", "NO", "NO", nil, nil, "-", "-", 1,
				"ORDER_ID", 1, 1, "NO", "NO", "NUMBER", nil, nil, 22, nil, 12, 0, "-", "N", nil).
			AddRow(3, "GHOST", "PHANTOM", "N", "-", "NO", "-", "NO", "-", "NO", "NO", nil, nil, "-", "-", 1,
				"COL", 1, 1, "NO", "NO", "NUMBER", nil, nil, 22, nil, 12, 0, "-", "N", nil))

	owners := map[ownerKey]string{{conID: 3, owner: "APP"}: "104"}
	rows, err := c.fetchMetadataRows(context.Background(), schemasQueryTemplate, []string{"'APP'"}, owners, nil)
	require.NoError(t, err)

	require.Len(t, rows, 1, "the row for an owner absent from the owners map must be dropped")
	assert.Equal(t, "ORDERS", rows[0].TableName)
}

// TestEmptyContainerStillEmitsTerminatingPayload guards emitEmptyContainers: a container that
// has owners but produced no table rows (every table dropped, or none ever existed) must still
// get a terminating, empty-metadata payload, or the backend keeps serving its last snapshot.
func TestEmptyContainerStillEmitsTerminatingPayload(t *testing.T) {
	c, _, _, closeDB := newSchemaCheck(t)
	defer closeDB()
	c.config.Schemas.PayloadChunkSize = 100

	var payloads []schemaEvent
	collector := newSchemaCollector(&c, func(b []byte) {
		var e schemaEvent
		require.NoError(t, json.Unmarshal(b, &e))
		payloads = append(payloads, e)
	}, map[tableKey]*tableDetails{}, map[ownerKey]string{}, map[int64]string{5: "EMPTY_PDB"})

	collector.emitEmptyContainers(map[int64]string{5: "EMPTY_PDB"})

	require.Len(t, payloads, 1)
	assert.Equal(t, "5", payloads[0].Metadata[0].ID)
	assert.Empty(t, payloads[0].Metadata[0].Schemas)
	assert.Equal(t, 1, payloads[0].CollectionPayloadsCount, "an empty container's payload must still be marked complete")
}

// TestEmptyContainerSkippedIfAlreadyStarted guards the other half of emitEmptyContainers: a
// container that did produce rows must not additionally get a second, empty payload.
func TestEmptyContainerSkippedIfAlreadyStarted(t *testing.T) {
	c, _, _, closeDB := newSchemaCheck(t)
	defer closeDB()
	c.config.Schemas.PayloadChunkSize = 100

	var payloads []schemaEvent
	collector := newSchemaCollector(&c, func(b []byte) {
		var e schemaEvent
		require.NoError(t, json.Unmarshal(b, &e))
		payloads = append(payloads, e)
	}, map[tableKey]*tableDetails{}, map[ownerKey]string{}, map[int64]string{3: "APP_PDB"})

	collector.add(schemaRowDB{
		ConID: 3, Owner: "APP", TableName: "T", Temporary: "N", External: "NO",
		IotType: "-", ClusterName: "-", Partitioned: "NO",
		ColumnName: "C1", DataType: sql.NullString{String: "NUMBER", Valid: true}, Nullable: "Y",
	})
	collector.finish()
	collector.emitEmptyContainers(map[int64]string{3: "APP_PDB"})

	require.Len(t, payloads, 1, "a container that already produced a payload must not get a second, empty one")
}

// TestColumnDefaultTruncatedAtVarchar4000Cap guards truncateLongValue's use on DATA_DEFAULT: a
// default value read from the LONG column (pre-23ai) or its _VC replacement must be capped at
// VARCHAR2(4000) runes, matching what Oracle itself would report through a VARCHAR2 column.
func TestColumnDefaultTruncatedAtVarchar4000Cap(t *testing.T) {
	c, _, _, closeDB := newSchemaCheck(t)
	defer closeDB()
	c.config.Schemas.PayloadChunkSize = 100

	var payloads []schemaEvent
	collector := newSchemaCollector(&c, func(b []byte) {
		var e schemaEvent
		require.NoError(t, json.Unmarshal(b, &e))
		payloads = append(payloads, e)
	}, map[tableKey]*tableDetails{}, map[ownerKey]string{}, map[int64]string{})

	long := strings.Repeat("x", 4500)
	collector.add(schemaRowDB{
		ConID: 3, Owner: "APP", TableName: "T", Temporary: "N", External: "NO",
		IotType: "-", ClusterName: "-", Partitioned: "NO",
		ColumnName: "C1", DataType: sql.NullString{String: "VARCHAR2", Valid: true},
		DataLength: sql.NullInt64{Int64: 4000, Valid: true}, CharLength: sql.NullInt64{Int64: 4000, Valid: true},
		CharUsed: "C", Nullable: "Y",
		DataDefault: sql.NullString{String: long, Valid: true},
	})
	collector.finish()

	require.Len(t, payloads, 1)
	col := payloads[0].Metadata[0].Schemas[0].Tables[0].Columns[0]
	assert.Len(t, col.Default, 4000, "a default value beyond VARCHAR2(4000) must be truncated to exactly that cap")
}

// TestSchemaCollectionGatedByDbmOrDataObservability covers the four combinations of
// schemas.enabled, dbm_enabled and data_observability.enabled that decide whether Run() ever
// calls SchemaCollection. checkIntervalExpired uses the wall clock (not the mock clock), so the
// unrelated collection intervals are pre-marked as just-run to isolate the schemas gate.
func TestSchemaCollectionGatedByDbmOrDataObservability(t *testing.T) {
	cases := []struct {
		name           string
		schemasEnabled bool
		dbmEnabled     bool
		doEnabled      bool
		wantGateOpen   bool
	}{
		{"schemas disabled blocks collection even with data_observability enabled", false, false, true, false},
		{"schemas enabled but neither dbm nor data_observability leaves the gate closed", true, false, false, false},
		{"dbm_enabled alone opens the gate", true, true, false, true},
		{"data_observability.enabled alone opens the gate without dbm", true, false, true, true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			c, _, dbMock, closeDB := newSchemaCheck(t)
			defer closeDB()
			dbMock.MatchExpectationsInOrder(false)

			c.initialized = true
			c.dbmEnabled = tc.dbmEnabled
			c.config.DataObservability.Enabled = tc.doEnabled
			c.config.Schemas.Enabled = tc.schemasEnabled
			c.config.QuerySamples.Enabled = false // avoid an unrelated SampleSession call when dbmEnabled is true

			now := time.Now()
			c.metricLastRun = now
			c.dbInstanceLastRun = now
			c.tablespaceLastRun = now
			c.schemasLastRun = time.Time{}

			if tc.wantGateOpen {
				dbMock.ExpectQuery(`v\$containers`).WillReturnRows(sqlmock.NewRows([]string{"CON_ID", "NAME"}))
				dbMock.ExpectQuery("cdb_users").WillReturnRows(sqlmock.NewRows([]string{"CON_ID", "USERNAME", "USER_ID"}))
			}

			err := c.Run()

			if tc.wantGateOpen {
				require.NoError(t, err)
				assert.NoError(t, dbMock.ExpectationsWereMet(), "the gate must have opened and queried cdb_users")
			} else {
				// No expectations were primed: if the gate opened anyway, the resulting
				// unexpected-query error would surface here.
				require.NoError(t, err, "the gate must stay closed and touch the database not at all")
			}
		})
	}
}

// TestPassesFilterExcludeWinsOverInclude pins the precedence contract regexSQLClauses and
// schemaOwners/filterContainers all rely on: exclude wins outright, and when include is
// non-empty at least one include pattern must also match.
func TestPassesFilterExcludeWinsOverInclude(t *testing.T) {
	include := compiledPatterns([]string{"^APP.*"}, "", "include")
	exclude := compiledPatterns([]string{"^APP_TMP$"}, "", "exclude")

	assert.True(t, passesFilter("APP_ORDERS", include, exclude), "matches include, does not match exclude")
	assert.False(t, passesFilter("APP_TMP", include, exclude), "exclude wins even though it also matches include")
	assert.False(t, passesFilter("OTHER", include, exclude), "include is non-empty and OTHER matches none of it")
	assert.True(t, passesFilter("OTHER", nil, exclude), "an empty include list requires no match")
}

// TestFilterContainersAppliesIncludeExcludeDatabases guards the database-level (container)
// filter used ahead of schemaOwners and the main query: an unfiltered config must be a no-op,
// and exclude must win over a broader include, mirroring passesFilter.
func TestFilterContainersAppliesIncludeExcludeDatabases(t *testing.T) {
	containers := map[int64]string{1: "CDB$ROOT", 3: "APP_PDB", 7: "REPORTING_PDB"}

	assert.Equal(t, containers, filterContainers(containers, nil, nil, ""),
		"no configured filters must return every container unchanged")

	filtered := filterContainers(containers, []string{"PDB$"}, nil, "")
	assert.Equal(t, map[int64]string{3: "APP_PDB", 7: "REPORTING_PDB"}, filtered)

	filtered = filterContainers(containers, []string{"PDB$"}, []string{"^APP"}, "")
	assert.Equal(t, map[int64]string{7: "REPORTING_PDB"}, filtered, "exclude must win over a broader include")
}

// TestSchemaCollectionAppliesTableIncludeExcludeFilters guards regexSQLClauses actually reaching
// the query sent to Oracle: include_tables/exclude_tables are rendered as REGEXP_LIKE predicates
// substituted into /*TABLE_FILTERS*/, not applied after the fact in Go.
func TestSchemaCollectionAppliesTableIncludeExcludeFilters(t *testing.T) {
	c, _, dbMock, closeDB := newSchemaCheck(t)
	defer closeDB()
	dbMock.MatchExpectationsInOrder(false)

	c.config.Schemas.IncludeTables = []string{"^ORD"}
	c.config.Schemas.ExcludeTables = []string{"_STAGING$"}

	dbMock.ExpectQuery(`v\$containers`).WillReturnRows(sqlmock.NewRows([]string{"CON_ID", "NAME"}))
	dbMock.ExpectQuery("cdb_users").WillReturnRows(
		sqlmock.NewRows([]string{"CON_ID", "USERNAME", "USER_ID"}).AddRow(3, "APP", 104))

	expectedFilter := regexSQLClauses("t.table_name", c.config.Schemas.IncludeTables, c.config.Schemas.ExcludeTables)
	require.Contains(t, expectedFilter, "REGEXP_LIKE")

	dbMock.ExpectQuery(regexp.QuoteMeta(expectedFilter)).WillReturnRows(sqlmock.NewRows([]string{
		"CON_ID", "OWNER", "TABLE_NAME", "TEMPORARY", "DURATION", "EXTERNAL", "IOT_TYPE",
		"PARTITIONED", "CLUSTER_NAME", "CLUSTERING", "READ_ONLY", "NUM_ROWS", "LAST_ANALYZED",
		"OBJECT_TYPE_OWNER", "OBJECT_TYPE", "TOTAL_TABLES",
		"COLUMN_NAME", "COLUMN_ID", "INTERNAL_COLUMN_ID", "VIRTUAL_COLUMN", "HIDDEN_COLUMN", "DATA_TYPE",
		"DATA_TYPE_OWNER", "DATA_TYPE_MOD", "DATA_LENGTH", "CHAR_LENGTH", "DATA_PRECISION",
		"DATA_SCALE", "CHAR_USED", "NULLABLE", "DATA_DEFAULT_VC",
	}))

	require.NoError(t, c.SchemaCollection())
	assert.NoError(t, dbMock.ExpectationsWereMet(), "the query actually sent to Oracle must carry the substituted REGEXP_LIKE filter")
}

// TestViewCollectionAppliesTableIncludeExcludeFilters guards the fix that extended
// include_tables/exclude_tables to views: viewsQueryTemplate now carries the same
// /*TABLE_FILTERS*/ placeholder as schemasQueryTemplate, substituted against v.view_name (the
// column the views query actually names its rows with, not t.table_name).
func TestViewCollectionAppliesTableIncludeExcludeFilters(t *testing.T) {
	c, _, dbMock, closeDB := newSchemaCheck(t)
	defer closeDB()
	dbMock.MatchExpectationsInOrder(false)

	c.config.Schemas.IncludeTables = []string{"^ORD"}
	c.config.Schemas.ExcludeTables = []string{"^TMP_"}

	dbMock.ExpectQuery(`v\$containers`).WillReturnRows(sqlmock.NewRows([]string{"CON_ID", "NAME"}))
	dbMock.ExpectQuery("cdb_users").WillReturnRows(
		sqlmock.NewRows([]string{"CON_ID", "USERNAME", "USER_ID"}).AddRow(3, "APP", 104))

	tableFilter := regexSQLClauses("t.table_name", c.config.Schemas.IncludeTables, c.config.Schemas.ExcludeTables)
	dbMock.ExpectQuery(regexp.QuoteMeta(tableFilter)).WillReturnRows(sqlmock.NewRows([]string{
		"CON_ID", "OWNER", "TABLE_NAME", "TEMPORARY", "DURATION", "EXTERNAL", "IOT_TYPE",
		"PARTITIONED", "CLUSTER_NAME", "CLUSTERING", "READ_ONLY", "NUM_ROWS", "LAST_ANALYZED",
		"OBJECT_TYPE_OWNER", "OBJECT_TYPE", "TOTAL_TABLES",
		"COLUMN_NAME", "COLUMN_ID", "INTERNAL_COLUMN_ID", "VIRTUAL_COLUMN", "HIDDEN_COLUMN", "DATA_TYPE",
		"DATA_TYPE_OWNER", "DATA_TYPE_MOD", "DATA_LENGTH", "CHAR_LENGTH", "DATA_PRECISION",
		"DATA_SCALE", "CHAR_USED", "NULLABLE", "DATA_DEFAULT_VC",
	}))

	viewFilter := regexSQLClauses("v.view_name", c.config.Schemas.IncludeTables, c.config.Schemas.ExcludeTables)
	require.Contains(t, viewFilter, "REGEXP_LIKE")
	dbMock.ExpectQuery(regexp.QuoteMeta(viewFilter)).WillReturnRows(sqlmock.NewRows([]string{
		"CON_ID", "OWNER", "TABLE_NAME", "TEMPORARY", "DURATION", "EXTERNAL", "IOT_TYPE",
		"PARTITIONED", "CLUSTER_NAME", "CLUSTERING", "READ_ONLY", "NUM_ROWS", "LAST_ANALYZED",
		"COLUMN_NAME", "COLUMN_ID", "VIRTUAL_COLUMN", "HIDDEN_COLUMN", "DATA_TYPE",
		"DATA_TYPE_OWNER", "DATA_TYPE_MOD", "DATA_LENGTH", "CHAR_LENGTH", "DATA_PRECISION",
		"DATA_SCALE", "CHAR_USED", "NULLABLE", "DATA_DEFAULT_VC",
	}))

	require.NoError(t, c.SchemaCollection())
	assert.NoError(t, dbMock.ExpectationsWereMet(), "the views query actually sent to Oracle must carry the substituted REGEXP_LIKE filter")
}

// TestViewCollectionFilteredViewNotCountedAsTruncated guards the distinction the fix must
// preserve: a view dropped by exclude_tables at the SQL layer never reaches viewKeysWithCap, so
// it must not be mistaken for a view dropped by max_views. Here max_views is set to exactly the
// number of rows the (already filtered) query returns, so if the exclusion were instead applied
// client-side after the cap check, or double-counted somehow, this container would come back
// marked truncated.
func TestViewCollectionFilteredViewNotCountedAsTruncated(t *testing.T) {
	db, dbMock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()
	dbMock.MatchExpectationsInOrder(false)

	c, sender := newDbDoesNotExistCheck(t, "", "")
	c.db = sqlx.NewDb(db, "sqlmock")
	c.clock = clock.NewMock()
	c.dbVersion = "23.26.2.0.0"
	c.config.Schemas.Enabled = true
	c.config.Schemas.CollectionInterval = 600
	c.config.Schemas.PayloadChunkSize = 100
	c.config.Schemas.MaxViews = 1
	c.config.Schemas.ExcludeTables = []string{"^TMP_"}

	viewRelationColumns := []string{
		"CON_ID", "OWNER", "TABLE_NAME", "TEMPORARY", "DURATION", "EXTERNAL", "IOT_TYPE",
		"PARTITIONED", "CLUSTER_NAME", "CLUSTERING", "READ_ONLY", "NUM_ROWS", "LAST_ANALYZED",
		"COLUMN_NAME", "COLUMN_ID", "VIRTUAL_COLUMN", "HIDDEN_COLUMN", "DATA_TYPE",
		"DATA_TYPE_OWNER", "DATA_TYPE_MOD", "DATA_LENGTH", "CHAR_LENGTH", "DATA_PRECISION",
		"DATA_SCALE", "CHAR_USED", "NULLABLE", "DATA_DEFAULT_VC",
	}

	dbMock.ExpectQuery(`v\$containers`).WillReturnRows(
		sqlmock.NewRows([]string{"CON_ID", "NAME"}).AddRow(3, "APP_PDB"))
	dbMock.ExpectQuery("cdb_users").WillReturnRows(
		sqlmock.NewRows([]string{"CON_ID", "USERNAME", "USER_ID"}).AddRow(3, "APP", 104))
	dbMock.ExpectQuery("cdb_tables").WillReturnRows(sqlmock.NewRows([]string{
		"CON_ID", "OWNER", "TABLE_NAME", "TEMPORARY", "DURATION", "EXTERNAL", "IOT_TYPE",
		"PARTITIONED", "CLUSTER_NAME", "CLUSTERING", "READ_ONLY", "NUM_ROWS", "LAST_ANALYZED",
		"OBJECT_TYPE_OWNER", "OBJECT_TYPE", "TOTAL_TABLES",
		"COLUMN_NAME", "COLUMN_ID", "INTERNAL_COLUMN_ID", "VIRTUAL_COLUMN", "HIDDEN_COLUMN", "DATA_TYPE",
		"DATA_TYPE_OWNER", "DATA_TYPE_MOD", "DATA_LENGTH", "CHAR_LENGTH", "DATA_PRECISION",
		"DATA_SCALE", "CHAR_USED", "NULLABLE", "DATA_DEFAULT_VC",
	}))
	// Simulates what Oracle's REGEXP_LIKE(v.view_name, '^TMP_') filter does server-side: the
	// excluded view (TMP_ORDERS_V) never appears in the result set at all, only the surviving one.
	dbMock.ExpectQuery("cdb_views").WillReturnRows(
		sqlmock.NewRows(viewRelationColumns).AddRow(
			3, "APP", "V_ORDERS", "N", "-", "NO", "-", "NO", "-", "NO", "NO", nil, nil,
			"ORDER_ID", 1, "NO", "NO", "NUMBER", nil, nil, 22, nil, 12, 0, "-", "N", nil,
		))

	require.NoError(t, c.SchemaCollection())

	var views schemaEvent
	found := false
	for _, call := range sender.Calls {
		if call.Method != "EventPlatformEvent" {
			continue
		}
		var e schemaEvent
		require.NoError(t, json.Unmarshal(call.Arguments.Get(0).([]byte), &e))
		if e.Kind == "oracle_views" {
			views = e
			found = true
		}
	}
	require.True(t, found, "no oracle_views payload was emitted")

	require.Len(t, views.Metadata[0].Schemas, 1)
	require.Len(t, views.Metadata[0].Schemas[0].Views, 1)
	assert.Equal(t, "V_ORDERS", views.Metadata[0].Schemas[0].Views[0].Name)
	assert.False(t, views.Truncated,
		"a container fully accounted for by exclude_tables plus the surviving view must not be flagged as truncated by max_views")
}

// viewRelationColumns is the column shape cdb_views rows come back with (see
// TestViewCollectionEmitsSeparateKind); it is shared by the max_views tests below since each
// needs to fabricate several view rows.
var viewRelationColumns = []string{
	"CON_ID", "OWNER", "TABLE_NAME", "TEMPORARY", "DURATION", "EXTERNAL", "IOT_TYPE",
	"PARTITIONED", "CLUSTER_NAME", "CLUSTERING", "READ_ONLY", "NUM_ROWS", "LAST_ANALYZED",
	"COLUMN_NAME", "COLUMN_ID", "VIRTUAL_COLUMN", "HIDDEN_COLUMN", "DATA_TYPE",
	"DATA_TYPE_OWNER", "DATA_TYPE_MOD", "DATA_LENGTH", "CHAR_LENGTH", "DATA_PRECISION",
	"DATA_SCALE", "CHAR_USED", "NULLABLE", "DATA_DEFAULT_VC",
}

// addViewRow appends one cdb_views row for the given container/owner/view to rows, mirroring
// the AddRow calls in TestViewCollectionEmitsSeparateKind.
func addViewRow(rows *sqlmock.Rows, conID int64, owner, viewName string) *sqlmock.Rows {
	return rows.AddRow(
		conID, owner, viewName, "N", "-", "NO", "-", "NO", "-", "NO", "NO", nil, nil,
		"C1", 1, "NO", "NO", "NUMBER", nil, nil, 22, nil, 12, 0, "-", "N", nil,
	)
}

// emptyTablesRows returns an empty cdb_tables/cdb_object_tables result set, used by the
// max_views tests below to keep table collection out of the way of the view assertions.
func emptyTablesRows() *sqlmock.Rows {
	return sqlmock.NewRows([]string{
		"CON_ID", "OWNER", "TABLE_NAME", "TEMPORARY", "DURATION", "EXTERNAL", "IOT_TYPE",
		"PARTITIONED", "CLUSTER_NAME", "CLUSTERING", "READ_ONLY", "NUM_ROWS", "LAST_ANALYZED",
		"OBJECT_TYPE_OWNER", "OBJECT_TYPE", "TOTAL_TABLES",
		"COLUMN_NAME", "COLUMN_ID", "INTERNAL_COLUMN_ID", "VIRTUAL_COLUMN", "HIDDEN_COLUMN", "DATA_TYPE",
		"DATA_TYPE_OWNER", "DATA_TYPE_MOD", "DATA_LENGTH", "CHAR_LENGTH", "DATA_PRECISION",
		"DATA_SCALE", "CHAR_USED", "NULLABLE", "DATA_DEFAULT_VC",
	})
}

// viewPayloadsByContainer runs the check and returns every emitted oracle_views payload keyed
// by container ID, for tests that need to inspect more than one container's snapshot.
func viewPayloadsByContainer(t *testing.T, sender *mock.Mock) map[string]schemaEvent {
	t.Helper()
	byContainer := make(map[string]schemaEvent)
	for _, call := range sender.Calls {
		if call.Method != "EventPlatformEvent" {
			continue
		}
		var e schemaEvent
		require.NoError(t, json.Unmarshal(call.Arguments.Get(0).([]byte), &e))
		if e.Kind != "oracle_views" {
			continue
		}
		require.Len(t, e.Metadata, 1)
		byContainer[e.Metadata[0].ID] = e
	}
	return byContainer
}

// TestMaxViewsCapsViewsAndFlagsTruncation covers viewKeysWithCap's basic role: a container with
// more views than max_views must have its oracle_views payload capped at max_views views and
// marked truncated.
func TestMaxViewsCapsViewsAndFlagsTruncation(t *testing.T) {
	db, dbMock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()
	dbMock.MatchExpectationsInOrder(false)

	c, sender := newDbDoesNotExistCheck(t, "", "")
	c.db = sqlx.NewDb(db, "sqlmock")
	c.clock = clock.NewMock()
	c.dbVersion = "23.26.2.0.0"
	c.config.Schemas.Enabled = true
	c.config.Schemas.CollectionInterval = 600
	c.config.Schemas.PayloadChunkSize = 100
	c.config.Schemas.MaxViews = 2

	dbMock.ExpectQuery(`v\$containers`).WillReturnRows(
		sqlmock.NewRows([]string{"CON_ID", "NAME"}).AddRow(3, "APP_PDB"))
	dbMock.ExpectQuery("cdb_users").WillReturnRows(
		sqlmock.NewRows([]string{"CON_ID", "USERNAME", "USER_ID"}).AddRow(3, "APP", 104))
	dbMock.ExpectQuery("cdb_tables").WillReturnRows(emptyTablesRows())

	rows := sqlmock.NewRows(viewRelationColumns)
	addViewRow(rows, 3, "APP", "V1")
	addViewRow(rows, 3, "APP", "V2")
	addViewRow(rows, 3, "APP", "V3")
	dbMock.ExpectQuery("cdb_views").WillReturnRows(rows)

	require.NoError(t, c.SchemaCollection())

	byContainer := viewPayloadsByContainer(t, &sender.Mock)
	require.Contains(t, byContainer, "3")
	views := byContainer["3"]
	require.Len(t, views.Metadata[0].Schemas, 1)
	assert.Len(t, views.Metadata[0].Schemas[0].Views, 2, "max_views=2 must cap the container at 2 views")
	assert.True(t, views.Truncated, "a container with more views than max_views must be marked truncated")
}

// TestMaxViewsNotTruncatedWhenUnderCap mirrors TestMaxTablesNotTruncatedWhenUnderCap for views.
func TestMaxViewsNotTruncatedWhenUnderCap(t *testing.T) {
	db, dbMock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()
	dbMock.MatchExpectationsInOrder(false)

	c, sender := newDbDoesNotExistCheck(t, "", "")
	c.db = sqlx.NewDb(db, "sqlmock")
	c.clock = clock.NewMock()
	c.dbVersion = "23.26.2.0.0"
	c.config.Schemas.Enabled = true
	c.config.Schemas.CollectionInterval = 600
	c.config.Schemas.PayloadChunkSize = 100
	c.config.Schemas.MaxViews = 50

	dbMock.ExpectQuery(`v\$containers`).WillReturnRows(
		sqlmock.NewRows([]string{"CON_ID", "NAME"}).AddRow(3, "APP_PDB"))
	dbMock.ExpectQuery("cdb_users").WillReturnRows(
		sqlmock.NewRows([]string{"CON_ID", "USERNAME", "USER_ID"}).AddRow(3, "APP", 104))
	dbMock.ExpectQuery("cdb_tables").WillReturnRows(emptyTablesRows())

	rows := sqlmock.NewRows(viewRelationColumns)
	addViewRow(rows, 3, "APP", "V1")
	dbMock.ExpectQuery("cdb_views").WillReturnRows(rows)

	require.NoError(t, c.SchemaCollection())

	byContainer := viewPayloadsByContainer(t, &sender.Mock)
	require.Contains(t, byContainer, "3")
	views := byContainer["3"]
	require.Len(t, views.Metadata[0].Schemas, 1)
	assert.Len(t, views.Metadata[0].Schemas[0].Views, 1)
	assert.False(t, views.Truncated, "a container with fewer views than max_views must not be marked truncated")
}

// TestMaxViewsNotTruncatedWhenExactlyAtCap guards the boundary, mirroring
// TestMaxColumnsNotTruncatedWhenExactlyAtCap: a container with exactly max_views views must not
// be reported as truncated, since none were actually dropped.
func TestMaxViewsNotTruncatedWhenExactlyAtCap(t *testing.T) {
	db, dbMock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()
	dbMock.MatchExpectationsInOrder(false)

	c, sender := newDbDoesNotExistCheck(t, "", "")
	c.db = sqlx.NewDb(db, "sqlmock")
	c.clock = clock.NewMock()
	c.dbVersion = "23.26.2.0.0"
	c.config.Schemas.Enabled = true
	c.config.Schemas.CollectionInterval = 600
	c.config.Schemas.PayloadChunkSize = 100
	c.config.Schemas.MaxViews = 2

	dbMock.ExpectQuery(`v\$containers`).WillReturnRows(
		sqlmock.NewRows([]string{"CON_ID", "NAME"}).AddRow(3, "APP_PDB"))
	dbMock.ExpectQuery("cdb_users").WillReturnRows(
		sqlmock.NewRows([]string{"CON_ID", "USERNAME", "USER_ID"}).AddRow(3, "APP", 104))
	dbMock.ExpectQuery("cdb_tables").WillReturnRows(emptyTablesRows())

	rows := sqlmock.NewRows(viewRelationColumns)
	addViewRow(rows, 3, "APP", "V1")
	addViewRow(rows, 3, "APP", "V2")
	dbMock.ExpectQuery("cdb_views").WillReturnRows(rows)

	require.NoError(t, c.SchemaCollection())

	byContainer := viewPayloadsByContainer(t, &sender.Mock)
	require.Contains(t, byContainer, "3")
	views := byContainer["3"]
	require.Len(t, views.Metadata[0].Schemas, 1)
	assert.Len(t, views.Metadata[0].Schemas[0].Views, 2)
	assert.False(t, views.Truncated, "a container with exactly max_views views must not be marked truncated")
}

// TestMaxViewsCapEnforcedPerContainerIndependently is the regression test for the real bug:
// max_views was once enforced against a running total that was never reset per container, so a
// first container that used up the whole budget left later containers with zero views. Container
// 3 alone exceeds max_views and must be capped and truncated; container 4 stays under max_views
// on its own and must get every one of its views, none of them starved by container 3's usage.
// Under the old global counter, container 4 would see the counter already at max_views from
// container 3 and would come back with zero views and (wrongly) truncated=true.
func TestMaxViewsCapEnforcedPerContainerIndependently(t *testing.T) {
	db, dbMock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()
	dbMock.MatchExpectationsInOrder(false)

	c, sender := newDbDoesNotExistCheck(t, "", "")
	c.db = sqlx.NewDb(db, "sqlmock")
	c.clock = clock.NewMock()
	c.dbVersion = "23.26.2.0.0"
	c.config.Schemas.Enabled = true
	c.config.Schemas.CollectionInterval = 600
	c.config.Schemas.PayloadChunkSize = 100
	c.config.Schemas.MaxViews = 2

	dbMock.ExpectQuery(`v\$containers`).WillReturnRows(
		sqlmock.NewRows([]string{"CON_ID", "NAME"}).AddRow(3, "APP_PDB1").AddRow(4, "APP_PDB2"))
	dbMock.ExpectQuery("cdb_users").WillReturnRows(
		sqlmock.NewRows([]string{"CON_ID", "USERNAME", "USER_ID"}).
			AddRow(3, "APP", 104).AddRow(4, "APP", 105))
	dbMock.ExpectQuery("cdb_tables").WillReturnRows(emptyTablesRows())

	rows := sqlmock.NewRows(viewRelationColumns)
	addViewRow(rows, 3, "APP", "V1")
	addViewRow(rows, 3, "APP", "V2")
	addViewRow(rows, 3, "APP", "V3")
	addViewRow(rows, 4, "APP", "V4")
	addViewRow(rows, 4, "APP", "V5")
	dbMock.ExpectQuery("cdb_views").WillReturnRows(rows)

	require.NoError(t, c.SchemaCollection())

	byContainer := viewPayloadsByContainer(t, &sender.Mock)
	require.Contains(t, byContainer, "3")
	require.Contains(t, byContainer, "4")

	first := byContainer["3"]
	require.Len(t, first.Metadata[0].Schemas, 1)
	assert.Len(t, first.Metadata[0].Schemas[0].Views, 2, "container 3 alone exceeds max_views and must be capped")
	assert.True(t, first.Truncated, "container 3 hit max_views and must be marked truncated")

	second := byContainer["4"]
	require.Len(t, second.Metadata[0].Schemas, 1)
	assert.Len(t, second.Metadata[0].Schemas[0].Views, 2,
		"container 4's own views must not be starved by container 3 having already used up max_views")
	assert.False(t, second.Truncated, "container 4 is under max_views on its own and must not be marked truncated")
}
