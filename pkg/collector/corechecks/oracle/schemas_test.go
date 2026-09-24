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

func schemaPayloadEmitter(_ *Check, emit payloadEmitter) schemaEventEmitter {
	return func(event schemaEvent) {
		payload, err := json.Marshal(event)
		if err == nil {
			emit(payload)
		}
	}
}

func emitSchemaSnapshotEvents(events []schemaEvent, complete bool, emit payloadEmitter) error {
	coordinator := newSchemaSnapshotCoordinator(emit)
	for _, event := range events {
		coordinator.add(event)
	}
	if complete {
		return coordinator.complete()
	}
	return coordinator.err
}

func newSchemaCollector(c *Check, emit payloadEmitter, details map[tableKey]*tableDetails, owners map[ownerKey]string, containers map[int64]string) *schemaCollector {
	return newSchemaEventCollector(c, schemaPayloadEmitter(c, emit), details, owners, containers)
}

func (c *Check) tableDetails(ctx context.Context, allowed map[tableKey]struct{}, allowedColumns map[columnKey]struct{}) map[tableKey]*tableDetails {
	return c.tableDetailsForPage(ctx, allowed, allowedColumns, allowed)
}

func (c *Check) hydrateTablePage(ctx context.Context, keys []tableKey, maxColumns int, add func(schemaRowDB)) error {
	rows, err := c.tablePageRows(ctx, keys, maxColumns)
	if err != nil {
		return err
	}
	for _, row := range rows {
		add(row)
	}
	return nil
}

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
	dbMock.ExpectQuery("APP0999").WillReturnRows(sqlmock.NewRows(
		[]string{"CON_ID", "OWNER", "TABLE_NAME"}).AddRow(3, "APP0000", "T1"))
	dbMock.ExpectQuery("APP1000").WillReturnRows(sqlmock.NewRows(
		[]string{"CON_ID", "OWNER", "TABLE_NAME"}).AddRow(3, "APP1000", "T2"))
	dbMock.ExpectQuery("cdb_tab_cols").WillReturnRows(addTableRow(emptyTablesRows(), 3, "APP0000", "T1", 0))

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
	c.schemaPayloadChunkSize = 2

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

func TestViewSnapshotChunkSize(t *testing.T) {
	for _, tc := range []struct {
		name     string
		override int
		want     int
	}{
		{name: "default", want: defaultSchemaPayloadChunkSize},
		{name: "negative override", override: -1, want: defaultSchemaPayloadChunkSize},
		{name: "positive override", override: 2, want: 2},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c, _, _, closeDB := newSchemaCheck(t)
			defer closeDB()
			c.schemaPayloadChunkSize = tc.override
			var events []schemaEvent
			collector := newViewEventCollector(&c, func(event schemaEvent) {
				events = append(events, event)
			}, nil, nil, nil)

			for i := 0; i <= tc.want; i++ {
				collector.addView(schemaRowDB{ConID: 3, Owner: "APP", TableName: fmt.Sprintf("V%04d", i)})
			}
			collector.finish()

			require.Len(t, events, 2)
			for i, event := range events {
				assert.Equal(t, "oracle_views", event.Kind)
				require.Len(t, event.Metadata, 1)
				require.Len(t, event.Metadata[0].Schemas, 1)
				assert.Empty(t, event.Metadata[0].Schemas[0].Tables)
				want := tc.want
				if i == 1 {
					want = 1
				}
				assert.Len(t, event.Metadata[0].Schemas[0].Views, want)
			}
			assert.Equal(t, events[0].CollectionStartedAt, events[1].CollectionStartedAt)
			assert.Zero(t, events[0].CollectionPayloadsCount)
			assert.Equal(t, 2, events[1].CollectionPayloadsCount)
		})
	}
}

