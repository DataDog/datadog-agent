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
	"errors"
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

type deadlineCountingContext struct {
	context.Context
	deadlines int
}

func (c *deadlineCountingContext) Deadline() (time.Time, bool) {
	c.deadlines++
	return time.Time{}, false
}

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

	ctx := &deadlineCountingContext{Context: context.Background()}
	require.NoError(t, c.schemaCollection(ctx))
	assert.Equal(t, 2, ctx.deadlines, "container and owner queries must each create their own timeout")
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
			// Legacy character columns can report CHAR_USED='-'; treat them as byte semantics.
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

	for _, v := range []string{"19.21.0.0.0", "21.3.0.0.0", "12.2.0.1.0"} {
		c.dbVersion = v
		assert.Equal(t, "c.data_default", c.defaultValueColumn(), "version %s", v)
	}
}

func TestMainQueriesDoNotProjectLongDefaults(t *testing.T) {
	assert.NotContains(t, schemasQueryTemplate, "/*DEFAULT_COL*/")
	assert.NotContains(t, viewsQueryTemplate, "/*DEFAULT_COL*/")
	assert.NotContains(t, schemasQueryTemplate, "c.data_default AS")
	assert.NotContains(t, viewsQueryTemplate, "c.data_default AS")
}

func TestRelationFilterChunks(t *testing.T) {
	allowed := map[tableKey]struct{}{
		{conID: 3, owner: "APP", table: "USERS"}:    {},
		{conID: 3, owner: "APP", table: "O'RDER"}:   {},
		{conID: 4, owner: "REPORT", table: "DAILY"}: {},
	}
	columns := relationColumnNames{conID: "x.con_id", owner: "x.owner", relation: "x.table_name"}

	assert.Equal(t, []string{
		"((x.con_id = 3 AND x.owner = 'APP' AND x.table_name IN ('O''RDER', 'USERS')) OR " +
			"(x.con_id = 4 AND x.owner = 'REPORT' AND x.table_name IN ('DAILY')))",
	}, relationFilterChunks(allowed, columns))
}

func TestColumnFilterChunksIncludesOnlySelectedColumns(t *testing.T) {
	table := tableKey{conID: 3, owner: "APP", table: "ORDERS"}
	allowed := map[columnKey]struct{}{
		{tableKey: table, column: "STATUS"}: {},
		{tableKey: table, column: "ID"}:     {},
	}
	columns := relationColumnNames{conID: "c.con_id", owner: "c.owner", relation: "c.table_name"}

	assert.Equal(t, []string{
		"((c.con_id = 3 AND c.owner = 'APP' AND c.table_name = 'ORDERS' AND c.column_name IN ('ID', 'STATUS')))",
	}, columnFilterChunks(allowed, columns, "c.column_name"))
}

func TestRelationFilterChunksAtOracleLimit(t *testing.T) {
	allowed := make(map[tableKey]struct{}, maxSchemaRelationsPerQuery+1)
	for i := 0; i <= maxSchemaRelationsPerQuery; i++ {
		allowed[tableKey{conID: 3, owner: "APP", table: fmt.Sprintf("T%04d", i)}] = struct{}{}
	}

	filters := relationFilterChunks(allowed, relationColumnNames{conID: "con_id", owner: "owner", relation: "table_name"})
	require.Len(t, filters, 2)
	assert.NotContains(t, filters[0], "T1000")
	assert.Contains(t, filters[1], "T1000")
}

func TestCapMetadataRowsAcrossOwnerBatches(t *testing.T) {
	rows := []schemaRowDB{
		{ConID: 3, Owner: "A", TableName: "T1", ColumnName: "C1", TotalTables: sql.NullInt64{Int64: 1, Valid: true}},
		{ConID: 3, Owner: "A", TableName: "T1", ColumnName: "C2", TotalTables: sql.NullInt64{Int64: 1, Valid: true}},
		{ConID: 3, Owner: "B", TableName: "T2", ColumnName: "C1", TotalTables: sql.NullInt64{Int64: 1, Valid: true}},
		{ConID: 3, Owner: "C", TableName: "T3", ColumnName: "C1", TotalTables: sql.NullInt64{Int64: 1, Valid: true}},
		{ConID: 4, Owner: "D", TableName: "T4", ColumnName: "C1", TotalTables: sql.NullInt64{Int64: 1, Valid: true}},
		{ConID: 4, Owner: "E", TableName: "T5", ColumnName: "C1", TotalTables: sql.NullInt64{Int64: 1, Valid: true}},
	}

	capped, truncated := capMetadataRows(rows, 2)
	require.Len(t, capped, 5)
	assert.Equal(t, []string{"T1", "T1", "T2", "T4", "T5"}, []string{
		capped[0].TableName, capped[1].TableName, capped[2].TableName, capped[3].TableName, capped[4].TableName,
	})
	assert.Contains(t, truncated, int64(3))
	assert.NotContains(t, truncated, int64(4))
}

