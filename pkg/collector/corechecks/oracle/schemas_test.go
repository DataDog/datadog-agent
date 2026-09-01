// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build oracle_test

package oracle

import (
	"database/sql"
	"encoding/json"
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

func TestSchemaCollectionNoOwnersSkips(t *testing.T) {
	c, _, dbMock, closeDB := newSchemaCheck(t)
	defer closeDB()

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

	// DATA_DEFAULT_VC does not exist before 23ai; selecting it there is ORA-00904.
	for _, v := range []string{"19.21.0.0.0", "21.3.0.0.0", "12.2.0.1.0"} {
		c.dbVersion = v
		assert.Equal(t, "CAST(NULL AS VARCHAR2(4000))", c.defaultValueColumn(), "version %s", v)
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
	for i, name := range []string{"T1", "T2", "T3", "T4", "T5"} {
		collector.add(schemaRowDB{
			ConID: 3, Owner: "APP", TableName: name, Temporary: "N", External: "NO",
			IotType: "-", ClusterName: "-", Partitioned: "NO",
			ColumnName: "C1", DataType: sql.NullString{String: "NUMBER", Valid: true},
			Nullable: "Y",
		})
		_ = i
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