func TestSnapshotPerContainer(t *testing.T) {
	c, _, _, closeDB := newSchemaCheck(t)
	defer closeDB()

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

func TestSchemaPayloadKeepsMaterializedViewFreshness(t *testing.T) {
	c, _, dbMock, closeDB := newSchemaCheck(t)
	defer closeDB()

	key := tableKey{conID: 3, owner: "APP", table: "MV_ORDERS"}
	refreshedAt := time.Date(2026, 9, 24, 12, 0, 0, 0, time.UTC)
	dbMock.ExpectQuery("FROM cdb_mviews WHERE").WillReturnRows(sqlmock.NewRows(
		[]string{"CON_ID", "OWNER", "MVIEW_NAME", "REFRESH_MODE", "REFRESH_METHOD", "STALENESS", "LAST_REFRESH_DATE"},
	).AddRow(3, "APP", "MV_ORDERS", "DEMAND", "COMPLETE", "FRESH", refreshedAt))
	details := c.tableDetails(context.Background(), map[tableKey]struct{}{key: {}}, nil)
	require.NoError(t, dbMock.ExpectationsWereMet())

	var payloads []schemaEvent
	collector := newSchemaCollector(&c, func(payload []byte) {
		for _, name := range []string{"num_rows", "row_count_estimate", "last_analyzed", "modifications_details"} {
			assert.NotContains(t, string(payload), `"`+name+`":`)
		}
		var event schemaEvent
		require.NoError(t, json.Unmarshal(payload, &event))
		payloads = append(payloads, event)
	}, details, nil, nil)
	collector.add(schemaRowDB{ConID: 3, Owner: "APP", TableName: "MV_ORDERS"})
	collector.finish()

	require.Len(t, payloads, 1)
	require.Len(t, payloads[0].Metadata, 1)
	require.Len(t, payloads[0].Metadata[0].Schemas, 1)
	require.Len(t, payloads[0].Metadata[0].Schemas[0].Tables, 1)
	table := payloads[0].Metadata[0].Schemas[0].Tables[0]
	assert.Equal(t, "materialized_view", table.TableType)
	require.NotNil(t, table.Mview)
	assert.Equal(t, "DEMAND", table.Mview.RefreshMode)
	assert.Equal(t, "COMPLETE", table.Mview.RefreshMethod)
	assert.Equal(t, "FRESH", table.Mview.Staleness)
	assert.Equal(t, refreshedAt.Format(time.RFC3339), table.Mview.LastRefreshDate)

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
	collectViews := false
	c.config.Schemas.CollectViews = &collectViews

	dbMock.ExpectQuery(`v\$containers`).WillReturnRows(
		sqlmock.NewRows([]string{"CON_ID", "NAME"}).AddRow(3, "APP_PDB"))
	dbMock.ExpectQuery("cdb_users").WillReturnRows(
		sqlmock.NewRows([]string{"CON_ID", "USERNAME", "USER_ID"}).AddRow(3, "APP", 104))
	dbMock.ExpectQuery("cdb_object_tables").WillReturnRows(sqlmock.NewRows(
		[]string{"CON_ID", "OWNER", "TABLE_NAME"}).AddRow(3, "APP", "ORDERS"))

	mainRows := sqlmock.NewRows([]string{
		"CON_ID", "OWNER", "TABLE_NAME", "TEMPORARY", "DURATION", "EXTERNAL", "IOT_TYPE",
		"PARTITIONED", "CLUSTER_NAME", "CLUSTERING", "READ_ONLY",
		"OBJECT_TYPE_OWNER", "OBJECT_TYPE",
		"COLUMN_NAME", "COLUMN_ID", "INTERNAL_COLUMN_ID", "VIRTUAL_COLUMN", "HIDDEN_COLUMN", "DATA_TYPE",
		"DATA_TYPE_OWNER", "DATA_TYPE_MOD", "DATA_LENGTH", "CHAR_LENGTH", "DATA_PRECISION",
		"DATA_SCALE", "CHAR_USED", "NULLABLE", "DATA_DEFAULT_VC",
	}).AddRow(
		3, "APP", "ORDERS", "N", "-", "NO", "-", "NO", "-", "NO", "NO", "-", "-",
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

	dbMock.ExpectQuery(`v\$containers`).WillReturnRows(
		sqlmock.NewRows([]string{"CON_ID", "NAME"}).AddRow(3, "APP_PDB"))
	dbMock.ExpectQuery("cdb_users").WillReturnRows(
		sqlmock.NewRows([]string{"CON_ID", "USERNAME", "USER_ID"}).AddRow(3, "APP", 104))
	dbMock.ExpectQuery("SELECT con_id, owner, table_name").WillReturnRows(identityRows(
		tableKey{conID: 3, owner: "APP", table: "ORDERS"}))
	dbMock.ExpectQuery("cdb_tab_cols").WillReturnRows(addTableRow(emptyTablesRows(), 3, "APP", "ORDERS", 1))
	dbMock.ExpectQuery("SELECT con_id, owner, view_name").WillReturnRows(identityRows(
		tableKey{conID: 3, owner: "APP", table: "V_ORDERS"}))
	dbMock.ExpectQuery(`(?s)WITH ranked_columns.*FROM cdb_views v`).WillReturnRows(addViewRow(sqlmock.NewRows(viewRelationColumns), 3, "APP", "V_ORDERS", 1))

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
	assert.Equal(t, tables.CollectionStartedAt, views.CollectionStartedAt,
		"tables and views belong to the same snapshot")
	assert.Zero(t, tables.CollectionPayloadsCount)
	assert.Equal(t, 2, views.CollectionPayloadsCount)
}

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

	dbMock.ExpectQuery(`v\$containers`).WillReturnRows(
		sqlmock.NewRows([]string{"CON_ID", "NAME"}).AddRow(3, "APP_PDB"))
	dbMock.ExpectQuery("cdb_users").WillReturnRows(
		sqlmock.NewRows([]string{"CON_ID", "USERNAME", "USER_ID"}).AddRow(3, "APP", 104))
	dbMock.ExpectQuery("SELECT con_id, owner, table_name").WillReturnRows(identityRows(
		tableKey{conID: 3, owner: "APP", table: "ORDERS"}))
	dbMock.ExpectQuery("cdb_tab_cols").WillReturnRows(addTableRow(emptyTablesRows(), 3, "APP", "ORDERS", 1))
	// Leave the view query unprimed to simulate a missing grant.

	require.Error(t, c.SchemaCollection())

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

	var event schemaEvent
	for _, call := range sender.Calls {
		if call.Method == "EventPlatformEvent" {
			require.NoError(t, json.Unmarshal(call.Arguments.Get(0).([]byte), &event))
		}
	}
	assert.Zero(t, event.CollectionPayloadsCount,
		"a table payload must not complete a snapshot when view collection fails")
	sender.AssertNumberOfCalls(t, "Commit", 1)
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

func TestIndexInfoJSONContract(t *testing.T) {
	index := indexInfo{
		Name:   "ORDERS_COMPOSITE_IDX",
		Unique: true,
		Type:   "NORMAL",
		Columns: []indexKeyPart{
			{Column: "STATUS"},
			{Expression: `UPPER("STATUS")`},
			{Column: "CREATED_AT"},
		},
	}

	payload, err := json.Marshal(index)
	require.NoError(t, err)
	require.JSONEq(t, `{
		"name": "ORDERS_COMPOSITE_IDX",
		"is_unique": true,
		"index_type": "NORMAL",
		"columns": [
			{"name": "STATUS"},
			{"expression": "UPPER(\"STATUS\")"},
			{"name": "CREATED_AT"}
		]
	}`, string(payload))

	var serialized map[string]any
	require.NoError(t, json.Unmarshal(payload, &serialized))
	assert.NotContains(t, serialized, "unique")

	columns, ok := serialized["columns"].([]any)
	require.True(t, ok)
	require.Len(t, columns, 3)
	plainPart, ok := columns[0].(map[string]any)
	require.True(t, ok)
	assert.NotContains(t, plainPart, "column")
	assert.NotContains(t, plainPart, "expression")
	expressionPart, ok := columns[1].(map[string]any)
	require.True(t, ok)
	assert.NotContains(t, expressionPart, "name")
	assert.NotContains(t, expressionPart, "column")
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

func TestTableDetailsConstraintKeysPreserveQuotedIdentifiers(t *testing.T) {
	c, _, dbMock, closeDB := newSchemaCheck(t)
	defer closeDB()

	dbMock.MatchExpectationsInOrder(false)
	dbMock.ExpectQuery("cdb_constraints").WillReturnRows(
		sqlmock.NewRows([]string{
			"CON_ID", "OWNER", "TABLE_NAME", "CONSTRAINT_NAME", "CONSTRAINT_TYPE",
			"R_OWNER", "R_CONSTRAINT_NAME", "COLUMN_NAME", "SEARCH_CONDITION",
		}).
			AddRow(3, "A|B", "RIGHT_TARGET", "C", "P", "-", "-", "RIGHT_ID", nil).
			AddRow(3, "A", "WRONG_TARGET", "B|C", "P", "-", "-", "WRONG_ID", nil).
			AddRow(3, "SOURCE", "CHILD", "CHILD_FK", "R", "A|B", "C", "TARGET_ID", nil))

	allowed := map[tableKey]struct{}{
		{conID: 3, owner: "A|B", table: "RIGHT_TARGET"}: {},
		{conID: 3, owner: "A", table: "WRONG_TARGET"}:   {},
		{conID: 3, owner: "SOURCE", table: "CHILD"}:     {},
	}
	details := c.tableDetails(context.Background(), allowed, nil)

	fk := details[tableKey{conID: 3, owner: "SOURCE", table: "CHILD"}].Constraints[0]
	assert.Equal(t, "RIGHT_TARGET", fk.ReferencedTable)
	assert.Equal(t, []string{"RIGHT_ID"}, fk.ReferencedColumns)
}

func TestTableDetailsResolvesForeignKeyToSelectedTableOutsidePage(t *testing.T) {
	c, _, dbMock, closeDB := newSchemaCheck(t)
	defer closeDB()

	dbMock.MatchExpectationsInOrder(false)
	dbMock.ExpectQuery(`(?s)cdb_constraints c.*c\.table_name IN \('ORDER_ITEMS'\)`).WillReturnRows(
		sqlmock.NewRows([]string{
			"CON_ID", "OWNER", "TABLE_NAME", "CONSTRAINT_NAME", "CONSTRAINT_TYPE",
			"R_OWNER", "R_CONSTRAINT_NAME", "COLUMN_NAME", "SEARCH_CONDITION",
		}).
			AddRow(3, "APP", "ORDER_ITEMS", "ITEMS_FK", "R", "APP", "ORDERS_PK", "ORDER_ID", nil).
			AddRow(3, "APP", "ORDER_ITEMS", "ITEMS_FK", "R", "APP", "ORDERS_PK", "LINE_NO", nil))
	dbMock.ExpectQuery(`(?s)cdb_constraints c.*c\.constraint_name = 'ORDERS_PK'`).WillReturnRows(
		sqlmock.NewRows([]string{"CON_ID", "OWNER", "CONSTRAINT_NAME", "TABLE_NAME", "COLUMN_NAME"}).
			AddRow(3, "APP", "ORDERS_PK", "ORDERS", "ORDER_ID").
			AddRow(3, "APP", "ORDERS_PK", "ORDERS", "LINE_NO"))

	page := map[tableKey]struct{}{{conID: 3, owner: "APP", table: "ORDER_ITEMS"}: {}}
	selected := map[tableKey]struct{}{
		{conID: 3, owner: "APP", table: "ORDER_ITEMS"}: {},
		{conID: 3, owner: "APP", table: "ORDERS"}:      {},
	}
	details := c.tableDetailsForPage(context.Background(), page, nil, selected)

	fk := details[tableKey{conID: 3, owner: "APP", table: "ORDER_ITEMS"}].Constraints[0]
	assert.Equal(t, "ORDERS", fk.ReferencedTable)
	assert.Equal(t, []string{"ORDER_ID", "LINE_NO"}, fk.ReferencedColumns)
	assert.Empty(t, fk.ReferencedConstraint)
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
	c.schemaPayloadChunkSize = 2

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
	dbMock.ExpectQuery("cdb_object_tables").WillReturnRows(sqlmock.NewRows(
		[]string{"CON_ID", "OWNER", "TABLE_NAME"}).AddRow(3, "APP", "ORDERS"))

	// A NULL TABLE_NAME forces StructScan to fail.
	mainRows := sqlmock.NewRows([]string{
		"CON_ID", "OWNER", "TABLE_NAME", "TEMPORARY", "DURATION", "EXTERNAL", "IOT_TYPE",
		"PARTITIONED", "CLUSTER_NAME", "CLUSTERING", "READ_ONLY",
		"OBJECT_TYPE_OWNER", "OBJECT_TYPE",
		"COLUMN_NAME", "COLUMN_ID", "INTERNAL_COLUMN_ID", "VIRTUAL_COLUMN", "HIDDEN_COLUMN", "DATA_TYPE",
		"DATA_TYPE_OWNER", "DATA_TYPE_MOD", "DATA_LENGTH", "CHAR_LENGTH", "DATA_PRECISION",
		"DATA_SCALE", "CHAR_USED", "NULLABLE", "DATA_DEFAULT_VC",
	}).AddRow(
		3, "APP", nil, "N", "-", "NO", "-", "NO", "-", "NO", "NO", "-", "-",
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

func TestTableIdentitiesKeepsMaxPlusOnePerContainer(t *testing.T) {
	c, _, dbMock, closeDB := newSchemaCheck(t)
	defer closeDB()

	dbMock.ExpectQuery("cdb_object_tables").WillReturnRows(sqlmock.NewRows(
		[]string{"CON_ID", "OWNER", "TABLE_NAME"}).
		AddRow(3, "APP", "A").
		AddRow(3, "APP", "B").
		AddRow(3, "APP", "C"))
	dbMock.ExpectQuery("cdb_object_tables").WillReturnRows(sqlmock.NewRows(
		[]string{"CON_ID", "OWNER", "TABLE_NAME"}).
		AddRow(4, "APP", "A").
		AddRow(4, "APP", "B"))

	owners := map[ownerKey]string{
		{conID: 3, owner: "APP"}: "1",
		{conID: 4, owner: "APP"}: "1",
	}
	keys, truncated, err := c.tableIdentities(context.Background(), []string{"'APP'"}, owners, "", 2)
	require.NoError(t, err)
	require.Len(t, keys, 4)
	assert.Equal(t, []tableKey{
		{conID: 3, owner: "APP", table: "A"},
		{conID: 3, owner: "APP", table: "B"},
		{conID: 4, owner: "APP", table: "A"},
		{conID: 4, owner: "APP", table: "B"},
	}, keys)
	assert.Contains(t, truncated, int64(3))
	assert.NotContains(t, truncated, int64(4))
}

func TestTableIdentityQueryUsesOracle12CompatibleLimit(t *testing.T) {
	assert.Contains(t, tableIdentitiesQueryTemplate, "ROWNUM <= /*IDENTITY_LIMIT*/")
	assert.NotContains(t, tableIdentitiesQueryTemplate, "FETCH FIRST")
	assert.NotContains(t, tableIdentitiesQueryTemplate, "OFFSET")
}

func TestObjectTablesAreExcludedFromOrdinaryTableBranches(t *testing.T) {
	assert.Contains(t, tableIdentitiesQueryTemplate, "NOT EXISTS (\n\t\t\t\tSELECT 1 FROM cdb_object_tables")
	assert.Contains(t, schemasQueryTemplate, "NOT EXISTS (\n\t\tSELECT 1 FROM cdb_object_tables")
}

func TestOwnerListForKeysIsDistinctAndSorted(t *testing.T) {
	keys := []tableKey{
		{conID: 3, owner: "REPORTING", table: "R"},
		{conID: 3, owner: "APP", table: "B"},
		{conID: 3, owner: "APP", table: "A"},
	}
	assert.Equal(t, "'APP', 'REPORTING'", ownerListForKeys(keys))
}

func TestForEachTablePageBoundsPageSize(t *testing.T) {
	keys := make([]tableKey, schemaRelationPageSize*2+1)
	var pageSizes []int
	err := forEachTablePage(keys, func(page []tableKey) error {
		pageSizes = append(pageSizes, len(page))
		return nil
	})
	require.NoError(t, err)
	assert.Equal(t, []int{schemaRelationPageSize, schemaRelationPageSize, 1}, pageSizes)
}

func TestForEachTablePageDoesNotMixContainers(t *testing.T) {
	keys := make([]tableKey, schemaRelationPageSize+1)
	for i := 0; i < schemaRelationPageSize-1; i++ {
		keys[i].conID = 3
	}
	keys[schemaRelationPageSize-1].conID = 5
	keys[schemaRelationPageSize].conID = 5

	var pages [][]tableKey
	require.NoError(t, forEachTablePage(keys, func(page []tableKey) error {
		pages = append(pages, page)
		return nil
	}))

	require.Len(t, pages, 2)
	assert.Len(t, pages[0], schemaRelationPageSize-1)
	assert.Len(t, pages[1], 2)
	assert.Equal(t, int64(3), pages[0][0].conID)
	assert.Equal(t, int64(5), pages[1][0].conID)
}

func TestSchemaCollectorKeepsSnapshotStateAcrossPages(t *testing.T) {
	c, _, _, closeDB := newSchemaCheck(t)
	defer closeDB()
	c.schemaPayloadChunkSize = 40

	var payloads []schemaEvent
	coordinator := newSchemaSnapshotCoordinator(func(payload []byte) {
		var event schemaEvent
		require.NoError(t, json.Unmarshal(payload, &event))
		payloads = append(payloads, event)
	})
	keys := make([]tableKey, schemaRelationPageSize+1)
	for i := range keys {
		keys[i] = tableKey{conID: 3, owner: "APP", table: fmt.Sprintf("T%03d", i)}
	}
	require.NoError(t, forEachTablePage(keys, func(page []tableKey) error {
		collector := newSchemaEventCollector(&c, coordinator.add, nil, map[ownerKey]string{{conID: 3, owner: "APP"}: "1"}, map[int64]string{3: "PDB"})
		for _, key := range page {
			collector.add(schemaRowDB{ConID: key.conID, Owner: key.owner, TableName: key.table})
		}
		collector.finish()
		return nil
	}))
	require.NoError(t, coordinator.complete())

	require.Len(t, payloads, 4)
	assert.Zero(t, payloads[0].CollectionPayloadsCount)
	assert.Zero(t, payloads[1].CollectionPayloadsCount)
	assert.Zero(t, payloads[2].CollectionPayloadsCount)
	assert.Equal(t, 4, payloads[3].CollectionPayloadsCount)
	assert.Equal(t, payloads[0].CollectionStartedAt, payloads[3].CollectionStartedAt)
	for _, payload := range payloads {
		assert.LessOrEqual(t, len(payload.Metadata[0].Schemas[0].Tables), 40)
	}
}

func TestHydrateTablePageFailsWhenSelectedTableDisappears(t *testing.T) {
	c, _, dbMock, closeDB := newSchemaCheck(t)
	defer closeDB()

	dbMock.ExpectQuery("cdb_tab_cols").WillReturnRows(emptyTablesRows())
	err := c.hydrateTablePage(context.Background(), []tableKey{{conID: 3, owner: "APP", table: "ORDERS"}}, 50, func(schemaRowDB) {})
	require.ErrorContains(t, err, "selected table 3.APP.ORDERS disappeared before hydration")
}

func TestSchemaCollectorKeepsTableWithNoEligibleColumns(t *testing.T) {
	c, _, dbMock, closeDB := newSchemaCheck(t)
	defer closeDB()

	var events []schemaEvent
	collector := newSchemaCollector(&c, func(payload []byte) {
		var event schemaEvent
		require.NoError(t, json.Unmarshal(payload, &event))
		events = append(events, event)
	}, nil, map[ownerKey]string{{conID: 3, owner: "APP"}: "1"}, map[int64]string{3: "PDB"})
	dbMock.ExpectQuery("cdb_tab_cols").WillReturnRows(addTableWithoutColumns(emptyTablesRows(), 3, "APP", "EMPTY"))
	require.NoError(t, c.hydrateTablePage(context.Background(), []tableKey{{conID: 3, owner: "APP", table: "EMPTY"}}, 50, collector.add))
	collector.finish()

	require.Len(t, events, 1)
	table := events[0].Metadata[0].Schemas[0].Tables[0]
	assert.Equal(t, "EMPTY", table.Name)
	assert.Empty(t, table.Columns)
}

func TestSchemaCollectionEmitsCompletedPageBeforeNextPageFails(t *testing.T) {
	db, dbMock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()
	dbMock.MatchExpectationsInOrder(false)

	c, sender := newDbDoesNotExistCheck(t, "", "")
	c.db = sqlx.NewDb(db, "sqlmock")
	c.clock = clock.NewMock()
	c.dbVersion = "23.0.0.0.0"
	c.config.Schemas.Enabled = true
	c.config.Schemas.MaxTables = schemaRelationPageSize + 1
	c.config.Schemas.MaxColumns = 50
	c.schemaPayloadChunkSize = 50
	collectViews := false
	c.config.Schemas.CollectViews = &collectViews

	dbMock.ExpectQuery(`v\$containers`).WillReturnRows(
		sqlmock.NewRows([]string{"CON_ID", "NAME"}).AddRow(3, "APP_PDB"))
	dbMock.ExpectQuery("cdb_users").WillReturnRows(
		sqlmock.NewRows([]string{"CON_ID", "USERNAME", "USER_ID"}).AddRow(3, "APP", 1))
	identityRows := sqlmock.NewRows([]string{"CON_ID", "OWNER", "TABLE_NAME"})
	firstPageRows := emptyTablesRows()
	for i := 0; i <= schemaRelationPageSize; i++ {
		name := fmt.Sprintf("T%03d", i)
		identityRows.AddRow(3, "APP", name)
		if i < schemaRelationPageSize {
			addTableRow(firstPageRows, 3, "APP", name, 0)
		}
	}
	dbMock.ExpectQuery("cdb_object_tables").WillReturnRows(identityRows)
	dbMock.ExpectQuery("WITH ranked_columns").WillReturnRows(firstPageRows)
	dbMock.ExpectQuery("WITH ranked_columns").WillReturnError(errors.New("second page failed"))

	require.ErrorContains(t, c.SchemaCollection(), "second page failed")
	sender.AssertNumberOfCalls(t, "EventPlatformEvent", 2)
	sender.AssertNumberOfCalls(t, "Commit", 1)
	for _, call := range sender.Calls[:2] {
		var event schemaEvent
		require.NoError(t, json.Unmarshal(call.Arguments.Get(0).([]byte), &event))
		assert.Len(t, event.Metadata[0].Schemas[0].Tables, 50)
		assert.Zero(t, event.CollectionPayloadsCount)
	}
}

func TestEmptyContainerStillEmitsTerminatingPayload(t *testing.T) {
	c, _, _, closeDB := newSchemaCheck(t)
	defer closeDB()

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
			c.config.QuerySamples.Enabled = false // Prevent unrelated session sampling when DBM is enabled.

			// Mark unrelated collectors as recently run to isolate schema collection.
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
				require.NoError(t, err, "the gate must stay closed and touch the database not at all")
			}
		})
	}
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

	dbMock.ExpectQuery(regexp.QuoteMeta(expectedFilter)).WillReturnRows(sqlmock.NewRows(
		[]string{"CON_ID", "OWNER", "TABLE_NAME"}))
	dbMock.ExpectQuery("SELECT con_id, owner, view_name").WillReturnRows(identityRows())

	require.NoError(t, c.SchemaCollection())
	assert.NoError(t, dbMock.ExpectationsWereMet(), "the query actually sent to Oracle must carry the substituted REGEXP_LIKE filter")
}

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
	dbMock.ExpectQuery(regexp.QuoteMeta(tableFilter)).WillReturnRows(identityRows())

	viewFilter := regexSQLClauses("v.view_name", c.config.Schemas.IncludeTables, c.config.Schemas.ExcludeTables)
	require.Contains(t, viewFilter, "REGEXP_LIKE")
	dbMock.ExpectQuery(regexp.QuoteMeta(viewFilter)).WillReturnRows(identityRows())

	require.NoError(t, c.SchemaCollection())
	assert.NoError(t, dbMock.ExpectationsWereMet(), "the views query actually sent to Oracle must carry the substituted REGEXP_LIKE filter")
}

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
	c.config.Schemas.MaxViews = 1
	c.config.Schemas.ExcludeTables = []string{"^TMP_"}

	dbMock.ExpectQuery(`v\$containers`).WillReturnRows(
		sqlmock.NewRows([]string{"CON_ID", "NAME"}).AddRow(3, "APP_PDB"))
	dbMock.ExpectQuery("cdb_users").WillReturnRows(
		sqlmock.NewRows([]string{"CON_ID", "USERNAME", "USER_ID"}).AddRow(3, "APP", 104))
	dbMock.ExpectQuery("SELECT con_id, owner, table_name").WillReturnRows(identityRows())
	dbMock.ExpectQuery("SELECT con_id, owner, view_name").WillReturnRows(identityRows(
		tableKey{conID: 3, owner: "APP", table: "V_ORDERS"}))
	dbMock.ExpectQuery(`(?s)WITH ranked_columns.*FROM cdb_views v`).WillReturnRows(
		addViewRow(sqlmock.NewRows(viewRelationColumns), 3, "APP", "V_ORDERS", 1))

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