func TestSchemaCollectionCapsAcrossOwnerQueryBatches(t *testing.T) {
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
	c.config.Schemas.MaxTables = 1
	collectViews := false
	c.config.Schemas.CollectViews = &collectViews

	dbMock.ExpectQuery(`v\$containers`).WillReturnRows(
		sqlmock.NewRows([]string{"CON_ID", "NAME"}).AddRow(3, "APP_PDB"))
	ownerRows := sqlmock.NewRows([]string{"CON_ID", "USERNAME", "USER_ID"})
	for i := 0; i <= maxSchemaOwners; i++ {
		ownerRows.AddRow(3, fmt.Sprintf("APP%04d", i), 1000+i)
	}
	dbMock.ExpectQuery("cdb_users").WillReturnRows(ownerRows)
	dbMock.ExpectQuery("APP0999").WillReturnRows(addTableRow(emptyTablesRows(), 3, "APP0000", "T1", 1))
	dbMock.ExpectQuery("APP1000").WillReturnRows(addTableRow(emptyTablesRows(), 3, "APP1000", "T2", 1))

	require.NoError(t, c.SchemaCollection())
	assert.NoError(t, dbMock.ExpectationsWereMet())

	var event schemaEvent
	for _, call := range sender.Calls {
		if call.Method == "EventPlatformEvent" {
			require.NoError(t, json.Unmarshal(call.Arguments.Get(0).([]byte), &event))
			break
		}
	}
	require.Len(t, event.Metadata, 1)
	require.Len(t, event.Metadata[0].Schemas, 1)
	require.Len(t, event.Metadata[0].Schemas[0].Tables, 1)
	assert.Equal(t, "T1", event.Metadata[0].Schemas[0].Tables[0].Name)
	assert.True(t, event.Truncated)
}

func TestSchemaCollectionRequiresKnownOracle12OrLater(t *testing.T) {
	for _, version := range []string{"11.2.0.4.0", "", "unknown"} {
		t.Run(version, func(t *testing.T) {
			c, _, dbMock, closeDB := newSchemaCheck(t)
			defer closeDB()
			c.dbVersion = version

			require.NoError(t, c.SchemaCollection())
			assert.NoError(t, dbMock.ExpectationsWereMet())
		})
	}

	assert.True(t, schemaCollectionVersionSupported("12.1.0.2.0"))
	assert.True(t, schemaCollectionVersionSupported("23.26.2.0.0"))
}

func TestQueryMetadataReturnsIterationError(t *testing.T) {
	c, _, dbMock, closeDB := newSchemaCheck(t)
	defer closeDB()

	iterationErr := errors.New("iteration failed")
	dbMock.ExpectQuery("SELECT value").WillReturnRows(
		sqlmock.NewRows([]string{"VALUE"}).AddRow(1).RowError(0, iterationErr))

	err := c.queryMetadata(context.Background(), "SELECT value", func(rows *sqlx.Rows) error {
		var value int
		return rows.Scan(&value)
	})
	require.ErrorIs(t, err, iterationErr)
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

	// Both containers share a clock tick; snapshot IDs must still be unique.
	assert.NotEqual(t, payloads[0].CollectionStartedAt, payloads[1].CollectionStartedAt,
		"containers must not share a snapshot identifier")
}

func TestEmitSchemaSnapshotEventsCombinesKindsPerContainer(t *testing.T) {
	events := []schemaEvent{
		{Kind: "oracle_databases", CollectionStartedAt: 100, CollectionPayloadsCount: 1, Metadata: []containerObject{{ID: "1"}}},
		{Kind: "oracle_databases", CollectionStartedAt: 200, CollectionPayloadsCount: 1, Metadata: []containerObject{{ID: "3"}}},
		{Kind: "oracle_views", CollectionStartedAt: 300, CollectionPayloadsCount: 1, Metadata: []containerObject{{ID: "1"}}},
		{Kind: "oracle_views", CollectionStartedAt: 400, CollectionPayloadsCount: 1, Metadata: []containerObject{{ID: "3"}}},
	}

	var emitted []schemaEvent
	require.NoError(t, emitSchemaSnapshotEvents(events, true, func(payload []byte) {
		var event schemaEvent
		require.NoError(t, json.Unmarshal(payload, &event))
		emitted = append(emitted, event)
	}))

	require.Len(t, emitted, 4)
	assert.Equal(t, int64(100), emitted[0].CollectionStartedAt)
	assert.Equal(t, int64(200), emitted[1].CollectionStartedAt)
	assert.Equal(t, int64(100), emitted[2].CollectionStartedAt)
	assert.Equal(t, int64(200), emitted[3].CollectionStartedAt)
	assert.Zero(t, emitted[0].CollectionPayloadsCount)
	assert.Zero(t, emitted[1].CollectionPayloadsCount)
	assert.Equal(t, 2, emitted[2].CollectionPayloadsCount)
	assert.Equal(t, 2, emitted[3].CollectionPayloadsCount)
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
	collectViews := false
	c.config.Schemas.CollectViews = &collectViews

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

	// Leave detail queries unprimed to simulate missing grants.
	require.NoError(t, c.SchemaCollection())

	sender.AssertNumberOfCalls(t, "EventPlatformEvent", 1)
	sender.AssertCalled(t, "EventPlatformEvent", mock.Anything, "dbm-metadata")
	sender.AssertNumberOfCalls(t, "Commit", 1)

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

func TestTableDetailsIndexesGroupByName(t *testing.T) {
	c, _, dbMock, closeDB := newSchemaCheck(t)
	defer closeDB()

	dbMock.MatchExpectationsInOrder(false)
	dbMock.ExpectQuery(`(?s)cdb_indexes.*i\.con_id = 3 AND i\.table_owner = 'APP' AND i\.table_name IN \('ORDERS'\)`).WillReturnRows(
		sqlmock.NewRows([]string{"CON_ID", "TABLE_OWNER", "TABLE_NAME", "INDEX_NAME", "UNIQUENESS", "INDEX_TYPE", "COLUMN_NAME", "COLUMN_EXPRESSION"}).
			AddRow(3, "APP", "ORDERS", "ORDERS_COMPOSITE_IDX", "UNIQUE", "NORMAL", "STATUS", nil).
			AddRow(3, "APP", "ORDERS", "ORDERS_COMPOSITE_IDX", "UNIQUE", "NORMAL", "CREATED_AT", nil).
			AddRow(3, "APP", "ORDERS", "ORDERS_STATUS_IDX", "NONUNIQUE", "NORMAL", "STATUS", nil))

	allowed := map[tableKey]struct{}{{conID: 3, owner: "APP", table: "ORDERS"}: {}}
	details := c.tableDetails(context.Background(), allowed, nil)

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
	details := c.tableDetails(context.Background(), allowed, nil)

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
	details := c.tableDetails(context.Background(), allowed, nil)

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
	details := c.tableDetails(context.Background(), allowed, nil)

	fk := details[tableKey{conID: 3, owner: "APP", table: "ORDER_ITEMS"}].Constraints[0]
	assert.Empty(t, fk.ReferencedTable, "the referencing owner was never scanned, so the table cannot be resolved")
	assert.Equal(t, "ORDERS_PK", fk.ReferencedConstraint, "falls back to naming the constraint instead of looking corrupt")
}

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
	details := c.tableDetails(context.Background(), allowed, nil)

	con := details[tableKey{conID: 3, owner: "APP", table: "ORDERS"}].Constraints[0]
	assert.Equal(t, "check", con.Type, "constraintType must map C to \"check\", not pass through the raw code")
	assert.Equal(t, "status IN ('NEW','SHIPPED')", con.Condition)
	assert.Empty(t, con.ReferencedTable, "a check constraint has no referenced table")
}