var viewRelationColumns = []string{
	"CON_ID", "OWNER", "TABLE_NAME", "TEMPORARY", "DURATION", "EXTERNAL", "IOT_TYPE",
	"PARTITIONED", "CLUSTER_NAME", "CLUSTERING", "READ_ONLY",
	"OBJECT_TYPE_OWNER", "OBJECT_TYPE", "TOTAL_TABLES", "COLUMN_PRESENT",
	"COLUMN_NAME", "COLUMN_ID", "INTERNAL_COLUMN_ID", "VIRTUAL_COLUMN", "HIDDEN_COLUMN", "DATA_TYPE",
	"DATA_TYPE_OWNER", "DATA_TYPE_MOD", "DATA_LENGTH", "CHAR_LENGTH", "DATA_PRECISION",
	"DATA_SCALE", "CHAR_USED", "NULLABLE", "DATA_DEFAULT_VC", "TOTAL_COLUMNS",
}

func addViewRow(rows *sqlmock.Rows, conID int64, owner, viewName string, totalColumns int) *sqlmock.Rows {
	return rows.AddRow(
		conID, owner, viewName, "N", "-", "NO", "-", "NO", "-", "NO", "NO",
		"-", "-", nil, 1, "C1", 1, 1, "NO", "NO", "NUMBER", nil, nil, 22, nil, 12, 0, "-", "N", nil, totalColumns,
	)
}