func TestTableDetailsPartitionKeyJoin(t *testing.T) {
	c, _, dbMock, closeDB := newSchemaCheck(t)
	defer closeDB()

	dbMock.MatchExpectationsInOrder(false)
	// The LEFT JOIN repeats the table fields for each partition key column.
	dbMock.ExpectQuery("cdb_part_tables").WillReturnRows(
		sqlmock.NewRows([]string{"CON_ID", "OWNER", "TABLE_NAME", "PARTITIONING_TYPE", "SUBPARTITIONING_TYPE", "PARTITION_COUNT", "COLUMN_NAME"}).
			AddRow(3, "APP", "EVENTS", "RANGE", "NONE", 4, "EVENT_DATE").
			AddRow(3, "APP", "EVENTS", "RANGE", "NONE", 4, "REGION"))

	allowed := map[tableKey]struct{}{{conID: 3, owner: "APP", table: "EVENTS"}: {}}
	details := c.tableDetails(context.Background(), allowed, nil)

	p := details[tableKey{conID: 3, owner: "APP", table: "EVENTS"}].Partitioned
	require.NotNil(t, p)
	assert.Equal(t, int64(4), p.NumPartitions, "the repeated table row must not accumulate")
	assert.Empty(t, p.SubpartitionsType, "NONE must not surface as a subpartitioning type")
	assert.Equal(t, "RANGE (EVENT_DATE, REGION)", p.PartitionKey)
}

func TestTableDetailsPartitionedWithoutKeyColumns(t *testing.T) {
	c, _, dbMock, closeDB := newSchemaCheck(t)
	defer closeDB()

	dbMock.MatchExpectationsInOrder(false)
	dbMock.ExpectQuery("cdb_part_tables").WillReturnRows(
		sqlmock.NewRows([]string{"CON_ID", "OWNER", "TABLE_NAME", "PARTITIONING_TYPE", "SUBPARTITIONING_TYPE", "PARTITION_COUNT", "COLUMN_NAME"}).
			AddRow(3, "APP", "EVENTS", "HASH", "NONE", 8, nil))

	allowed := map[tableKey]struct{}{{conID: 3, owner: "APP", table: "EVENTS"}: {}}
	details := c.tableDetails(context.Background(), allowed, nil)

	p := details[tableKey{conID: 3, owner: "APP", table: "EVENTS"}].Partitioned
	require.NotNil(t, p, "a NULL key column must not discard the partitioning detail")
	assert.Equal(t, "HASH", p.PartitioningType)
	assert.Equal(t, int64(8), p.NumPartitions)
	assert.Empty(t, p.PartitionKey)
}

func TestTableDetailsExternalLocationsConcat(t *testing.T) {
	c, _, dbMock, closeDB := newSchemaCheck(t)
	defer closeDB()

	dbMock.MatchExpectationsInOrder(false)
	dbMock.ExpectQuery("cdb_external_tables").WillReturnRows(
		sqlmock.NewRows([]string{"CON_ID", "OWNER", "TABLE_NAME", "TYPE_NAME", "NVL(default_directory_name, '-')", "NVL(directory_name, '-')", "LOCATION"}).
			AddRow(3, "APP", "EXT_ORDERS", "ORACLE_LOADER", "DEFAULT_DIR", "LOAD_DIR", "orders_2024.csv").
			AddRow(3, "APP", "EXT_ORDERS", "ORACLE_LOADER", "DEFAULT_DIR", "-", "orders_fallback.csv"))

	allowed := map[tableKey]struct{}{{conID: 3, owner: "APP", table: "EXT_ORDERS"}: {}}
	details := c.tableDetails(context.Background(), allowed, nil)

	ext := details[tableKey{conID: 3, owner: "APP", table: "EXT_ORDERS"}].External
	require.NotNil(t, ext)
	assert.Equal(t, "ORACLE_LOADER", ext.AccessDriver)
	assert.Equal(t, "DEFAULT_DIR", ext.Directory)
	require.Len(t, ext.Locations, 2)
	assert.Equal(t, "LOAD_DIR:orders_2024.csv", ext.Locations[0])
	assert.Equal(t, "orders_fallback.csv", ext.Locations[1],
		"a NULL directory_name must not be concatenated as a literal '-:' prefix")
}

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
	details := c.tableDetails(context.Background(), allowed, nil)

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

// Multiple columns per table ensure chunking happens at table boundaries, not row boundaries.

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

// A partial snapshot has no completion marker, so scan errors must emit no payload.