func addViewWithoutColumns(rows *sqlmock.Rows, conID int64, owner, viewName string) *sqlmock.Rows {
	return rows.AddRow(
		conID, owner, viewName, "N", "-", "NO", "-", "NO", "-", "NO", "NO",
		"-", "-", nil, 0, "-", nil, nil, "-", "-", nil, nil, nil, nil, nil, nil, nil, "-", "-", nil, nil,
	)
}

func identityRows(keys ...tableKey) *sqlmock.Rows {
	rows := sqlmock.NewRows([]string{"CON_ID", "OWNER", "TABLE_NAME"})
	for _, key := range keys {
		rows.AddRow(key.conID, key.owner, key.table)
	}
	return rows
}

func TestViewPagesAreBounded(t *testing.T) {
	keys := make([]tableKey, schemaRelationPageSize+1)
	for i := range keys {
		keys[i] = tableKey{conID: 3, owner: "APP", table: fmt.Sprintf("V%03d", i)}
	}
	var sizes []int
	require.NoError(t, forEachTablePage(keys, func(page []tableKey) error {
		sizes = append(sizes, len(page))
		return nil
	}))
	assert.Equal(t, []int{schemaRelationPageSize, 1}, sizes)
}

func TestViewPageRowsKeepsZeroColumnView(t *testing.T) {
	c, _, dbMock, closeDB := newSchemaCheck(t)
	defer closeDB()
	key := tableKey{conID: 3, owner: "APP", table: "EMPTY_VIEW"}
	dbMock.ExpectQuery(`(?s)WITH ranked_columns.*FROM cdb_views v`).WillReturnRows(
		addViewWithoutColumns(sqlmock.NewRows(viewRelationColumns), key.conID, key.owner, key.table))
	rows, err := c.viewPageRows(context.Background(), []tableKey{key}, 50)
	require.NoError(t, err)
	require.Len(t, rows, 1)
	assert.False(t, rows[0].ColumnPresent.Valid && rows[0].ColumnPresent.Int64 == 1)
}

func TestViewPageRowsFailsWhenIdentityDisappears(t *testing.T) {
	c, _, dbMock, closeDB := newSchemaCheck(t)
	defer closeDB()
	key := tableKey{conID: 3, owner: "APP", table: "DROPPED_VIEW"}
	dbMock.ExpectQuery(`(?s)WITH ranked_columns.*FROM cdb_views v`).WillReturnRows(sqlmock.NewRows(viewRelationColumns))
	_, err := c.viewPageRows(context.Background(), []tableKey{key}, 50)
	require.ErrorContains(t, err, "disappeared before hydration")
}

func TestViewDetailsAreScopedToPage(t *testing.T) {
	c, _, dbMock, closeDB := newSchemaCheck(t)
	defer closeDB()
	dbMock.MatchExpectationsInOrder(false)
	key := tableKey{conID: 3, owner: "APP", table: "V1"}
	dbMock.ExpectQuery("cdb_views").WillReturnRows(
		sqlmock.NewRows([]string{"CON_ID", "OWNER", "VIEW_NAME", "TEXT_VC"}).
			AddRow(3, "APP", "V1", "SELECT 1 FROM dual").
			AddRow(3, "APP", "V2", "SELECT 2 FROM dual"))
	dbMock.ExpectQuery("cdb_objects").WillReturnRows(
		sqlmock.NewRows([]string{"CON_ID", "OWNER", "OBJECT_NAME", "OBJECT_ID", "CREATED", "LAST_DDL_TIME"}))
	dbMock.ExpectQuery("cdb_tab_comments").WillReturnRows(
		sqlmock.NewRows([]string{"CON_ID", "OWNER", "TABLE_NAME", "COMMENTS"}))

	details := c.viewDetailsForPage(context.Background(), map[tableKey]struct{}{key: {}})
	require.Len(t, details, 1)
	assert.Equal(t, "SELECT 1 FROM dual", details[key].Definition)
	assert.NotContains(t, details, tableKey{conID: 3, owner: "APP", table: "V2"})
}