func TestSchemaCollectionScanErrorEmitsNoPayload(t *testing.T) {
	c, _, dbMock, closeDB := newSchemaCheck(t)
	defer closeDB()
	dbMock.MatchExpectationsInOrder(false)

	dbMock.ExpectQuery(`v\$containers`).WillReturnRows(
		sqlmock.NewRows([]string{"CON_ID", "NAME"}).AddRow(3, "APP_PDB"))
	dbMock.ExpectQuery("cdb_users").WillReturnRows(
		sqlmock.NewRows([]string{"CON_ID", "USERNAME", "USER_ID"}).AddRow(3, "APP", 104))

	// A NULL TABLE_NAME forces StructScan to fail.
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

	// One returned row with TOTAL_TABLES=2 represents a max_tables=1 result.
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

	// Two returned rows with TOTAL_COLUMNS=4 represent a max_columns=2 result.
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

// Owner names are interpolated into IN lists, so only unquoted Oracle identifiers are accepted.

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

func TestColumnDefaultTruncatedAtVarchar4000Cap(t *testing.T) {
	c, _, dbMock, closeDB := newSchemaCheck(t)
	defer closeDB()
	c.config.Schemas.PayloadChunkSize = 100
	dbMock.MatchExpectationsInOrder(false)

	table := tableKey{conID: 3, owner: "APP", table: "T"}
	allowed := map[tableKey]struct{}{table: {}}
	allowedColumns := map[columnKey]struct{}{{tableKey: table, column: "C1"}: {}}
	dbMock.ExpectQuery(`(?s)data_default_vc.*c\.column_name IN \('C1'\)`).WillReturnRows(
		sqlmock.NewRows([]string{"CON_ID", "OWNER", "TABLE_NAME", "COLUMN_NAME", "DATA_DEFAULT"}).
			AddRow(3, "APP", "T", "C1", strings.Repeat("x", 4500)))
	details := c.tableDetails(context.Background(), allowed, allowedColumns)

	var payloads []schemaEvent
	collector := newSchemaCollector(&c, func(b []byte) {
		var e schemaEvent
		require.NoError(t, json.Unmarshal(b, &e))
		payloads = append(payloads, e)
	}, details, map[ownerKey]string{}, map[int64]string{})

	collector.add(schemaRowDB{
		ConID: 3, Owner: "APP", TableName: "T", Temporary: "N", External: "NO",
		IotType: "-", ClusterName: "-", Partitioned: "NO",
		ColumnName: "C1", DataType: sql.NullString{String: "VARCHAR2", Valid: true},
		DataLength: sql.NullInt64{Int64: 4000, Valid: true}, CharLength: sql.NullInt64{Int64: 4000, Valid: true},
		CharUsed: "C", Nullable: "Y",
	})
	collector.finish()

	require.Len(t, payloads, 1)
	col := payloads[0].Metadata[0].Schemas[0].Tables[0].Columns[0]
	assert.Len(t, col.Default, 4000, "a default value beyond VARCHAR2(4000) must be truncated to exactly that cap")
	assert.NoError(t, dbMock.ExpectationsWereMet())
}

func TestPassesFilterExcludeWinsOverInclude(t *testing.T) {
	include := compiledPatterns([]string{"^APP.*"}, "", "include")
	exclude := compiledPatterns([]string{"^APP_TMP$"}, "", "exclude")

	assert.True(t, passesFilter("APP_ORDERS", include, exclude), "matches include, does not match exclude")
	assert.False(t, passesFilter("APP_TMP", include, exclude), "exclude wins even though it also matches include")
	assert.False(t, passesFilter("OTHER", include, exclude), "include is non-empty and OTHER matches none of it")
	assert.True(t, passesFilter("OTHER", nil, exclude), "an empty include list requires no match")
}

func TestFilterContainersAppliesIncludeExcludeDatabases(t *testing.T) {
	containers := map[int64]string{1: "CDB$ROOT", 3: "APP_PDB", 7: "REPORTING_PDB"}

	assert.Equal(t, containers, filterContainers(containers, nil, nil, ""),
		"no configured filters must return every container unchanged")

	filtered := filterContainers(containers, []string{"PDB$"}, nil, "")
	assert.Equal(t, map[int64]string{3: "APP_PDB", 7: "REPORTING_PDB"}, filtered)

	filtered = filterContainers(containers, []string{"PDB$"}, []string{"^APP"}, "")
	assert.Equal(t, map[int64]string{7: "REPORTING_PDB"}, filtered, "exclude must win over a broader include")
}

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

func addTableRow(rows *sqlmock.Rows, conID int64, owner, table string, totalTables int) *sqlmock.Rows {
	return rows.AddRow(
		conID, owner, table, "N", "-", "NO", "-", "NO", "-", "NO", "NO", nil, nil, "-", "-", totalTables,
		"C1", 1, 1, "NO", "NO", "NUMBER", nil, nil, 22, nil, 12, 0, "-", "Y", nil,
	)
}

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