func TestSchemaCollectionUsesOwnerContainerWhenContainerLookupIsEmpty(t *testing.T) {
	db, dbMock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()
	c, sender := newDbDoesNotExistCheck(t, "", "")
	c.db = sqlx.NewDb(db, "sqlmock")
	c.clock = clock.NewMock()
	c.dbVersion = "23.0.0.0.0"
	c.config.Schemas.Enabled = true
	dbMock.ExpectQuery(`v\$containers`).WillReturnRows(sqlmock.NewRows([]string{"CON_ID", "NAME"}))
	dbMock.ExpectQuery("cdb_users").WillReturnRows(
		sqlmock.NewRows([]string{"CON_ID", "USERNAME", "USER_ID"}).AddRow(3, "APP", 104))
	dbMock.ExpectQuery("SELECT con_id, owner, table_name").WillReturnRows(identityRows())
	dbMock.ExpectQuery("SELECT con_id, owner, view_name").WillReturnRows(identityRows())

	require.NoError(t, c.SchemaCollection())
	var events []schemaEvent
	for _, call := range sender.Calls {
		if call.Method != "EventPlatformEvent" {
			continue
		}
		var event schemaEvent
		require.NoError(t, json.Unmarshal(call.Arguments.Get(0).([]byte), &event))
		events = append(events, event)
	}
	require.Len(t, events, 1)
	assert.Equal(t, "3", events[0].Metadata[0].ID)
	assert.Equal(t, 1, events[0].CollectionPayloadsCount)
}

func TestSchemaCollectionContinuesAfterContainerIdentityFailure(t *testing.T) {
	db, dbMock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()
	c, sender := newDbDoesNotExistCheck(t, "", "")
	c.db = sqlx.NewDb(db, "sqlmock")
	c.clock = clock.NewMock()
	c.dbVersion = "23.0.0.0.0"
	c.config.Schemas.Enabled = true
	dbMock.MatchExpectationsInOrder(false)
	dbMock.ExpectQuery(`v\$containers`).WillReturnRows(
		sqlmock.NewRows([]string{"CON_ID", "NAME"}).AddRow(3, "PDB1").AddRow(4, "PDB2"))
	dbMock.ExpectQuery("cdb_users").WillReturnRows(
		sqlmock.NewRows([]string{"CON_ID", "USERNAME", "USER_ID"}).AddRow(3, "APP", 103).AddRow(4, "APP", 104))
	dbMock.ExpectQuery("SELECT con_id, owner, table_name.*t.con_id = 3").WillReturnError(errors.New("identity failed"))
	dbMock.ExpectQuery("SELECT con_id, owner, table_name.*t.con_id = 4").WillReturnRows(identityRows())
	dbMock.ExpectQuery("SELECT con_id, owner, view_name.*v.con_id = 4").WillReturnRows(identityRows())

	require.ErrorContains(t, c.SchemaCollection(), "container 3 table identities")
	var completed []string
	for _, call := range sender.Calls {
		if call.Method != "EventPlatformEvent" {
			continue
		}
		var event schemaEvent
		require.NoError(t, json.Unmarshal(call.Arguments.Get(0).([]byte), &event))
		if event.CollectionPayloadsCount > 0 {
			completed = append(completed, event.Metadata[0].ID)
		}
	}
	assert.Equal(t, []string{"4"}, completed)
}

func addTableRow(rows *sqlmock.Rows, conID int64, owner, table string, totalTables int) *sqlmock.Rows {
	return rows.AddRow(
		conID, owner, table, "N", "-", "NO", "-", "NO", "-", "NO", "NO", "-", "-", totalTables,
		1, "C1", 1, 1, "NO", "NO", "NUMBER", nil, nil, 22, nil, 12, 0, "-", "Y", nil,
	)
}

func addTableWithoutColumns(rows *sqlmock.Rows, conID int64, owner, table string) *sqlmock.Rows {
	return rows.AddRow(
		conID, owner, table, "N", "-", "NO", "-", "NO", "-", "NO", "NO", "-", "-", nil,
		0, "-", nil, nil, "-", "-", nil, nil, nil, nil, nil, nil, nil, "-", "-", nil,
	)
}

func emptyTablesRows() *sqlmock.Rows {
	return sqlmock.NewRows([]string{
		"CON_ID", "OWNER", "TABLE_NAME", "TEMPORARY", "DURATION", "EXTERNAL", "IOT_TYPE",
		"PARTITIONED", "CLUSTER_NAME", "CLUSTERING", "READ_ONLY",
		"OBJECT_TYPE_OWNER", "OBJECT_TYPE", "TOTAL_TABLES",
		"COLUMN_PRESENT", "COLUMN_NAME", "COLUMN_ID", "INTERNAL_COLUMN_ID", "VIRTUAL_COLUMN", "HIDDEN_COLUMN", "DATA_TYPE",
		"DATA_TYPE_OWNER", "DATA_TYPE_MOD", "DATA_LENGTH", "CHAR_LENGTH", "DATA_PRECISION",
		"DATA_SCALE", "CHAR_USED", "NULLABLE", "DATA_DEFAULT_VC",
	})
}

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
	c.config.Schemas.MaxViews = 2

	dbMock.ExpectQuery(`v\$containers`).WillReturnRows(
		sqlmock.NewRows([]string{"CON_ID", "NAME"}).AddRow(3, "APP_PDB"))
	dbMock.ExpectQuery("cdb_users").WillReturnRows(
		sqlmock.NewRows([]string{"CON_ID", "USERNAME", "USER_ID"}).AddRow(3, "APP", 104))
	dbMock.ExpectQuery("SELECT con_id, owner, table_name").WillReturnRows(identityRows())

	dbMock.ExpectQuery("SELECT con_id, owner, view_name").WillReturnRows(identityRows(
		tableKey{conID: 3, owner: "APP", table: "V1"}, tableKey{conID: 3, owner: "APP", table: "V2"},
		tableKey{conID: 3, owner: "APP", table: "V3"}))
	rows := sqlmock.NewRows(viewRelationColumns)
	addViewRow(rows, 3, "APP", "V1", 1)
	addViewRow(rows, 3, "APP", "V2", 1)
	dbMock.ExpectQuery(`(?s)WITH ranked_columns.*FROM cdb_views v`).WillReturnRows(rows)

	require.NoError(t, c.SchemaCollection())

	byContainer := viewPayloadsByContainer(t, &sender.Mock)
	require.Contains(t, byContainer, "3")
	views := byContainer["3"]
	require.Len(t, views.Metadata[0].Schemas, 1)
	assert.Len(t, views.Metadata[0].Schemas[0].Views, 2, "max_views=2 must cap the container at 2 views")
	assert.True(t, views.Truncated, "a container with more views than max_views must be marked truncated")
}

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
	c.config.Schemas.MaxViews = 50

	dbMock.ExpectQuery(`v\$containers`).WillReturnRows(
		sqlmock.NewRows([]string{"CON_ID", "NAME"}).AddRow(3, "APP_PDB"))
	dbMock.ExpectQuery("cdb_users").WillReturnRows(
		sqlmock.NewRows([]string{"CON_ID", "USERNAME", "USER_ID"}).AddRow(3, "APP", 104))
	dbMock.ExpectQuery("SELECT con_id, owner, table_name").WillReturnRows(identityRows())

	dbMock.ExpectQuery("SELECT con_id, owner, view_name").WillReturnRows(identityRows(
		tableKey{conID: 3, owner: "APP", table: "V1"}))
	rows := sqlmock.NewRows(viewRelationColumns)
	addViewRow(rows, 3, "APP", "V1", 1)
	dbMock.ExpectQuery(`(?s)WITH ranked_columns.*FROM cdb_views v`).WillReturnRows(rows)

	require.NoError(t, c.SchemaCollection())

	byContainer := viewPayloadsByContainer(t, &sender.Mock)
	require.Contains(t, byContainer, "3")
	views := byContainer["3"]
	require.Len(t, views.Metadata[0].Schemas, 1)
	assert.Len(t, views.Metadata[0].Schemas[0].Views, 1)
	assert.False(t, views.Truncated, "a container with fewer views than max_views must not be marked truncated")
}

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
	c.config.Schemas.MaxViews = 2

	dbMock.ExpectQuery(`v\$containers`).WillReturnRows(
		sqlmock.NewRows([]string{"CON_ID", "NAME"}).AddRow(3, "APP_PDB"))
	dbMock.ExpectQuery("cdb_users").WillReturnRows(
		sqlmock.NewRows([]string{"CON_ID", "USERNAME", "USER_ID"}).AddRow(3, "APP", 104))
	dbMock.ExpectQuery("SELECT con_id, owner, table_name").WillReturnRows(identityRows())

	dbMock.ExpectQuery("SELECT con_id, owner, view_name").WillReturnRows(identityRows(
		tableKey{conID: 3, owner: "APP", table: "V1"}, tableKey{conID: 3, owner: "APP", table: "V2"}))
	rows := sqlmock.NewRows(viewRelationColumns)
	addViewRow(rows, 3, "APP", "V1", 1)
	addViewRow(rows, 3, "APP", "V2", 1)
	dbMock.ExpectQuery(`(?s)WITH ranked_columns.*FROM cdb_views v`).WillReturnRows(rows)

	require.NoError(t, c.SchemaCollection())

	byContainer := viewPayloadsByContainer(t, &sender.Mock)
	require.Contains(t, byContainer, "3")
	views := byContainer["3"]
	require.Len(t, views.Metadata[0].Schemas, 1)
	assert.Len(t, views.Metadata[0].Schemas[0].Views, 2)
	assert.False(t, views.Truncated, "a container with exactly max_views views must not be marked truncated")
}

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
	c.config.Schemas.MaxViews = 2

	dbMock.ExpectQuery(`v\$containers`).WillReturnRows(
		sqlmock.NewRows([]string{"CON_ID", "NAME"}).AddRow(3, "APP_PDB1").AddRow(4, "APP_PDB2"))
	dbMock.ExpectQuery("cdb_users").WillReturnRows(
		sqlmock.NewRows([]string{"CON_ID", "USERNAME", "USER_ID"}).
			AddRow(3, "APP", 104).AddRow(4, "APP", 105))
	dbMock.ExpectQuery("SELECT con_id, owner, table_name.*t.con_id = 3").WillReturnRows(identityRows())
	dbMock.ExpectQuery("SELECT con_id, owner, table_name.*t.con_id = 4").WillReturnRows(identityRows())

	dbMock.ExpectQuery("SELECT con_id, owner, view_name.*v.con_id = 3").WillReturnRows(identityRows(
		tableKey{conID: 3, owner: "APP", table: "V1"}, tableKey{conID: 3, owner: "APP", table: "V2"},
		tableKey{conID: 3, owner: "APP", table: "V3"}))
	rows3 := sqlmock.NewRows(viewRelationColumns)
	addViewRow(rows3, 3, "APP", "V1", 1)
	addViewRow(rows3, 3, "APP", "V2", 1)
	dbMock.ExpectQuery(`(?s)WITH ranked_columns.*FROM cdb_views v.*v\.con_id = 3`).WillReturnRows(rows3)
	dbMock.ExpectQuery("SELECT con_id, owner, view_name.*v.con_id = 4").WillReturnRows(identityRows(
		tableKey{conID: 4, owner: "APP", table: "V4"}, tableKey{conID: 4, owner: "APP", table: "V5"}))
	rows4 := sqlmock.NewRows(viewRelationColumns)
	addViewRow(rows4, 4, "APP", "V4", 1)
	addViewRow(rows4, 4, "APP", "V5", 1)
	dbMock.ExpectQuery(`(?s)WITH ranked_columns.*FROM cdb_views v.*v\.con_id = 4`).WillReturnRows(rows4)

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
