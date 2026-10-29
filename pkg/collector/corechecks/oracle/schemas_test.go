// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build oracle_test

package oracle

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/DataDog/datadog-agent/pkg/aggregator/mocksender"
	"github.com/benbjohnson/clock"
	"github.com/jmoiron/sqlx"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

type schemaTestCollector struct {
	*schemaCollector
	t           *testing.T
	coordinator *schemaSnapshotCoordinator
}

func newSchemaCollector(t *testing.T, c *Check, emit payloadEmitter, details map[tableKey]*tableDetails, owners map[ownerKey]string, containers map[int64]string) *schemaTestCollector {
	coordinator := newSchemaSnapshotCoordinator(c.nextSnapshotID, emit)
	return &schemaTestCollector{schemaCollector: newSchemaEventCollector(c, coordinator.add, details, owners, containers), t: t, coordinator: coordinator}
}

func (s *schemaTestCollector) finish() {
	s.t.Helper()
	s.schemaCollector.finish()
	require.NoError(s.t, s.coordinator.complete())
}

func (s *schemaTestCollector) emitEmptyContainers(containers map[int64]string) {
	s.t.Helper()
	s.schemaCollector.emitEmptyContainers(containers)
	require.NoError(s.t, s.coordinator.complete())
}

func (c *Check) tableDetails(ctx context.Context, allowed map[tableKey]struct{}, allowedColumns map[columnKey]struct{}) map[tableKey]*tableDetails {
	details := c.tableDetailsForPage(ctx, allowed, allowedColumns)
	for key, constraints := range c.tableConstraintsForPage(ctx, allowed, allowed) {
		if details[key] == nil {
			details[key] = &tableDetails{}
		}
		details[key].Constraints = constraints.Constraints
	}
	return details
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

func TestSchemaRelationsDoNotEmitObjectIDs(t *testing.T) {
	for name, relation := range map[string]any{
		"table": schemaTable{Name: "ORDERS", Owner: "APP"},
		"view":  viewObject{Name: "ORDERS", Owner: "APP"},
	} {
		t.Run(name, func(t *testing.T) {
			_, hasID := reflect.TypeOf(relation).FieldByName("ID")
			assert.False(t, hasID)
			payload, err := json.Marshal(relation)
			require.NoError(t, err)
			var fields map[string]any
			require.NoError(t, json.Unmarshal(payload, &fields))
			assert.NotContains(t, fields, "id")
			assert.Equal(t, "ORDERS", fields["name"])
			assert.Equal(t, "APP", fields["owner"])
		})
	}
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

func expectSchemaContainerAvailable(dbMock sqlmock.Sqlmock, conID int64) {
	dbMock.ExpectQuery(regexp.QuoteMeta(containerNamesQuery + " AND con_id = :con_id")).WithArgs(sql.Named("con_id", conID)).
		WillReturnRows(sqlmock.NewRows([]string{"CON_ID", "NAME"}).AddRow(conID, "PDB"))
}

func TestSchemaCollectionUnavailableContainerDoesNotComplete(t *testing.T) {
	for _, hasOwners := range []bool{false, true} {
		for _, queryFails := range []bool{false, true} {
			t.Run(fmt.Sprintf("owners_%t_query_error_%t", hasOwners, queryFails), func(t *testing.T) {
				c, _, dbMock, cleanup := newSchemaCheck(t)
				defer cleanup()
				views := false
				c.config.Schemas.CollectViews = &views
				dbMock.ExpectQuery(regexp.QuoteMeta(containerNamesQuery)).WillReturnRows(
					sqlmock.NewRows([]string{"CON_ID", "NAME"}).AddRow(3, "APP_PDB"))
				owners := sqlmock.NewRows([]string{"CON_ID", "USERNAME", "USER_ID"})
				if hasOwners {
					owners.AddRow(3, "APP", 104)
				}
				dbMock.ExpectQuery("cdb_users").WillReturnRows(owners)
				if hasOwners {
					dbMock.ExpectQuery("SELECT con_id, owner, table_name").WillReturnRows(
						sqlmock.NewRows([]string{"CON_ID", "OWNER", "TABLE_NAME"}))
				}
				query := dbMock.ExpectQuery(regexp.QuoteMeta(containerNamesQuery + " AND con_id = :con_id")).WithArgs(sql.Named("con_id", int64(3)))
				if queryFails {
					query.WillReturnError(errors.New("availability check failed"))
				} else {
					query.WillReturnRows(sqlmock.NewRows([]string{"CON_ID", "NAME"}))
				}
				err := c.SchemaCollection()
				if queryFails {
					require.ErrorContains(t, err, "availability check failed")
				} else {
					require.ErrorContains(t, err, "no longer available")
				}
				sender, err := c.GetRawSender()
				require.NoError(t, err)
				mockSender, ok := sender.(*mocksender.MockSender)
				require.True(t, ok)
				for _, call := range mockSender.Calls {
					if call.Method == "EventPlatformEvent" {
						var event schemaEvent
						require.NoError(t, json.Unmarshal(call.Arguments.Get(0).([]byte), &event))
						require.Zero(t, event.CollectionPayloadsCount)
					}
				}
				require.NoError(t, dbMock.ExpectationsWereMet())
			})
		}
	}
}

func TestSchemaCollectionEmptyContainerValidationFailureDoesNotBlockNext(t *testing.T) {
	c, _, dbMock, cleanup := newSchemaCheck(t)
	defer cleanup()
	dbMock.MatchExpectationsInOrder(false)
	dbMock.ExpectQuery(regexp.QuoteMeta(containerNamesQuery)).WillReturnRows(
		sqlmock.NewRows([]string{"CON_ID", "NAME"}).AddRow(3, "PDB1").AddRow(4, "PDB2"))
	dbMock.ExpectQuery("cdb_users").WillReturnRows(sqlmock.NewRows([]string{"CON_ID", "USERNAME", "USER_ID"}))
	dbMock.ExpectQuery(regexp.QuoteMeta(containerNamesQuery + " AND con_id = :con_id")).WithArgs(sql.Named("con_id", int64(3))).WillReturnError(context.DeadlineExceeded)
	expectSchemaContainerAvailable(dbMock, 4)
	require.ErrorIs(t, c.SchemaCollection(), context.DeadlineExceeded)
	sender, err := c.GetRawSender()
	require.NoError(t, err)
	mockSender, ok := sender.(*mocksender.MockSender)
	require.True(t, ok)
	var completed []string
	for _, call := range mockSender.Calls {
		if call.Method == "EventPlatformEvent" {
			var event schemaEvent
			require.NoError(t, json.Unmarshal(call.Arguments.Get(0).([]byte), &event))
			if event.CollectionPayloadsCount > 0 {
				completed = append(completed, event.Metadata[0].ID)
			}
		}
	}
	require.Equal(t, []string{"4"}, completed)
	require.NoError(t, dbMock.ExpectationsWereMet())
}

func TestSchemaSnapshotValidationFailureKeepsOtherContainers(t *testing.T) {
	c, _, _, cleanup := newSchemaCheck(t)
	defer cleanup()
	var events []schemaEvent
	coordinator := newSchemaSnapshotCoordinator(c.nextSnapshotID, func(payload []byte) {
		var event schemaEvent
		require.NoError(t, json.Unmarshal(payload, &event))
		events = append(events, event)
	})
	unavailable := errors.New("PDB closed")
	coordinator.validate = func(id string) error {
		if id == "3" {
			return unavailable
		}
		return nil
	}
	for _, id := range []string{"3", "4"} {
		coordinator.add(schemaEvent{CollectionStartedAt: 123, Metadata: []containerObject{{ID: id}}})
	}
	require.ErrorIs(t, coordinator.complete(), unavailable)
	require.Len(t, events, 2)
	require.Zero(t, events[0].CollectionPayloadsCount)
	require.Equal(t, 1, events[1].CollectionPayloadsCount)
	require.NoError(t, coordinator.complete())
	require.Len(t, events, 2)
}

func TestContainerNamesExcludesUnavailablePDBs(t *testing.T) {
	c, _, dbMock, cleanup := newSchemaCheck(t)
	defer cleanup()
	dbMock.ExpectQuery(`SELECT con_id, name FROM v\$containers WHERE open_mode IN \('READ WRITE', 'READ ONLY'\) AND NVL\(restricted, 'NO'\) = 'NO'`).
		WillReturnRows(sqlmock.NewRows([]string{"CON_ID", "NAME"}).AddRow(3, "OPEN_PDB"))
	names, err := c.containerNames(context.Background())
	require.NoError(t, err)
	require.Equal(t, map[int64]string{3: "OPEN_PDB"}, names)
	require.NoError(t, dbMock.ExpectationsWereMet())
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

	for _, v := range []string{"19.21.0.0.0", "21.3.0.0.0", "12.2.0.1.0", "12.1.0.2.0"} {
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

	filters := relationFilterChunks(allowed, columns)
	require.Len(t, filters, 1)
	require.Contains(t, filters[0].text, "x.con_id = :rel0con AND x.owner = :rel0owner AND x.table_name IN (:rel0name0,")
	require.Contains(t, filters[0].text, " OR (x.con_id = :rel1con")
	require.Equal(t, int64(3), filters[0].args[0].(sql.NamedArg).Value)
	require.Equal(t, "APP", filters[0].args[1].(sql.NamedArg).Value)
	require.Equal(t, sql.NullString{String: "O'RDER", Valid: true}, filters[0].args[2].(sql.NamedArg).Value)
	require.Equal(t, sql.NullString{String: "USERS", Valid: true}, filters[0].args[3].(sql.NamedArg).Value)
	require.Equal(t, int64(4), filters[0].args[102].(sql.NamedArg).Value)
	require.Equal(t, "REPORT", filters[0].args[103].(sql.NamedArg).Value)
	require.Equal(t, sql.NullString{String: "DAILY", Valid: true}, filters[0].args[104].(sql.NamedArg).Value)
	require.NotContains(t, filters[0].text, "O'RDER")
}

func TestColumnFilterChunksIncludesOnlySelectedColumns(t *testing.T) {
	table := tableKey{conID: 3, owner: "APP", table: "ORDERS"}
	allowed := map[columnKey]struct{}{
		{tableKey: table, column: "STATUS"}: {},
		{tableKey: table, column: "ID"}:     {},
	}
	columns := relationColumnNames{conID: "c.con_id", owner: "c.owner", relation: "c.table_name"}

	filters := columnFilterChunks(allowed, columns, "c.column_name")
	require.Len(t, filters, 1)
	require.Contains(t, filters[0].text, "c.table_name = :col0table AND c.column_name IN (:col0name0,")
	require.Len(t, filters[0].args, 3+schemaColumnBindSlots)
	require.Equal(t, sql.NullString{String: "ID", Valid: true}, filters[0].args[3].(sql.NamedArg).Value)
	require.Equal(t, sql.NullString{String: "STATUS", Valid: true}, filters[0].args[4].(sql.NamedArg).Value)
	for _, arg := range filters[0].args[5:] {
		require.Equal(t, sql.NullString{}, arg.(sql.NamedArg).Value)
	}
}

func TestRelationFilterChunksAtOracleLimit(t *testing.T) {
	allowed := make(map[tableKey]struct{}, maxSchemaRelationsPerQuery+1)
	for i := 0; i <= maxSchemaRelationsPerQuery; i++ {
		allowed[tableKey{conID: 3, owner: "APP", table: fmt.Sprintf("T%04d", i)}] = struct{}{}
	}

	filters := relationFilterChunks(allowed, relationColumnNames{conID: "con_id", owner: "owner", relation: "table_name"})
	require.Len(t, filters, 2)
	require.Len(t, filters[0].args, maxSchemaRelationsPerQuery+2)
	require.Equal(t, sql.NullString{String: "T0999", Valid: true}, filters[0].args[1001].(sql.NamedArg).Value)
	require.Equal(t, sql.NullString{String: "T1000", Valid: true}, filters[1].args[2].(sql.NamedArg).Value)
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
	dbMock.ExpectQuery("SELECT con_id, owner, table_name").WithArgs(schemaIdentityArgs(3, 2, namesForSchemaTest(1000))...).WillReturnRows(sqlmock.NewRows(
		[]string{"CON_ID", "OWNER", "TABLE_NAME"}).AddRow(3, "APP0000", "T1"))
	dbMock.ExpectQuery("SELECT con_id, owner, table_name").WithArgs(schemaIdentityArgs(3, 1, []string{"APP1000"})...).WillReturnRows(sqlmock.NewRows(
		[]string{"CON_ID", "OWNER", "TABLE_NAME"}).AddRow(3, "APP1000", "T2"))
	dbMock.ExpectQuery("cdb_tab_cols").WillReturnRows(addTableRow(emptyTablesRows(), 3, "APP0000", "T1"))

	expectSchemaContainerAvailable(dbMock, 3)
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
	collector := newSchemaCollector(t, &c, func(b []byte) {
		var e schemaEvent
		require.NoError(t, json.Unmarshal(b, &e))
		payloads = append(payloads, e)
	}, map[tableKey]*tableDetails{}, map[ownerKey]string{}, map[int64]string{})

	for _, name := range []string{"T1", "T2", "T3", "T4", "T5"} {
		for _, column := range []string{"C1", "C2", "C3"} {
			collector.add(schemaRowDB{
				ConID: 3, Owner: "APP", TableName: name, Temporary: "N", External: "NO",
				IotType: "-", ClusterName: "-", Partitioned: "NO",
				ColumnName: column, DataType: sql.NullString{String: "NUMBER", Valid: true},
				Nullable: "Y",
			})
		}
	}
	collector.finish()

	require.Len(t, payloads, 3, "5 tables at chunk size 2 must split into 3 payloads")

	var names []string
	for i, p := range payloads {
		assert.Equal(t, payloads[0].CollectionStartedAt, p.CollectionStartedAt,
			"payload %d must carry the snapshot id", i)
		require.Len(t, p.Metadata, 1)
		require.Len(t, p.Metadata[0].Schemas, 1)
		assert.Len(t, p.Metadata[0].Schemas[0].Tables, min(2, 5-i*2))
		for _, table := range p.Metadata[0].Schemas[0].Tables {
			names = append(names, table.Name)
			assert.Len(t, table.Columns, 3, "table %s must keep its columns in one payload", table.Name)
		}
	}
	assert.Equal(t, []string{"T1", "T2", "T3", "T4", "T5"}, names)
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
			coordinator := newSchemaSnapshotCoordinator(c.nextSnapshotID, func(payload []byte) {
				var event schemaEvent
				require.NoError(t, json.Unmarshal(payload, &event))
				events = append(events, event)
			})
			collector := newViewEventCollector(&c, coordinator.add, nil, nil, nil)

			for i := 0; i <= tc.want; i++ {
				collector.addView(schemaRowDB{ConID: 3, Owner: "APP", TableName: fmt.Sprintf("V%04d", i)})
			}
			collector.finish()
			require.NoError(t, coordinator.complete())

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
	collector := newSchemaCollector(t, &c, func(b []byte) {
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
	c, _, _, cleanup := newSchemaCheck(t)
	defer cleanup()
	events := []schemaEvent{
		{Kind: "oracle_databases", CollectionStartedAt: 100, CollectionPayloadsCount: 1, Metadata: []containerObject{{ID: "1"}}},
		{Kind: "oracle_databases", CollectionStartedAt: 200, CollectionPayloadsCount: 1, Metadata: []containerObject{{ID: "3"}}},
		{Kind: "oracle_views", CollectionStartedAt: 300, CollectionPayloadsCount: 1, Metadata: []containerObject{{ID: "1"}}},
		{Kind: "oracle_views", CollectionStartedAt: 400, CollectionPayloadsCount: 1, Metadata: []containerObject{{ID: "3"}}},
	}

	var emitted []schemaEvent
	coordinator := newSchemaSnapshotCoordinator(c.nextSnapshotID, func(payload []byte) {
		var event schemaEvent
		require.NoError(t, json.Unmarshal(payload, &event))
		emitted = append(emitted, event)
	})
	for _, event := range events {
		coordinator.add(event)
	}
	require.NoError(t, coordinator.complete())

	require.Len(t, emitted, 4)
	assert.Equal(t, int64(1), emitted[0].CollectionStartedAt)
	assert.Equal(t, int64(2), emitted[1].CollectionStartedAt)
	assert.Equal(t, emitted[0].CollectionStartedAt, emitted[2].CollectionStartedAt)
	assert.Equal(t, emitted[1].CollectionStartedAt, emitted[3].CollectionStartedAt)
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
	for _, view := range []string{"cdb_blockchain_tables", "cdb_immutable_tables"} {
		dbMock.ExpectQuery("FROM " + view).WillReturnRows(sqlmock.NewRows([]string{"UNUSED"}))
	}
	dbMock.ExpectQuery("FROM cdb_mviews WHERE").WillReturnRows(sqlmock.NewRows(
		[]string{"CON_ID", "OWNER", "MVIEW_NAME", "REFRESH_MODE", "REFRESH_METHOD", "STALENESS", "LAST_REFRESH_DATE"},
	).AddRow(3, "APP", "MV_ORDERS", "DEMAND", "COMPLETE", "FRESH", refreshedAt))
	for _, view := range []string{"cdb_tab_comments", "cdb_indexes", "cdb_external_tables", "cdb_part_tables", "cdb_constraints"} {
		dbMock.ExpectQuery("FROM " + view).WillReturnRows(sqlmock.NewRows([]string{"UNUSED"}))
	}
	details := c.tableDetails(context.Background(), map[tableKey]struct{}{key: {}}, nil)
	require.NoError(t, dbMock.ExpectationsWereMet())

	var payloads []schemaEvent
	collector := newSchemaCollector(t, &c, func(payload []byte) {
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
	collector := newSchemaCollector(t, &c, func(b []byte) {
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
	expectSchemaContainerAvailable(dbMock, 3)
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
	collector := newSchemaCollector(t, &c, func(b []byte) {
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
	dbMock.ExpectQuery("cdb_tab_cols").WillReturnRows(addTableRow(emptyTablesRows(), 3, "APP", "ORDERS"))
	dbMock.ExpectQuery("SELECT con_id, owner, view_name").WillReturnRows(identityRows(
		tableKey{conID: 3, owner: "APP", table: "V_ORDERS"}))
	dbMock.ExpectQuery(`(?s)WITH ranked_columns.*FROM cdb_views v`).WillReturnRows(addViewRow(sqlmock.NewRows(viewRelationColumns), 3, "APP", "V_ORDERS", 1))

	expectSchemaContainerAvailable(dbMock, 3)
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
	dbMock.ExpectQuery("cdb_tab_cols").WillReturnRows(addTableRow(emptyTablesRows(), 3, "APP", "ORDERS"))
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

func TestTableDetailsIndexesGroupByOwnerAndName(t *testing.T) {
	c, _, dbMock, cleanup := newSchemaCheck(t)
	defer cleanup()
	dbMock.MatchExpectationsInOrder(false)
	dbMock.ExpectQuery(`(?s)SELECT con_id, table_owner, table_name, index_owner,.*ic\.con_id = :rel0con AND ic\.table_owner = :rel0owner AND ic\.table_name IN \(:rel0name0,.*UNION ALL.*WHERE i.join_index = 'YES'.*ORDER BY con_id, table_owner, table_name, index_owner, index_name, column_position`).
		WillReturnRows(sqlmock.NewRows([]string{"CON_ID", "TABLE_OWNER", "TABLE_NAME", "INDEX_OWNER", "INDEX_NAME", "UNIQUENESS", "INDEX_TYPE", "COLUMN_NAME"}).
			AddRow(3, "APP", "T", "A", "IDX", "UNIQUE", "NORMAL", "C1").
			AddRow(3, "APP", "T", "A", "IDX", "UNIQUE", "NORMAL", "C2").
			AddRow(3, "APP", "T", "B", "IDX", "NONUNIQUE", "FUNCTION-BASED NORMAL", "SYS_NC1$"))
	dbMock.ExpectQuery("FROM cdb_tab_cols").WillReturnRows(sqlmock.NewRows([]string{"CON_ID", "OWNER", "TABLE_NAME", "COLUMN_NAME", "DATA_DEFAULT"}).
		AddRow(3, "APP", "T", "SYS_NC1$", "LOWER(C3)"))
	key := tableKey{conID: 3, owner: "APP", table: "T"}
	details := c.tableDetails(context.Background(), map[tableKey]struct{}{key: {}}, nil)
	require.Contains(t, details, key)
	indexes := details[key].Indexes
	require.Len(t, indexes, 2)
	require.Equal(t, "IDX", indexes[0].Name)
	require.Equal(t, "IDX", indexes[1].Name)
	require.True(t, indexes[0].Unique)
	require.False(t, indexes[1].Unique)
	require.Equal(t, columnParts("C1", "C2"), indexes[0].Columns)
	require.Equal(t, []indexKeyPart{{Expression: "LOWER(C3)"}}, indexes[1].Columns)
	require.NoError(t, dbMock.ExpectationsWereMet())
}

func TestColumnCommentsOnlyQuerySelectedColumns(t *testing.T) {
	c, _, dbMock, cleanup := newSchemaCheck(t)
	defer cleanup()
	dbMock.MatchExpectationsInOrder(false)
	key := tableKey{conID: 3, owner: "APP", table: "T"}
	allowedColumns := map[columnKey]struct{}{
		{tableKey: key, column: "C1"}: {},
	}
	dbMock.ExpectQuery(`(?s)FROM cdb_col_comments.*column_name IN \(:col0name0,`).WithArgs(schemaColumnArgs(3, "APP", "T", "C1")...).
		WillReturnRows(sqlmock.NewRows([]string{"CON_ID", "OWNER", "TABLE_NAME", "COLUMN_NAME", "COMMENTS"}).
			AddRow(3, "APP", "T", "C1", "Selected column"))
	details := c.tableDetails(context.Background(), map[tableKey]struct{}{key: {}}, allowedColumns)
	require.Contains(t, details, key)
	require.Equal(t, map[string]string{"C1": "Selected column"}, details[key].ColumnComments)
	require.NoError(t, dbMock.ExpectationsWereMet())
}

func TestTableDetailsIndexesGroupByName(t *testing.T) {
	c, _, dbMock, closeDB := newSchemaCheck(t)
	defer closeDB()

	dbMock.MatchExpectationsInOrder(false)
	dbMock.ExpectQuery(`(?s)cdb_indexes.*i\.table_name IN \(:rel0name0,`).WithArgs(schemaRelationArgs(3, "APP", "ORDERS")...).WillReturnRows(
		sqlmock.NewRows([]string{"CON_ID", "TABLE_OWNER", "TABLE_NAME", "INDEX_OWNER", "INDEX_NAME", "UNIQUENESS", "INDEX_TYPE", "COLUMN_NAME"}).
			AddRow(3, "APP", "ORDERS", "APP", "ORDERS_COMPOSITE_IDX", "UNIQUE", "NORMAL", "STATUS").
			AddRow(3, "APP", "ORDERS", "APP", "ORDERS_COMPOSITE_IDX", "UNIQUE", "NORMAL", "CREATED_AT").
			AddRow(3, "APP", "ORDERS", "APP", "ORDERS_STATUS_IDX", "NONUNIQUE", "NORMAL", "STATUS"))

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
}

func TestTableDetailsIndexesFunctionBasedSubstitutesExpression(t *testing.T) {
	c, _, dbMock, closeDB := newSchemaCheck(t)
	defer closeDB()

	dbMock.MatchExpectationsInOrder(false)
	dbMock.ExpectQuery("cdb_indexes").WillReturnRows(
		sqlmock.NewRows([]string{"CON_ID", "TABLE_OWNER", "TABLE_NAME", "INDEX_OWNER", "INDEX_NAME", "UNIQUENESS", "INDEX_TYPE", "COLUMN_NAME"}).
			AddRow(3, "APP", "ORDERS", "APP", "ORDERS_FBI_IDX", "NONUNIQUE", "FUNCTION-BASED NORMAL", "SYS_NC00004$").
			AddRow(3, "APP", "ORDERS", "APP", "ORDERS_FBI_COMPOSITE_IDX", "NONUNIQUE", "FUNCTION-BASED NORMAL", "CUSTOMER_ID").
			AddRow(3, "APP", "ORDERS", "APP", "ORDERS_FBI_COMPOSITE_IDX", "NONUNIQUE", "FUNCTION-BASED NORMAL", "SYS_NC00005$"))
	dbMock.ExpectQuery("FROM cdb_tab_cols").WillReturnRows(sqlmock.NewRows([]string{"CON_ID", "OWNER", "TABLE_NAME", "COLUMN_NAME", "DATA_DEFAULT"}).
		AddRow(3, "APP", "ORDERS", "SYS_NC00004$", `UPPER("STATUS")`).
		AddRow(3, "APP", "ORDERS", "SYS_NC00005$", `UPPER("STATUS")`))

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

func TestTableDetailsCollectionPresence(t *testing.T) {
	for _, tc := range []struct {
		name            string
		constraints     []string
		constraintErr   error
		partial         bool
		blockchainErr   error
		immutableErr    error
		readOnly        bool
		wantConstraints string
		wantProperties  string
	}{
		{
			name:            "empty",
			wantConstraints: `[]`,
			wantProperties:  `[]`,
		},
		{
			name:            "populated",
			constraints:     []string{"ORDERS_PK", "ORDERS_UK"},
			readOnly:        true,
			wantConstraints: `[{"name":"ORDERS_PK","type":"unique","columns":["ID"]},{"name":"ORDERS_UK","type":"unique","columns":["ID"]}]`,
			wantProperties:  `["read_only"]`,
		},
		{
			name:            "one constraint removed",
			constraints:     []string{"ORDERS_PK"},
			wantConstraints: `[{"name":"ORDERS_PK","type":"unique","columns":["ID"]}]`,
			wantProperties:  `[]`,
		},
		{
			name:           "constraints unavailable",
			constraintErr:  errors.New("ORA-00942: table or view does not exist"),
			wantProperties: `[]`,
		},
		{
			name:           "constraint query failed",
			constraintErr:  errors.New("ORA-01031: insufficient privileges"),
			wantProperties: `[]`,
		},
		{
			name:           "constraint rows failed after one constraint",
			constraints:    []string{"ORDERS_PK", "ORDERS_UK"},
			partial:        true,
			wantProperties: `[]`,
		},
		{
			name:            "blockchain unavailable",
			blockchainErr:   errors.New("ORA-00942: table or view does not exist"),
			readOnly:        true,
			wantConstraints: `[]`,
		},
		{
			name:            "immutable query failed",
			immutableErr:    errors.New("ORA-01031: insufficient privileges"),
			readOnly:        true,
			wantConstraints: `[]`,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c, _, dbMock, closeDB := newSchemaCheck(t)
			defer closeDB()
			dbMock.MatchExpectationsInOrder(false)

			for _, query := range []struct {
				view string
				err  error
			}{
				{"cdb_blockchain_tables", tc.blockchainErr},
				{"cdb_immutable_tables", tc.immutableErr},
				{"cdb_mviews", nil},
				{"cdb_tab_comments", nil},
				{"cdb_indexes", nil},
				{"cdb_external_tables", nil},
				{"cdb_part_tables", nil},
			} {
				expected := dbMock.ExpectQuery("FROM " + query.view)
				if query.err != nil {
					expected.WillReturnError(query.err)
				} else {
					expected.WillReturnRows(sqlmock.NewRows([]string{"UNUSED"}))
				}
			}
			rows := sqlmock.NewRows([]string{
				"CON_ID", "OWNER", "TABLE_NAME", "CONSTRAINT_NAME", "CONSTRAINT_TYPE",
				"R_OWNER", "R_CONSTRAINT_NAME", "COLUMN_NAME", "SEARCH_CONDITION",
			})
			for _, name := range tc.constraints {
				rows.AddRow(3, "APP", "ORDERS", name, "U", "-", "-", "ID", nil)
			}
			if tc.partial {
				rows.RowError(1, errors.New("connection lost during fetch"))
			}
			expected := dbMock.ExpectQuery("FROM cdb_constraints")
			if tc.constraintErr != nil {
				expected.WillReturnError(tc.constraintErr)
			} else {
				expected.WillReturnRows(rows)
			}

			allowed := map[tableKey]struct{}{{conID: 3, owner: "APP", table: "ORDERS"}: {}}
			details := c.tableDetails(context.Background(), allowed, nil)
			require.NoError(t, dbMock.ExpectationsWereMet())
			var payload []byte
			collector := newSchemaCollector(t, &c, func(value []byte) { payload = value }, details,
				map[ownerKey]string{{conID: 3, owner: "APP"}: "1"}, map[int64]string{3: "PDB"})
			row := schemaRowDB{ConID: 3, Owner: "APP", TableName: "ORDERS", IotType: "-", ClusterName: "-"}
			if tc.readOnly {
				row.ReadOnly = "YES"
			}
			collector.add(row)
			collector.finish()

			var event struct {
				Metadata []struct {
					Schemas []struct {
						Tables []map[string]json.RawMessage `json:"tables"`
					} `json:"schemas"`
				} `json:"metadata"`
			}
			require.NoError(t, json.Unmarshal(payload, &event))
			require.Len(t, event.Metadata, 1)
			require.Len(t, event.Metadata[0].Schemas, 1)
			require.Len(t, event.Metadata[0].Schemas[0].Tables, 1)
			table := event.Metadata[0].Schemas[0].Tables[0]
			for field, want := range map[string]string{
				"constraints": tc.wantConstraints, "table_properties": tc.wantProperties,
			} {
				if want == "" {
					assert.NotContains(t, table, field)
				} else {
					assert.JSONEq(t, want, string(table[field]), field)
				}
			}
		})
	}
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
			AddRow(3, "APP", "ORDERS", "ORDERS_PK", "P", "-", "-", "ORDER_ID", nil).
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
	dbMock.ExpectQuery(`(?s)cdb_constraints c.*cc\.con_id = :rel0con AND cc\.owner = :rel0owner AND cc\.table_name IN \(:rel0name0,.*ORDER BY c.con_id, c.owner, c.table_name, c.constraint_name, cc.position, cc.column_name`).WithArgs(schemaRelationArgs(3, "APP", "ORDER_ITEMS")...).WillReturnRows(
		sqlmock.NewRows([]string{
			"CON_ID", "OWNER", "TABLE_NAME", "CONSTRAINT_NAME", "CONSTRAINT_TYPE",
			"R_OWNER", "R_CONSTRAINT_NAME", "COLUMN_NAME", "SEARCH_CONDITION",
		}).
			AddRow(3, "APP", "ORDER_ITEMS", "ITEMS_FK", "R", "APP", "ORDERS_PK", "ORDER_ID", nil).
			AddRow(3, "APP", "ORDER_ITEMS", "ITEMS_FK", "R", "APP", "ORDERS_PK", "LINE_NO", nil))
	dbMock.ExpectQuery(`(?s)SELECT c.con_id, c.owner, c.constraint_name, c.table_name.*c\.constraint_name IN \(:rel0name0,`).WithArgs(schemaRelationArgs(3, "APP", "ORDERS_PK")...).WillReturnRows(
		sqlmock.NewRows([]string{"CON_ID", "OWNER", "CONSTRAINT_NAME", "TABLE_NAME"}).
			AddRow(3, "APP", "ORDERS_PK", "ORDERS"))
	columnArgs := schemaRelationArgs(3, "APP", "ORDERS")
	columnArgs = append(columnArgs, schemaBindTestArgs("targetConstraint", []string{"ORDERS_PK"}, schemaRelationPageSize)...)
	dbMock.ExpectQuery(`(?s)FROM cdb_cons_columns cc.*cc\.table_name IN \(:rel0name0,.*cc\.constraint_name IN \(:targetConstraint0,`).WithArgs(columnArgs...).WillReturnRows(
		sqlmock.NewRows([]string{"CON_ID", "OWNER", "CONSTRAINT_NAME", "TABLE_NAME", "COLUMN_NAME"}).
			AddRow(3, "APP", "ORDERS_PK", "ORDERS", "ORDER_ID").
			AddRow(3, "APP", "ORDERS_PK", "ORDERS", "LINE_NO"))

	page := map[tableKey]struct{}{{conID: 3, owner: "APP", table: "ORDER_ITEMS"}: {}}
	selected := map[tableKey]struct{}{
		{conID: 3, owner: "APP", table: "ORDER_ITEMS"}: {},
		{conID: 3, owner: "APP", table: "ORDERS"}:      {},
	}
	details := c.tableConstraintsForPage(context.Background(), page, selected)

	fk := details[tableKey{conID: 3, owner: "APP", table: "ORDER_ITEMS"}].Constraints[0]
	assert.Equal(t, "ORDERS", fk.ReferencedTable)
	assert.Equal(t, []string{"ORDER_ID", "LINE_NO"}, fk.ReferencedColumns)
	assert.Empty(t, fk.ReferencedConstraint)
	require.NoError(t, dbMock.ExpectationsWereMet())
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
	for _, tc := range []struct {
		version      string
		column       string
		tableVersion string
	}{
		{"12.1.0.2.0", "NULL", ""},
		{"12.2.0.1.0", "NULL", ""},
		{"19.0.0.0.0", "NULL", ""},
		{"21.3.0.0.0", "NULL", ""},
		{"23.0.0.0.0", "table_version", "v2"},
		{"26.0.0.0.0", "table_version", "v2"},
		{"unknown", "NULL", ""},
	} {
		t.Run(tc.version, func(t *testing.T) {
			c, _, dbMock, closeDB := newSchemaCheck(t)
			defer closeDB()
			c.dbVersion = tc.version
			dbMock.MatchExpectationsInOrder(false)
			var tableVersion any
			if tc.tableVersion != "" {
				tableVersion = tc.tableVersion
			}
			dbMock.ExpectQuery(regexp.QuoteMeta("hash_algorithm, " + tc.column + " AS table_version FROM cdb_blockchain_tables")).WillReturnRows(
				sqlmock.NewRows([]string{
					"CON_ID", "SCHEMA_NAME", "TABLE_NAME", "ROW_RETENTION", "ROW_RETENTION_LOCKED",
					"TABLE_INACTIVITY_RETENTION", "HASH_ALGORITHM", "TABLE_VERSION",
				}).AddRow(3, "APP", "LEDGER", 90, "YES", 30, "SHA2_512", tableVersion))
			dbMock.ExpectQuery("cdb_immutable_tables").WillReturnRows(
				sqlmock.NewRows([]string{
					"CON_ID", "SCHEMA_NAME", "TABLE_NAME", "ROW_RETENTION", "ROW_RETENTION_LOCKED",
					"TABLE_INACTIVITY_RETENTION",
				}).AddRow(3, "APP", "AUDIT_LOG", 365, "NO", nil))
			for _, view := range []string{"cdb_mviews", "cdb_tab_comments", "cdb_indexes", "cdb_constraints", "cdb_external_tables", "cdb_part_tables"} {
				dbMock.ExpectQuery("FROM " + view).WillReturnRows(sqlmock.NewRows([]string{"UNUSED"}))
			}

			allowed := map[tableKey]struct{}{
				{conID: 3, owner: "APP", table: "LEDGER"}:    {},
				{conID: 3, owner: "APP", table: "AUDIT_LOG"}: {},
				{conID: 3, owner: "APP", table: "ORDERS"}:    {},
			}
			details := c.tableDetails(context.Background(), allowed, nil)
			require.NoError(t, dbMock.ExpectationsWereMet())

			ledger := details[tableKey{conID: 3, owner: "APP", table: "LEDGER"}].Blockchain
			require.NotNil(t, ledger)
			require.NotNil(t, ledger.RowRetentionDays)
			assert.Equal(t, int64(90), *ledger.RowRetentionDays)
			assert.True(t, ledger.RowRetentionLocked)
			assert.Equal(t, "SHA2_512", ledger.HashAlgorithm)
			assert.Equal(t, tc.tableVersion, ledger.TableVersion)

			audit := details[tableKey{conID: 3, owner: "APP", table: "AUDIT_LOG"}].Immutable
			require.NotNil(t, audit)
			assert.False(t, audit.RowRetentionLocked)
			assert.Nil(t, audit.InactivityRetentionDays, "a NULL inactivity retention must stay nil, not zero")
			assert.Empty(t, audit.HashAlgorithm, "immutable tables carry no hash algorithm")

			var event schemaEvent
			collector := newSchemaCollector(t, &c, func(payload []byte) {
				require.NoError(t, json.Unmarshal(payload, &event))
			}, details, map[ownerKey]string{{conID: 3, owner: "APP"}: "1"}, map[int64]string{3: "PDB"})
			collector.add(schemaRowDB{ConID: 3, Owner: "APP", TableName: "ORDERS", Temporary: "Y", IotType: "-", ClusterName: "-"})
			collector.finish()
			orders := findTable([]schemaEvent{event}, "APP", "ORDERS")
			require.NotNil(t, orders)
			assert.Equal(t, []string{"temporary"}, orders.Properties)
		})
	}
}

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

func TestSchemaIdentityLimits(t *testing.T) {
	for _, kind := range []string{"table", "view"} {
		for _, count := range []int{0, 1, 2, 3} {
			t.Run(fmt.Sprintf("%s/%d", kind, count), func(t *testing.T) {
				c, _, dbMock, closeDB := newSchemaCheck(t)
				defer closeDB()
				const limit = 2
				var selected []tableKey
				for i := 0; i < count; i++ {
					selected = append(selected, tableKey{conID: 3, owner: "APP", table: fmt.Sprintf("R%d", i)})
				}
				dbMock.ExpectQuery("SELECT con_id, owner, " + kind + "_name").
					WithArgs(schemaIdentityArgs(3, limit+1, []string{"APP"})...).
					WillReturnRows(identityRows(selected...))
				discover := c.tableIdentities
				if kind == "view" {
					discover = c.viewIdentities
				}
				keys, truncated, err := discover(context.Background(), ownerListChunks([]string{"APP"}),
					map[ownerKey]string{{conID: 3, owner: "APP"}: "1"}, schemaSQL{}, limit)
				require.NoError(t, err)
				require.Equal(t, selected[:min(count, limit)], keys)
				_, capped := truncated[3]
				require.Equal(t, count > limit, capped)
				require.NoError(t, dbMock.ExpectationsWereMet())
			})
		}
	}
}

func TestMaxColumnsCap(t *testing.T) {
	for _, tc := range []struct {
		name         string
		limit, total int
	}{
		{name: "above_cap", limit: 2, total: 4},
		{name: "under_cap", limit: 50, total: 1},
		{name: "exactly_at_cap", limit: 2, total: 2},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c, _, _, closeDB := newSchemaCheck(t)
			defer closeDB()
			c.config.Schemas.MaxColumns = tc.limit
			var payloads []schemaEvent
			collector := newSchemaCollector(t, &c, func(b []byte) {
				var event schemaEvent
				require.NoError(t, json.Unmarshal(b, &event))
				payloads = append(payloads, event)
			}, nil, nil, nil)
			for i := range min(tc.limit, tc.total) {
				collector.add(schemaRowDB{
					ConID: 3, Owner: "APP", TableName: "T", Temporary: "N", External: "NO",
					IotType: "-", ClusterName: "-", Partitioned: "NO",
					TotalColumns: sql.NullInt64{Int64: int64(tc.total), Valid: true},
					ColumnName:   fmt.Sprintf("C%d", i), DataType: sql.NullString{String: "NUMBER", Valid: true}, Nullable: "Y",
				})
			}
			collector.finish()
			require.Len(t, payloads, 1)
			require.Len(t, payloads[0].Metadata, 1)
			require.Len(t, payloads[0].Metadata[0].Schemas, 1)
			require.Len(t, payloads[0].Metadata[0].Schemas[0].Tables, 1)
			assert.Len(t, payloads[0].Metadata[0].Schemas[0].Tables[0].Columns, min(tc.limit, tc.total))
			assert.Equal(t, tc.total > tc.limit, payloads[0].Truncated)
		})
	}
}

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

	owners, names, err := c.schemaOwners(context.Background(), map[int64]string{3: "APP_PDB"})
	require.NoError(t, err)
	assert.Len(t, names, total, "every owner beyond the 1000-item IN-list cap must still be returned")
	assert.Len(t, owners, total)

	chunks := ownerListChunks(names)
	require.Len(t, chunks, 2, "1010 owners must split into two IN-list batches")
	assert.Equal(t, sql.NullString{String: "APP0999", Valid: true}, chunks[0].args[999].(sql.NamedArg).Value)
	assert.Equal(t, chunks[0].text, chunks[1].text)
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
	keys, truncated, err := c.tableIdentities(context.Background(), ownerListChunks([]string{"APP"}), owners, schemaSQL{}, 2)
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
	assert.Contains(t, tableIdentitiesQueryTemplate, "NOT EXISTS (\n\t\t\t\tSELECT 1 FROM (")
	assert.Contains(t, schemasQueryTemplate, "NOT EXISTS (\n\t\tSELECT 1 FROM cdb_object_tables")
}

func TestSchemaDiscoveryObjectBarrier(t *testing.T) {
	owners := map[ownerKey]string{
		{conID: 3, owner: "APP"}:       "1",
		{conID: 3, owner: "REPORTING"}: "2",
		{conID: 4, owner: "APP"}:       "1",
	}
	ownerLists := ownerListChunks([]string{"APP", "REPORTING"})
	filter := schemaSQL{text: "AND t.table_name <> :excluded", args: []any{sql.Named("excluded", "SKIP")}}

	template := tableIdentitiesQueryTemplate
	require.Contains(t, template, "ORDER BY owner, table_name")
	require.Contains(t, template, "ROWNUM <= /*IDENTITY_LIMIT*/")
	c, db, mock, cleanup := newSchemaCheck(t)
	defer cleanup()
	var statements []schemaSQL
	c.schemaQueryer = schemaQueryFunc(func(ctx context.Context, query string, args ...any) (*sqlx.Rows, error) {
		statements = append(statements, schemaSQL{text: query, args: append([]any(nil), args...)})
		return db.QueryxContext(ctx, query, args...)
	})
	mock.ExpectQuery("cdb_object_tables").WillReturnRows(sqlmock.NewRows(
		[]string{"CON_ID", "OWNER", "TABLE_NAME"}).
		AddRow(3, "APP", "A").AddRow(3, "APP", "B").AddRow(3, "REPORTING", "A"))
	mock.ExpectQuery("cdb_object_tables").WillReturnRows(sqlmock.NewRows(
		[]string{"CON_ID", "OWNER", "TABLE_NAME"}).
		AddRow(4, "APP", "A").AddRow(4, "APP", "B"))
	keys, truncated, err := c.tableIdentities(context.Background(), ownerLists, owners, filter, 2)
	require.NoError(t, err)
	require.Len(t, statements, 2)
	require.Equal(t, []tableKey{
		{conID: 3, owner: "APP", table: "A"}, {conID: 3, owner: "APP", table: "B"},
		{conID: 4, owner: "APP", table: "A"}, {conID: 4, owner: "APP", table: "B"},
	}, keys)
	require.Equal(t, map[int64]struct{}{3: {}}, truncated)
	for i, statement := range statements {
		require.Equal(t, sql.Named("con_id", int64(3+i)), statement.args[0])
		require.Equal(t, sql.Named("identity_limit", 3), statement.args[1])
		require.Equal(t, 2, strings.Count(statement.text, filter.text))
		require.NotContains(t, statement.text, "/*")
		require.Contains(t, statement.text, "WHERE con_id = :con_id AND owner IN ("+ownerLists[0].text+")")
		require.Contains(t, statement.text, "AND ROWNUM > 0")
	}
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestOwnerListForKeysIsDistinctAndSorted(t *testing.T) {
	keys := []tableKey{
		{conID: 3, owner: "REPORTING", table: "R"},
		{conID: 3, owner: "APP", table: "B"},
		{conID: 3, owner: "APP", table: "A"},
	}
	query := ownerListForKeys(keys)
	require.Equal(t, sql.NullString{String: "APP", Valid: true}, query.args[0].(sql.NamedArg).Value)
	require.Equal(t, sql.NullString{String: "REPORTING", Valid: true}, query.args[1].(sql.NamedArg).Value)
	require.Len(t, query.args, schemaRelationPageSize)
}

func TestForEachTablePageBoundsPageSize(t *testing.T) {
	keys := make([]tableKey, schemaRelationPageSize*2+1)
	var pageSizes []int
	err := forEachTablePage(keys, schemaRelationPageSize, func(page []tableKey) error {
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
	require.NoError(t, forEachTablePage(keys, schemaRelationPageSize, func(page []tableKey) error {
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
	coordinator := newSchemaSnapshotCoordinator(c.nextSnapshotID, func(payload []byte) {
		var event schemaEvent
		require.NoError(t, json.Unmarshal(payload, &event))
		payloads = append(payloads, event)
	})
	keys := make([]tableKey, schemaRelationPageSize+1)
	for i := range keys {
		keys[i] = tableKey{conID: 3, owner: "APP", table: fmt.Sprintf("T%03d", i)}
	}
	require.NoError(t, forEachTablePage(keys, schemaRelationPageSize, func(page []tableKey) error {
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

func TestHydrateTablePageSkipsDisappearedTables(t *testing.T) {
	for _, allGone := range []bool{false, true} {
		t.Run(fmt.Sprintf("all_gone_%t", allGone), func(t *testing.T) {
			c, _, dbMock, cleanup := newSchemaCheck(t)
			defer cleanup()
			keys := []tableKey{{conID: 3, owner: "APP", table: "DROPPED"}, {conID: 3, owner: "APP", table: "KEPT"}}
			rows := emptyTablesRows()
			if !allGone {
				addTableRow(rows, 3, "APP", "KEPT")
			}
			dbMock.ExpectQuery("WITH ranked_columns").WillReturnRows(rows)
			var collected []string
			require.NoError(t, c.hydrateTablePage(context.Background(), keys, 50, func(row schemaRowDB) {
				collected = append(collected, row.TableName)
			}))
			if allGone {
				require.Empty(t, collected)
			} else {
				require.Equal(t, []string{"KEPT"}, collected)
			}
			require.NoError(t, dbMock.ExpectationsWereMet())
		})
	}
}

func TestSnapshotKeepsTableWhenCappedViewsDisappear(t *testing.T) {
	db, dbMock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()
	c, sender := newDbDoesNotExistCheck(t, "", "")
	c.db = sqlx.NewDb(db, "sqlmock")
	c.clock = clock.NewMock()
	c.dbVersion = "23.0.0.0.0"
	c.config.Schemas.MaxViews = 1
	dbMock.ExpectQuery(regexp.QuoteMeta(containerNamesQuery)).WillReturnRows(
		sqlmock.NewRows([]string{"CON_ID", "NAME"}).AddRow(3, "APP_PDB"))
	dbMock.ExpectQuery("cdb_users").WillReturnRows(
		sqlmock.NewRows([]string{"CON_ID", "USERNAME", "USER_ID"}).AddRow(3, "APP", 104))
	dbMock.ExpectQuery("SELECT con_id, owner, table_name").WillReturnRows(
		identityRows(tableKey{conID: 3, owner: "APP", table: "T"}))
	dbMock.ExpectQuery("FROM cdb_constraints").WillReturnRows(emptyConstraintRows())
	dbMock.ExpectQuery("WITH ranked_columns").WillReturnRows(addTableWithoutColumns(emptyTablesRows(), 3, "APP", "T"))
	for _, view := range []string{
		"cdb_blockchain_tables", "cdb_immutable_tables", "cdb_mviews", "cdb_tab_comments", "cdb_col_comments",
		"cdb_tab_cols", "cdb_indexes",
	} {
		dbMock.ExpectQuery("FROM " + view).WillReturnRows(sqlmock.NewRows([]string{"UNUSED"}))
	}
	dbMock.ExpectQuery("SELECT con_id, owner, view_name").WillReturnRows(
		identityRows(tableKey{conID: 3, owner: "APP", table: "V1"}, tableKey{conID: 3, owner: "APP", table: "V2"}))
	dbMock.ExpectQuery("WITH ranked_columns").WillReturnRows(sqlmock.NewRows(viewRelationColumns))
	expectSchemaContainerAvailable(dbMock, 3)
	require.NoError(t, c.SchemaCollection())
	var events []schemaEvent
	for _, call := range sender.Calls {
		if call.Method == "EventPlatformEvent" {
			var event schemaEvent
			require.NoError(t, json.Unmarshal(call.Arguments.Get(0).([]byte), &event))
			events = append(events, event)
		}
	}
	require.Len(t, events, 1)
	require.Equal(t, 1, events[0].CollectionPayloadsCount)
	require.True(t, events[0].Truncated)
	require.Len(t, events[0].Metadata[0].Schemas, 1)
	require.Len(t, events[0].Metadata[0].Schemas[0].Tables, 1)
	require.Equal(t, "T", events[0].Metadata[0].Schemas[0].Tables[0].Name)
	require.NoError(t, dbMock.ExpectationsWereMet())
}

func TestHydrateTablePagePropagatesQueryAndRowErrors(t *testing.T) {
	for _, rowError := range []bool{false, true} {
		t.Run(fmt.Sprintf("row_error_%t", rowError), func(t *testing.T) {
			c, _, dbMock, cleanup := newSchemaCheck(t)
			defer cleanup()
			queryErr := context.DeadlineExceeded
			query := dbMock.ExpectQuery("WITH ranked_columns")
			if rowError {
				query.WillReturnRows(addTableRow(emptyTablesRows(), 3, "APP", "T").RowError(0, queryErr))
			} else {
				query.WillReturnError(queryErr)
			}
			err := c.hydrateTablePage(context.Background(), []tableKey{{conID: 3, owner: "APP", table: "T"}}, 50, func(schemaRowDB) {})
			require.ErrorIs(t, err, queryErr)
			require.NoError(t, dbMock.ExpectationsWereMet())
		})
	}
}

func TestSchemaCollectorKeepsTableWithNoEligibleColumns(t *testing.T) {
	c, _, dbMock, closeDB := newSchemaCheck(t)
	defer closeDB()

	var events []schemaEvent
	collector := newSchemaCollector(t, &c, func(payload []byte) {
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
	firstPage := emptyTablesRows()
	for i := 0; i <= schemaRelationPageSize; i++ {
		name := fmt.Sprintf("T%03d", i)
		identityRows.AddRow(3, "APP", name)
		if i < schemaRelationPageSize {
			addTableRow(firstPage, 3, "APP", name)
		}
	}
	dbMock.ExpectQuery("cdb_object_tables").WillReturnRows(identityRows)
	dbMock.ExpectQuery("WITH ranked_columns").WillReturnRows(firstPage)
	dbMock.ExpectQuery("WITH ranked_columns").WillReturnError(errors.New("second page failed"))
	dbMock.ExpectQuery("FROM cdb_constraints").WillReturnRows(emptyConstraintRows())
	dbMock.ExpectQuery("FROM cdb_col_comments").WillReturnRows(sqlmock.NewRows([]string{"UNUSED"}))
	dbMock.ExpectQuery("FROM cdb_tab_cols").WillReturnRows(sqlmock.NewRows([]string{"UNUSED"}))
	for _, view := range []string{
		"cdb_blockchain_tables", "cdb_immutable_tables", "cdb_mviews", "cdb_tab_comments",
		"cdb_indexes",
	} {
		dbMock.ExpectQuery("FROM " + view).WillReturnRows(sqlmock.NewRows([]string{"UNUSED"}))
	}

	require.ErrorContains(t, c.SchemaCollection(), "second page failed")
	sender.AssertNumberOfCalls(t, "EventPlatformEvent", schemaRelationPageSize/50)
	sender.AssertNumberOfCalls(t, "Commit", 1)
	for _, call := range sender.Calls {
		if call.Method != "EventPlatformEvent" {
			continue
		}
		var event schemaEvent
		require.NoError(t, json.Unmarshal(call.Arguments.Get(0).([]byte), &event))
		assert.Len(t, event.Metadata[0].Schemas[0].Tables, 50)
		assert.Zero(t, event.CollectionPayloadsCount)
	}
	require.NoError(t, dbMock.ExpectationsWereMet())
}

func TestEmptyContainerStillEmitsTerminatingPayload(t *testing.T) {
	c, _, _, closeDB := newSchemaCheck(t)
	defer closeDB()

	var payloads []schemaEvent
	collector := newSchemaCollector(t, &c, func(b []byte) {
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
	collector := newSchemaCollector(t, &c, func(b []byte) {
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
	dbMock.ExpectQuery(`(?s)data_default_vc.*c\.column_name IN \(:col0name0,`).WithArgs(schemaColumnArgs(3, "APP", "T", "C1")...).WillReturnRows(
		sqlmock.NewRows([]string{"CON_ID", "OWNER", "TABLE_NAME", "COLUMN_NAME", "DATA_DEFAULT"}).
			AddRow(3, "APP", "T", "C1", strings.Repeat("x", 4500)))
	details := c.tableDetails(context.Background(), allowed, allowedColumns)

	var payloads []schemaEvent
	collector := newSchemaCollector(t, &c, func(b []byte) {
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
			waitForSchemaWorker(t, &c)

			if tc.wantGateOpen {
				require.NoError(t, err)
				assert.NoError(t, dbMock.ExpectationsWereMet(), "the gate must have opened and queried cdb_users")
			} else {
				require.NoError(t, err, "the gate must stay closed and touch the database not at all")
			}
		})
	}
}

func TestRegexSQLClausesUsesOracleCaseInsensitiveMatching(t *testing.T) {
	require.Equal(t, schemaSQL{}, regexSQLClauses("name", nil, nil))
	query := regexSQLClauses("name", []string{"^orders$", "o'brien"}, []string{"^tmp"})
	require.Equal(t, " AND NOT REGEXP_LIKE(name, :exclude0, 'i') AND (REGEXP_LIKE(name, :include0, 'i') OR REGEXP_LIKE(name, :include1, 'i'))", query.text)
	require.Equal(t, []any{sql.Named("exclude0", "^tmp"), sql.Named("include0", "^orders$"), sql.Named("include1", "o'brien")}, query.args)
	backref := regexSQLClauses("name", []string{`^([a-z]+)\1$`}, nil)
	require.Equal(t, []any{sql.Named("include0", `^([a-z]+)\1$`)}, backref.args)
}

func TestSchemaOwnersUsesOracleFilters(t *testing.T) {
	c, _, dbMock, cleanup := newSchemaCheck(t)
	defer cleanup()
	c.config.Schemas.IncludeSchemas = []string{"^app"}
	c.config.Schemas.ExcludeSchemas = []string{"_tmp$"}
	query := schemaOwnersQuery + " AND NOT REGEXP_LIKE(username, :exclude0, 'i') AND (REGEXP_LIKE(username, :include0, 'i'))"
	dbMock.ExpectQuery(regexp.QuoteMeta(query)).WillReturnRows(
		sqlmock.NewRows([]string{"CON_ID", "USERNAME", "USER_ID"}).AddRow(3, "APP", 104))
	owners, names, err := c.schemaOwners(context.Background(), map[int64]string{3: "APP_PDB"})
	require.NoError(t, err)
	require.Equal(t, []string{"APP"}, names)
	require.Equal(t, "104", owners[ownerKey{conID: 3, owner: "APP"}])
	require.NoError(t, dbMock.ExpectationsWereMet())
}

func TestContainerNamesUsesOracleFilters(t *testing.T) {
	c, _, dbMock, cleanup := newSchemaCheck(t)
	defer cleanup()
	c.config.Schemas.IncludeDatabases = []string{"pdb$"}
	c.config.Schemas.ExcludeDatabases = []string{"^tmp"}
	query := containerNamesQuery + " AND NOT REGEXP_LIKE(name, :exclude0, 'i') AND (REGEXP_LIKE(name, :include0, 'i'))"
	dbMock.ExpectQuery(regexp.QuoteMeta(query)).WillReturnRows(
		sqlmock.NewRows([]string{"CON_ID", "NAME"}).AddRow(3, "APP_PDB"))
	names, err := c.containerNames(context.Background())
	require.NoError(t, err)
	require.Equal(t, map[int64]string{3: "APP_PDB"}, names)
	require.NoError(t, dbMock.ExpectationsWereMet())
}

func TestSchemaCollectionFilterErrorsDoNotEmitEmptySnapshot(t *testing.T) {
	for _, stage := range []string{"database", "schema", "table"} {
		t.Run(stage, func(t *testing.T) {
			db, dbMock, err := sqlmock.New()
			require.NoError(t, err)
			defer db.Close()
			c, sender := newDbDoesNotExistCheck(t, "", "")
			c.db = sqlx.NewDb(db, "sqlmock")
			c.dbVersion = "23.0.0.0.0"
			c.config.Schemas.IncludeDatabases = []string{"["}
			c.config.Schemas.IncludeSchemas = []string{"["}
			c.config.Schemas.IncludeTables = []string{"["}
			queryErr := errors.New("ORA-12726: unmatched bracket in regular expression")
			containerQuery := dbMock.ExpectQuery(regexp.QuoteMeta(containerNamesQuery))
			if stage == "database" {
				containerQuery.WillReturnError(queryErr)
			} else {
				containerQuery.WillReturnRows(sqlmock.NewRows([]string{"CON_ID", "NAME"}).AddRow(3, "APP_PDB"))
				ownerQuery := dbMock.ExpectQuery("cdb_users")
				if stage == "schema" {
					ownerQuery.WillReturnError(queryErr)
				} else {
					ownerQuery.WillReturnRows(sqlmock.NewRows([]string{"CON_ID", "USERNAME", "USER_ID"}).AddRow(3, "APP", 104))
					dbMock.ExpectQuery("SELECT con_id, owner, table_name").WillReturnError(queryErr)
				}
			}
			require.ErrorIs(t, c.SchemaCollection(), queryErr)
			sender.AssertNotCalled(t, "EventPlatformEvent", mock.Anything, mock.Anything)
			require.NoError(t, dbMock.ExpectationsWereMet())
		})
	}
}

func TestSchemaCollectionAppliesTableIncludeExcludeFilters(t *testing.T) {
	c, _, dbMock, closeDB := newSchemaCheck(t)
	defer closeDB()
	dbMock.MatchExpectationsInOrder(false)

	c.config.Schemas.IncludeTables = []string{"^ORD"}
	c.config.Schemas.ExcludeTables = []string{"_STAGING$"}

	dbMock.ExpectQuery(`v\$containers`).WillReturnRows(sqlmock.NewRows([]string{"CON_ID", "NAME"}).AddRow(3, "APP_PDB"))
	dbMock.ExpectQuery("cdb_users").WillReturnRows(
		sqlmock.NewRows([]string{"CON_ID", "USERNAME", "USER_ID"}).AddRow(3, "APP", 104))

	expectedFilter := regexSQLClauses("t.table_name", c.config.Schemas.IncludeTables, c.config.Schemas.ExcludeTables)
	require.Contains(t, expectedFilter.text, "REGEXP_LIKE")

	dbMock.ExpectQuery(regexp.QuoteMeta(expectedFilter.text)).WillReturnRows(sqlmock.NewRows(
		[]string{"CON_ID", "OWNER", "TABLE_NAME"}))
	dbMock.ExpectQuery("SELECT con_id, owner, view_name").WillReturnRows(identityRows())

	expectSchemaContainerAvailable(dbMock, 3)
	require.NoError(t, c.SchemaCollection())
	assert.NoError(t, dbMock.ExpectationsWereMet(), "the query actually sent to Oracle must carry the substituted REGEXP_LIKE filter")
}

func TestViewCollectionAppliesTableIncludeExcludeFilters(t *testing.T) {
	c, _, dbMock, closeDB := newSchemaCheck(t)
	defer closeDB()
	dbMock.MatchExpectationsInOrder(false)

	c.config.Schemas.IncludeTables = []string{"^ORD"}
	c.config.Schemas.ExcludeTables = []string{"^TMP_"}

	dbMock.ExpectQuery(`v\$containers`).WillReturnRows(sqlmock.NewRows([]string{"CON_ID", "NAME"}).AddRow(3, "APP_PDB"))
	dbMock.ExpectQuery("cdb_users").WillReturnRows(
		sqlmock.NewRows([]string{"CON_ID", "USERNAME", "USER_ID"}).AddRow(3, "APP", 104))

	tableFilter := regexSQLClauses("t.table_name", c.config.Schemas.IncludeTables, c.config.Schemas.ExcludeTables)
	dbMock.ExpectQuery(regexp.QuoteMeta(tableFilter.text)).WillReturnRows(identityRows())

	viewFilter := regexSQLClauses("v.view_name", c.config.Schemas.IncludeTables, c.config.Schemas.ExcludeTables)
	require.Contains(t, viewFilter.text, "REGEXP_LIKE")
	dbMock.ExpectQuery(regexp.QuoteMeta(viewFilter.text)).WillReturnRows(identityRows())

	expectSchemaContainerAvailable(dbMock, 3)
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

	expectSchemaContainerAvailable(dbMock, 3)
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
	"OBJECT_TYPE_OWNER", "OBJECT_TYPE", "COLUMN_PRESENT",
	"COLUMN_NAME", "COLUMN_ID", "INTERNAL_COLUMN_ID", "VIRTUAL_COLUMN", "HIDDEN_COLUMN", "DATA_TYPE",
	"DATA_TYPE_OWNER", "DATA_TYPE_MOD", "DATA_LENGTH", "CHAR_LENGTH", "DATA_PRECISION",
	"DATA_SCALE", "CHAR_USED", "NULLABLE", "DATA_DEFAULT_VC", "TOTAL_COLUMNS",
}

func addViewRow(rows *sqlmock.Rows, conID int64, owner, viewName string, totalColumns int) *sqlmock.Rows {
	return rows.AddRow(
		conID, owner, viewName, "N", "-", "NO", "-", "NO", "-", "NO", "NO",
		"-", "-", 1, "C1", 1, 1, "NO", "NO", "NUMBER", nil, nil, 22, nil, 12, 0, "-", "N", nil, totalColumns,
	)
}

func addViewWithoutColumns(rows *sqlmock.Rows, conID int64, owner, viewName string) *sqlmock.Rows {
	return rows.AddRow(
		conID, owner, viewName, "N", "-", "NO", "-", "NO", "-", "NO", "NO",
		"-", "-", 0, "-", nil, nil, "-", "-", nil, nil, nil, nil, nil, nil, nil, "-", "-", nil, nil,
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
	require.NoError(t, forEachTablePage(keys, schemaRelationPageSize, func(page []tableKey) error {
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

func TestViewPageRowsSkipsDisappearedViews(t *testing.T) {
	for _, allGone := range []bool{false, true} {
		t.Run(fmt.Sprintf("all_gone_%t", allGone), func(t *testing.T) {
			c, _, dbMock, cleanup := newSchemaCheck(t)
			defer cleanup()
			keys := []tableKey{{conID: 3, owner: "APP", table: "DROPPED"}, {conID: 3, owner: "APP", table: "KEPT"}}
			rows := sqlmock.NewRows(viewRelationColumns)
			if !allGone {
				addViewRow(rows, 3, "APP", "KEPT", 1)
			}
			dbMock.ExpectQuery("WITH ranked_columns").WillReturnRows(rows)
			collected, err := c.viewPageRows(context.Background(), keys, 50)
			require.NoError(t, err)
			if allGone {
				require.Empty(t, collected)
			} else {
				require.Len(t, collected, 1)
				require.Equal(t, "KEPT", collected[0].TableName)
			}
			require.NoError(t, dbMock.ExpectationsWereMet())
		})
	}
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
	created := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	modified := created.Add(time.Hour)
	dbMock.ExpectQuery("SELECT con_id, owner, object_name, created, last_ddl_time").WillReturnRows(
		sqlmock.NewRows([]string{"CON_ID", "OWNER", "OBJECT_NAME", "CREATED", "LAST_DDL_TIME"}).
			AddRow(3, "APP", "V1", created, modified).
			AddRow(3, "APP", "V2", created, modified))
	dbMock.ExpectQuery("cdb_tab_comments").WillReturnRows(
		sqlmock.NewRows([]string{"CON_ID", "OWNER", "TABLE_NAME", "COMMENTS"}))

	details := c.viewDetailsForPage(context.Background(), map[tableKey]struct{}{key: {}})
	require.Len(t, details, 1)
	assert.Equal(t, "SELECT 1 FROM dual", details[key].Definition)
	assert.Equal(t, created.Format(time.RFC3339), details[key].CreateDate)
	assert.Equal(t, modified.Format(time.RFC3339), details[key].ModifyDate)
	assert.NotContains(t, details, tableKey{conID: 3, owner: "APP", table: "V2"})
	require.NoError(t, dbMock.ExpectationsWereMet())
}

func TestSchemaCollectionSkipsOwnerContainerAbsentFromAvailabilityLookup(t *testing.T) {
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
	require.Empty(t, events)
	require.NoError(t, dbMock.ExpectationsWereMet())
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
	dbMock.ExpectQuery("SELECT con_id, owner, table_name.*t.con_id = :con_id").WithArgs(schemaIdentityArgs(3, c.config.Schemas.MaxTables+1, []string{"APP"})...).WillReturnError(errors.New("identity failed"))
	dbMock.ExpectQuery("SELECT con_id, owner, table_name.*t.con_id = :con_id").WithArgs(schemaIdentityArgs(4, c.config.Schemas.MaxTables+1, []string{"APP"})...).WillReturnRows(identityRows())
	dbMock.ExpectQuery("SELECT con_id, owner, view_name.*v.con_id = :con_id").WithArgs(schemaIdentityArgs(4, c.config.Schemas.MaxViews+1, []string{"APP"})...).WillReturnRows(identityRows())

	expectSchemaContainerAvailable(dbMock, 4)
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

func addTableRow(rows *sqlmock.Rows, conID int64, owner, table string) *sqlmock.Rows {
	return rows.AddRow(
		conID, owner, table, "N", "-", "NO", "-", "NO", "-", "NO", "NO", "-", "-",
		1, "C1", 1, 1, "NO", "NO", "NUMBER", nil, nil, 22, nil, 12, 0, "-", "Y", nil,
	)
}

func addTableWithoutColumns(rows *sqlmock.Rows, conID int64, owner, table string) *sqlmock.Rows {
	return rows.AddRow(
		conID, owner, table, "N", "-", "NO", "-", "NO", "-", "NO", "NO", "-", "-",
		0, "-", nil, nil, "-", "-", nil, nil, nil, nil, nil, nil, nil, "-", "-", nil,
	)
}

func emptyTablesRows() *sqlmock.Rows {
	return sqlmock.NewRows([]string{
		"CON_ID", "OWNER", "TABLE_NAME", "TEMPORARY", "DURATION", "EXTERNAL", "IOT_TYPE",
		"PARTITIONED", "CLUSTER_NAME", "CLUSTERING", "READ_ONLY",
		"OBJECT_TYPE_OWNER", "OBJECT_TYPE",
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

func TestMaxViewsCap(t *testing.T) {
	for _, tc := range []struct {
		name      string
		available int
		limit     int
		collected int
		truncated bool
	}{
		{name: "above_cap", available: 3, limit: 2, collected: 2, truncated: true},
		{name: "under_cap", available: 1, limit: 50, collected: 1},
		{name: "exactly_at_cap", available: 2, limit: 2, collected: 2},
	} {
		t.Run(tc.name, func(t *testing.T) {
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
			c.config.Schemas.MaxViews = tc.limit

			dbMock.ExpectQuery(`v\$containers`).WillReturnRows(
				sqlmock.NewRows([]string{"CON_ID", "NAME"}).AddRow(3, "APP_PDB"))
			dbMock.ExpectQuery("cdb_users").WillReturnRows(
				sqlmock.NewRows([]string{"CON_ID", "USERNAME", "USER_ID"}).AddRow(3, "APP", 104))
			dbMock.ExpectQuery("SELECT con_id, owner, table_name").WillReturnRows(identityRows())

			identities := identityRows()
			rows := sqlmock.NewRows(viewRelationColumns)
			for i := range tc.available {
				name := fmt.Sprintf("V%d", i+1)
				identities.AddRow(3, "APP", name)
				if i < tc.collected {
					addViewRow(rows, 3, "APP", name, 1)
				}
			}
			dbMock.ExpectQuery("SELECT con_id, owner, view_name").WillReturnRows(identities)
			dbMock.ExpectQuery(`(?s)WITH ranked_columns.*FROM cdb_views v`).WillReturnRows(rows)

			expectSchemaContainerAvailable(dbMock, 3)
			require.NoError(t, c.SchemaCollection())

			byContainer := viewPayloadsByContainer(t, &sender.Mock)
			require.Contains(t, byContainer, "3")
			views := byContainer["3"]
			require.Len(t, views.Metadata[0].Schemas, 1)
			assert.Len(t, views.Metadata[0].Schemas[0].Views, tc.collected)
			assert.Equal(t, tc.truncated, views.Truncated)
		})
	}
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
	dbMock.ExpectQuery("SELECT con_id, owner, table_name.*t.con_id = :con_id").WithArgs(schemaIdentityArgs(3, c.config.Schemas.MaxTables+1, []string{"APP"})...).WillReturnRows(identityRows())
	dbMock.ExpectQuery("SELECT con_id, owner, table_name.*t.con_id = :con_id").WithArgs(schemaIdentityArgs(4, c.config.Schemas.MaxTables+1, []string{"APP"})...).WillReturnRows(identityRows())

	dbMock.ExpectQuery("SELECT con_id, owner, view_name.*v.con_id = :con_id").WithArgs(schemaIdentityArgs(3, c.config.Schemas.MaxViews+1, []string{"APP"})...).WillReturnRows(identityRows(
		tableKey{conID: 3, owner: "APP", table: "V1"}, tableKey{conID: 3, owner: "APP", table: "V2"},
		tableKey{conID: 3, owner: "APP", table: "V3"}))
	rows3 := sqlmock.NewRows(viewRelationColumns)
	addViewRow(rows3, 3, "APP", "V1", 1)
	addViewRow(rows3, 3, "APP", "V2", 1)
	dbMock.ExpectQuery(`(?s)WITH ranked_columns.*FROM cdb_views v`).WithArgs(append([]driver.Value{sql.Named("max_columns", c.config.Schemas.MaxColumns)}, schemaRelationArgs(3, "APP", "V1", "V2")...)...).WillReturnRows(rows3)
	dbMock.ExpectQuery("SELECT con_id, owner, view_name.*v.con_id = :con_id").WithArgs(schemaIdentityArgs(4, c.config.Schemas.MaxViews+1, []string{"APP"})...).WillReturnRows(identityRows(
		tableKey{conID: 4, owner: "APP", table: "V4"}, tableKey{conID: 4, owner: "APP", table: "V5"}))
	rows4 := sqlmock.NewRows(viewRelationColumns)
	addViewRow(rows4, 4, "APP", "V4", 1)
	addViewRow(rows4, 4, "APP", "V5", 1)
	dbMock.ExpectQuery(`(?s)WITH ranked_columns.*FROM cdb_views v`).WithArgs(append([]driver.Value{sql.Named("max_columns", c.config.Schemas.MaxColumns)}, schemaRelationArgs(4, "APP", "V4", "V5")...)...).WillReturnRows(rows4)

	expectSchemaContainerAvailable(dbMock, 3)
	expectSchemaContainerAvailable(dbMock, 4)
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

func TestSchemaSnapshotAfterDisappearedPage(t *testing.T) {
	for _, outcome := range []string{"all_gone", "capped", "query_error", "row_error"} {
		t.Run(outcome, func(t *testing.T) {
			db, dbMock, err := sqlmock.New()
			require.NoError(t, err)
			defer db.Close()
			c, sender := newDbDoesNotExistCheck(t, "", "")
			c.db = sqlx.NewDb(db, "sqlmock")
			c.clock = clock.NewMock()
			c.dbVersion = "23.0.0.0.0"
			views := false
			c.config.Schemas.CollectViews = &views
			c.config.Schemas.MaxTables = schemaRelationPageSize + 1
			dbMock.ExpectQuery(regexp.QuoteMeta(containerNamesQuery)).WillReturnRows(
				sqlmock.NewRows([]string{"CON_ID", "NAME"}).AddRow(3, "APP_PDB"))
			dbMock.ExpectQuery("cdb_users").WillReturnRows(
				sqlmock.NewRows([]string{"CON_ID", "USERNAME", "USER_ID"}).AddRow(3, "APP", 104))
			identities := sqlmock.NewRows([]string{"CON_ID", "OWNER", "TABLE_NAME"})
			for i := 0; i <= schemaRelationPageSize; i++ {
				identities.AddRow(3, "APP", fmt.Sprintf("T%03d", i))
			}
			if outcome == "capped" {
				identities.AddRow(3, "APP", fmt.Sprintf("T%03d", schemaRelationPageSize+1))
			}
			dbMock.ExpectQuery("SELECT con_id, owner, table_name").WillReturnRows(identities)
			dbMock.ExpectQuery("FROM cdb_constraints").WillReturnRows(emptyConstraintRows())
			dbMock.ExpectQuery("WITH ranked_columns").WillReturnRows(emptyTablesRows())
			lastPage := dbMock.ExpectQuery("WITH ranked_columns")
			switch outcome {
			case "all_gone", "capped":
				lastPage.WillReturnRows(emptyTablesRows())
			case "query_error":
				lastPage.WillReturnError(context.DeadlineExceeded)
			case "row_error":
				lastPage.WillReturnRows(addTableRow(emptyTablesRows(), 3, "APP", fmt.Sprintf("T%03d", schemaRelationPageSize)).RowError(0, context.DeadlineExceeded))
			}
			if outcome == "all_gone" || outcome == "capped" {
				expectSchemaContainerAvailable(dbMock, 3)
			}
			err = c.SchemaCollection()
			if outcome == "all_gone" || outcome == "capped" {
				require.NoError(t, err)
			} else {
				require.ErrorIs(t, err, context.DeadlineExceeded)
			}
			var events []schemaEvent
			for _, call := range sender.Calls {
				if call.Method == "EventPlatformEvent" {
					var event schemaEvent
					require.NoError(t, json.Unmarshal(call.Arguments.Get(0).([]byte), &event))
					events = append(events, event)
				}
			}
			if outcome == "all_gone" || outcome == "capped" {
				require.Len(t, events, 1)
				require.Equal(t, 1, events[0].CollectionPayloadsCount)
				require.Empty(t, events[0].Metadata[0].Schemas)
				require.Equal(t, outcome == "capped", events[0].Truncated)
			} else {
				require.Empty(t, events)
			}
			require.NoError(t, dbMock.ExpectationsWereMet())
		})
	}
}

func TestViewSnapshotAfterDisappearedPage(t *testing.T) {
	for _, outcome := range []string{"all_gone", "capped", "query_error", "row_error"} {
		t.Run(outcome, func(t *testing.T) {
			db, dbMock, err := sqlmock.New()
			require.NoError(t, err)
			defer db.Close()
			c, sender := newDbDoesNotExistCheck(t, "", "")
			c.db = sqlx.NewDb(db, "sqlmock")
			c.clock = clock.NewMock()
			c.dbVersion = "23.0.0.0.0"
			views := true
			c.config.Schemas.CollectViews = &views
			c.config.Schemas.MaxViews = schemaRelationPageSize + 1
			dbMock.ExpectQuery(regexp.QuoteMeta(containerNamesQuery)).WillReturnRows(
				sqlmock.NewRows([]string{"CON_ID", "NAME"}).AddRow(3, "APP_PDB"))
			dbMock.ExpectQuery("cdb_users").WillReturnRows(
				sqlmock.NewRows([]string{"CON_ID", "USERNAME", "USER_ID"}).AddRow(3, "APP", 104))
			identities := sqlmock.NewRows([]string{"CON_ID", "OWNER", "TABLE_NAME"})
			for i := 0; i <= schemaRelationPageSize; i++ {
				identities.AddRow(3, "APP", fmt.Sprintf("T%03d", i))
			}
			if outcome == "capped" {
				identities.AddRow(3, "APP", "T101")
			}
			dbMock.ExpectQuery("SELECT con_id, owner, table_name").WillReturnRows(identityRows())
			dbMock.ExpectQuery("SELECT con_id, owner, view_name").WillReturnRows(identities)
			dbMock.ExpectQuery("WITH ranked_columns").WillReturnRows(sqlmock.NewRows(viewRelationColumns))
			lastPage := dbMock.ExpectQuery("WITH ranked_columns")
			switch outcome {
			case "all_gone", "capped":
				lastPage.WillReturnRows(sqlmock.NewRows(viewRelationColumns))
			case "query_error":
				lastPage.WillReturnError(context.DeadlineExceeded)
			case "row_error":
				lastPage.WillReturnRows(addViewRow(sqlmock.NewRows(viewRelationColumns), 3, "APP", "T100", 0).RowError(0, context.DeadlineExceeded))
			}
			if outcome == "all_gone" || outcome == "capped" {
				expectSchemaContainerAvailable(dbMock, 3)
			}
			err = c.SchemaCollection()
			if outcome == "all_gone" || outcome == "capped" {
				require.NoError(t, err)
			} else {
				require.ErrorIs(t, err, context.DeadlineExceeded)
			}
			var events []schemaEvent
			for _, call := range sender.Calls {
				if call.Method == "EventPlatformEvent" {
					var event schemaEvent
					require.NoError(t, json.Unmarshal(call.Arguments.Get(0).([]byte), &event))
					events = append(events, event)
				}
			}
			if outcome == "all_gone" || outcome == "capped" {
				require.Len(t, events, 1)
				require.Equal(t, 1, events[0].CollectionPayloadsCount)
				require.Empty(t, events[0].Metadata[0].Schemas)
				require.Equal(t, outcome == "capped", events[0].Truncated)
			} else {
				require.Empty(t, events)
			}
			require.NoError(t, dbMock.ExpectationsWereMet())
		})
	}
}

func schemaBindTestArgs(prefix string, names []string, size int) []driver.Value {
	args := make([]driver.Value, size)
	for i := range args {
		var value sql.NullString
		if i < len(names) {
			value = sql.NullString{String: names[i], Valid: true}
		}
		args[i] = sql.Named(fmt.Sprintf("%s%d", prefix, i), value)
	}
	return args
}

func schemaRelationArgs(conID int64, owner string, names ...string) []driver.Value {
	args := []driver.Value{sql.Named("rel0con", conID), sql.Named("rel0owner", owner)}
	return append(args, schemaBindTestArgs("rel0name", names, schemaRelationPageSize)...)
}

func schemaColumnArgs(conID int64, owner, table string, columns ...string) []driver.Value {
	args := []driver.Value{sql.Named("col0con", conID), sql.Named("col0owner", owner), sql.Named("col0table", table)}
	return append(args, schemaBindTestArgs("col0name", columns, schemaColumnBindSlots)...)
}

func schemaIdentityArgs(conID int64, limit int, owners []string) []driver.Value {
	args := []driver.Value{sql.Named("con_id", conID), sql.Named("identity_limit", limit)}
	return append(args, schemaBindTestArgs("owner", owners, maxSchemaOwners)...)
}

func namesForSchemaTest(n int) []string {
	names := make([]string, n)
	for i := range names {
		names[i] = fmt.Sprintf("APP%04d", i)
	}
	return names
}

func TestColumnFilterChunksSharesIdenticalSets(t *testing.T) {
	build := func(owner, tablePrefix, columnPrefix string, count int) schemaSQL {
		allowed := make(map[columnKey]struct{})
		for table := count - 1; table >= 0; table-- {
			for column := 9; column >= 0; column-- {
				allowed[columnKey{tableKey: tableKey{conID: 3, owner: owner, table: fmt.Sprintf("%s%03d", tablePrefix, table)}, column: fmt.Sprintf("%s%02d", columnPrefix, column)}] = struct{}{}
			}
		}
		filters := columnFilterChunks(allowed, relationColumnNames{conID: "c.con_id", owner: "c.owner", relation: "c.table_name"}, "c.column_name")
		require.Len(t, filters, 1)
		return filters[0]
	}
	first := build("APP", "T", "C", 100)
	second := build("O'WNER", "T'", "C'", 99)
	require.Equal(t, first.text, second.text)
	require.Len(t, first.args, 2+schemaRelationPageSize+schemaColumnBindSlots)
	require.Contains(t, first.text, "c.table_name IN (:rel0name0,")
	require.Contains(t, first.text, "c.column_name IN (:sharedColumn0,")
	require.Equal(t, sql.Named("rel0con", int64(3)), first.args[0])
	require.Equal(t, sql.Named("rel0owner", "APP"), first.args[1])
	require.Equal(t, sql.Named("rel0name0", sql.NullString{String: "T000", Valid: true}), first.args[2])
	require.Equal(t, sql.Named("rel0name99", sql.NullString{String: "T099", Valid: true}), first.args[101])
	require.Equal(t, sql.Named("sharedColumn0", sql.NullString{String: "C00", Valid: true}), first.args[102])
	require.Equal(t, sql.Named("sharedColumn9", sql.NullString{String: "C09", Valid: true}), first.args[111])
	for _, arg := range first.args[112:] {
		require.Equal(t, sql.NullString{}, arg.(sql.NamedArg).Value)
	}
	require.NotContains(t, second.text, "O'WNER")
	require.NotContains(t, second.text, "T'")
	require.NotContains(t, second.text, "C'")
}

func TestColumnFilterChunksPreservesExactPairsAndFallbacks(t *testing.T) {
	for _, scenario := range []string{"shared", "hidden", "local_container", "column_cap", "different_columns", "different_counts", "owners", "containers", "one_table", "chunk_boundary", "wide_boundary"} {
		t.Run(scenario, func(t *testing.T) {
			allowed := make(map[columnKey]struct{})
			add := func(conID int64, owner, table string, names ...string) {
				for _, name := range names {
					allowed[columnKey{tableKey: tableKey{conID: conID, owner: owner, table: table}, column: name}] = struct{}{}
				}
			}
			add(3, "APP", "A", "ID", "VALUE")
			add(3, "APP", "B", "ID", "VALUE")
			wantShared, wantQueries := true, 1
			switch scenario {
			case "hidden", "local_container":
				for _, table := range []string{"A", "B"} {
					add(3, "APP", table, "SYS_NC00002$", "SYS_NC_OID$", "SYS_NC_USER")
				}
			case "column_cap":
				delete(allowed, columnKey{tableKey: tableKey{conID: 3, owner: "APP", table: "A"}, column: "VALUE"})
				wantShared = false
			case "different_columns":
				delete(allowed, columnKey{tableKey: tableKey{conID: 3, owner: "APP", table: "B"}, column: "VALUE"})
				add(3, "APP", "B", "OTHER")
				wantShared = false
			case "different_counts":
				add(3, "APP", "B", "THIRD", "FOURTH")
				wantShared = false
			case "owners":
				add(3, "OTHER", "A", "ID", "VALUE")
				wantShared = false
			case "containers":
				add(4, "APP", "A", "ID", "VALUE")
				wantShared = false
			case "one_table":
				clear(allowed)
				add(3, "APP", "A", "ID", "VALUE")
				wantShared = false
			case "chunk_boundary":
				clear(allowed)
				for table := 0; table < 501; table++ {
					add(3, "APP", fmt.Sprintf("T%04d", table), "ID", "VALUE")
				}
				wantQueries = 2
			case "wide_boundary":
				clear(allowed)
				for table := 0; table < 3; table++ {
					for column := 0; column < 600; column++ {
						add(3, "APP", fmt.Sprintf("T%d", table), fmt.Sprintf("C%04d", column))
					}
				}
				wantShared, wantQueries = false, 2
			}
			containerColumn := "c.con_id"
			if scenario == "local_container" {
				containerColumn = schemaCurrentContainerID
			}
			filters := columnFilterChunks(allowed, relationColumnNames{conID: containerColumn, owner: "c.owner", relation: "c.table_name"}, "c.column_name")
			require.Len(t, filters, wantQueries)
			if scenario == "local_container" {
				require.Contains(t, filters[0].text, schemaCurrentContainerID+" = :rel0con")
				require.NotContains(t, filters[0].text, "c.con_id")
			}
			require.Equal(t, wantShared, strings.Contains(filters[0].text, ":sharedColumn0"))
			actual := make(map[columnKey]struct{}, len(allowed))
			for _, filter := range filters {
				args := make(map[string]any, len(filter.args))
				for _, arg := range filter.args {
					bound := arg.(sql.NamedArg)
					require.NotContains(t, args, bound.Name)
					args[bound.Name] = bound.Value
				}
				for _, list := range regexp.MustCompile(`IN \(([^)]*)\)`).FindAllStringSubmatch(filter.text, -1) {
					require.LessOrEqual(t, len(strings.Split(list[1], ",")), maxSchemaRelationsPerQuery)
				}
				appendPair := func(key tableKey, column string) {
					pair := columnKey{tableKey: key, column: column}
					require.NotContains(t, actual, pair)
					actual[pair] = struct{}{}
				}
				if strings.Contains(filter.text, ":sharedColumn0") {
					for name, value := range args {
						if !strings.HasPrefix(name, "rel0name") || !value.(sql.NullString).Valid {
							continue
						}
						key := tableKey{conID: args["rel0con"].(int64), owner: args["rel0owner"].(string), table: value.(sql.NullString).String}
						for name, value := range args {
							if strings.HasPrefix(name, "sharedColumn") && value.(sql.NullString).Valid {
								appendPair(key, value.(sql.NullString).String)
							}
						}
					}
				} else {
					require.Contains(t, filter.text, "c.table_name = :col0table")
					for group := 0; ; group++ {
						prefix := fmt.Sprintf("col%d", group)
						table, ok := args[prefix+"table"]
						if !ok {
							break
						}
						conID := args[prefix+"con"]
						if conID == nil {
							conID = args["pagecon"]
						}
						key := tableKey{conID: conID.(int64), owner: args[prefix+"owner"].(string), table: table.(string)}
						for name, value := range args {
							if strings.HasPrefix(name, prefix+"name") && value.(sql.NullString).Valid {
								appendPair(key, value.(sql.NullString).String)
							}
						}
					}
				}
			}
			require.Equal(t, allowed, actual)
		})
	}
}

func emptyConstraintRows() *sqlmock.Rows {
	return sqlmock.NewRows([]string{
		"CON_ID", "OWNER", "TABLE_NAME", "CONSTRAINT_NAME", "CONSTRAINT_TYPE",
		"R_OWNER", "R_CONSTRAINT_NAME", "COLUMN_NAME", "SEARCH_CONDITION",
	})
}

func TestSchemaConstraintWindowsKeepHydrationPagesBounded(t *testing.T) {
	for _, scenario := range []string{"complete", "cross_window_fk", "second_window_deadline", "properties_incomplete"} {
		t.Run(scenario, func(t *testing.T) {
			c, db, dbMock, cleanup := newSchemaCheck(t)
			defer cleanup()
			views := false
			c.config.Schemas.CollectViews = &views
			c.config.Schemas.MaxTables = schemaConstraintPageSize + 1
			c.config.Schemas.MaxColumns = 100
			var events []schemaEvent
			c.schemaEmitter = func(payload []byte) {
				var event schemaEvent
				require.NoError(t, json.Unmarshal(payload, &event))
				events = append(events, event)
			}
			constraintQueries, hydrationQueries := 0, 0
			c.schemaQueryer = schemaQueryFunc(func(ctx context.Context, query string, args ...any) (*sqlx.Rows, error) {
				if strings.Contains(query, "c.constraint_type IN") {
					constraintQueries++
					if constraintQueries == 2 {
						require.NotEmpty(t, events, "the first window must emit before loading the second")
					}
				}
				if strings.HasPrefix(query, "WITH ranked_columns") {
					hydrationQueries++
				}
				return db.QueryxContext(ctx, query, args...)
			})
			dbMock.ExpectQuery(regexp.QuoteMeta(containerNamesQuery)).WillReturnRows(
				sqlmock.NewRows([]string{"CON_ID", "NAME"}).AddRow(3, "APP_PDB"))
			dbMock.ExpectQuery("cdb_users").WillReturnRows(
				sqlmock.NewRows([]string{"CON_ID", "USERNAME", "USER_ID"}).AddRow(3, "APP", 104))
			identities := identityRows()
			names := make([]string, schemaConstraintPageSize+1)
			for i := range names {
				names[i] = fmt.Sprintf("T%04d", i)
				identities.AddRow(3, "APP", names[i])
			}
			dbMock.ExpectQuery("SELECT con_id, owner, table_name").WillReturnRows(identities)
			for start := 0; start < len(names); start += schemaConstraintPageSize {
				end := min(start+schemaConstraintPageSize, len(names))
				args := []driver.Value{sql.Named("rel0con", int64(3)), sql.Named("rel0owner", "APP")}
				slots := schemaRelationPageSize
				if end-start > slots {
					slots = schemaConstraintPageSize
				}
				args = append(args, schemaBindTestArgs("rel0name", names[start:end], slots)...)
				if scenario == "second_window_deadline" && start == schemaConstraintPageSize {
					dbMock.ExpectQuery("cdb_constraints c").WithArgs(args...).WillReturnError(context.DeadlineExceeded)
					break
				}
				constraints := emptyConstraintRows().AddRow(3, "APP", names[start], "PK_"+names[start], "P", "-", "-", "C1", nil)
				if scenario == "cross_window_fk" {
					if start == 0 {
						constraints.AddRow(3, "APP", names[999], "CHILD_FK", "R", "APP", "PK_"+names[1000], "C1", nil)
						constraints.AddRow(3, "APP", names[999], "CHILD_FK", "R", "APP", "PK_"+names[1000], "C2", nil)
					} else {
						constraints.AddRow(3, "APP", names[start], "PK_"+names[start], "P", "-", "-", "C2", nil)
					}
				}
				dbMock.ExpectQuery("cdb_constraints c").WithArgs(args...).WillReturnRows(constraints)
				if scenario == "cross_window_fk" && start == 0 {
					dbMock.ExpectQuery("SELECT c.con_id, c.owner, c.constraint_name").
						WithArgs(schemaRelationArgs(3, "APP", "PK_"+names[1000])...).WillReturnRows(
						sqlmock.NewRows([]string{"CON_ID", "OWNER", "CONSTRAINT_NAME", "TABLE_NAME"}).
							AddRow(3, "APP", "PK_"+names[1000], names[1000]))
					columnArgs := schemaRelationArgs(3, "APP", names[1000])
					columnArgs = append(columnArgs, schemaBindTestArgs("targetConstraint", []string{"PK_" + names[1000]}, schemaRelationPageSize)...)
					dbMock.ExpectQuery("FROM cdb_cons_columns cc").WithArgs(columnArgs...).WillReturnRows(
						sqlmock.NewRows([]string{"CON_ID", "OWNER", "CONSTRAINT_NAME", "TABLE_NAME", "COLUMN_NAME"}).
							AddRow(3, "APP", "PK_"+names[1000], names[1000], "C1").
							AddRow(3, "APP", "PK_"+names[1000], names[1000], "C2"))
				}
				for pageStart := start; pageStart < end; pageStart += schemaRelationPageSize {
					pageEnd := min(pageStart+schemaRelationPageSize, end)
					rows := emptyTablesRows()
					for _, name := range names[pageStart:pageEnd] {
						addTableRow(rows, 3, "APP", name)
					}
					pageArgs := []driver.Value{sql.Named("max_columns", 100), sql.Named("rel0con", int64(3)), sql.Named("rel0owner", "APP")}
					pageArgs = append(pageArgs, schemaBindTestArgs("rel0name", names[pageStart:pageEnd], schemaRelationPageSize)...)
					pageArgs = append(pageArgs, schemaBindTestArgs("pageOwner", []string{"APP"}, schemaRelationPageSize)...)
					dbMock.ExpectQuery("WITH ranked_columns").WithArgs(pageArgs...).WillReturnRows(rows)
					for _, view := range []string{
						"cdb_blockchain_tables", "cdb_immutable_tables", "cdb_mviews", "cdb_tab_comments", "cdb_col_comments",
						"cdb_tab_cols", "cdb_indexes",
					} {
						if scenario == "properties_incomplete" && view == "cdb_blockchain_tables" {
							dbMock.ExpectQuery("FROM " + view).WillReturnError(errors.New("ORA-00942: table or view does not exist"))
							continue
						}
						dbMock.ExpectQuery("FROM " + view).WillReturnRows(sqlmock.NewRows([]string{"UNUSED"}))
					}
				}
			}
			if scenario != "second_window_deadline" {
				expectSchemaContainerAvailable(dbMock, 3)
			}
			err := c.SchemaCollection()
			if scenario == "second_window_deadline" {
				require.ErrorIs(t, err, context.DeadlineExceeded)
				require.Equal(t, schemaConstraintPageSize/schemaRelationPageSize, hydrationQueries)
			} else {
				require.NoError(t, err)
				require.Equal(t, schemaConstraintPageSize/schemaRelationPageSize+1, hydrationQueries)
			}
			require.NoError(t, dbMock.ExpectationsWereMet())
			require.Equal(t, 2, constraintQueries)
			seen := make(map[string]bool)
			for _, event := range events {
				if scenario == "second_window_deadline" {
					require.Zero(t, event.CollectionPayloadsCount)
				}
				for _, schema := range event.Metadata[0].Schemas {
					for _, table := range schema.Tables {
						require.False(t, seen[table.Name])
						seen[table.Name] = true
						if scenario == "properties_incomplete" {
							require.Nil(t, table.Properties)
						} else {
							require.NotNil(t, table.Properties)
						}
						if scenario == "cross_window_fk" && table.Name == names[999] {
							require.Len(t, table.Constraints, 1)
							fk := table.Constraints[0]
							require.Equal(t, names[1000], fk.ReferencedTable)
							require.Equal(t, []string{"C1", "C2"}, fk.ReferencedColumns)
							require.Empty(t, fk.ReferencedConstraint)
						}
						if table.Name == names[0] || table.Name == names[schemaConstraintPageSize] {
							require.Len(t, table.Constraints, 1)
						}
					}
				}
			}
			if scenario == "second_window_deadline" {
				require.NotEmpty(t, seen)
				require.LessOrEqual(t, len(seen), schemaConstraintPageSize)
			} else {
				require.Len(t, seen, len(names))
				require.Equal(t, len(events), events[len(events)-1].CollectionPayloadsCount)
			}
		})
	}
}

func TestSchemaConstraintWindowResolvesTargetsAcrossHydrationPages(t *testing.T) {
	c, db, dbMock, cleanup := newSchemaCheck(t)
	defer cleanup()
	allowed := make(map[tableKey]struct{}, schemaConstraintPageSize)
	for i := 0; i < schemaConstraintPageSize; i++ {
		allowed[tableKey{conID: 3, owner: "APP", table: fmt.Sprintf("T%04d", i)}] = struct{}{}
	}
	dbMock.ExpectQuery("cdb_constraints c").WillReturnRows(emptyConstraintRows().
		AddRow(3, "APP", "T0000", "CHILD_FK", "R", "APP", "PARENT_PK", "PARENT_ID", nil).
		AddRow(3, "APP", "T0999", "PARENT_PK", "P", "-", "-", "ID", nil).
		AddRow(3, "APP", "T0999", "PARENT_PK", "P", "-", "-", "VERSION", nil))
	queries := 0
	c.schemaQueryer = schemaQueryFunc(func(ctx context.Context, query string, args ...any) (*sqlx.Rows, error) {
		queries++
		return db.QueryxContext(ctx, query, args...)
	})
	details := c.tableConstraintsForPage(context.Background(), allowed, allowed)
	require.NoError(t, dbMock.ExpectationsWereMet())
	require.Equal(t, 1, queries)
	child := details[tableKey{conID: 3, owner: "APP", table: "T0000"}]
	require.NotNil(t, child)
	require.Len(t, child.Constraints, 1)
	require.Equal(t, "T0999", child.Constraints[0].ReferencedTable)
	require.Equal(t, []string{"ID", "VERSION"}, child.Constraints[0].ReferencedColumns)
	require.Empty(t, child.Constraints[0].ReferencedConstraint)
}

func TestSchemaConstraintWindowDeduplicatesExcludedTargets(t *testing.T) {
	c, db, dbMock, cleanup := newSchemaCheck(t)
	defer cleanup()
	allowed := map[tableKey]struct{}{
		{conID: 3, owner: "APP", table: "FIRST"}: {},
		{conID: 3, owner: "APP", table: "LAST"}:  {},
	}
	dbMock.ExpectQuery("cdb_constraints c").WillReturnRows(emptyConstraintRows().
		AddRow(3, "APP", "FIRST", "FIRST_FK", "R", "OTHER", "PARENT_PK", "PARENT_ID", nil).
		AddRow(3, "APP", "LAST", "LAST_FK", "R", "OTHER", "PARENT_PK", "PARENT_ID", nil))
	dbMock.ExpectQuery("SELECT c.con_id, c.owner, c.constraint_name").WithArgs(schemaRelationArgs(3, "OTHER", "PARENT_PK")...).WillReturnRows(
		sqlmock.NewRows([]string{"CON_ID", "OWNER", "CONSTRAINT_NAME", "TABLE_NAME"}).
			AddRow(3, "OTHER", "PARENT_PK", "PARENT"))
	queries := 0
	c.schemaQueryer = schemaQueryFunc(func(ctx context.Context, query string, args ...any) (*sqlx.Rows, error) {
		queries++
		return db.QueryxContext(ctx, query, args...)
	})
	details := c.tableConstraintsForPage(context.Background(), allowed, allowed)
	require.NoError(t, dbMock.ExpectationsWereMet())
	require.Equal(t, 2, queries)
	for key := range allowed {
		require.NotNil(t, details[key])
		require.Len(t, details[key].Constraints, 1)
		fk := details[key].Constraints[0]
		require.Empty(t, fk.ReferencedTable)
		require.Empty(t, fk.ReferencedColumns)
		require.Equal(t, "PARENT_PK", fk.ReferencedConstraint)
	}
}

func TestSchemaConstraintWindowDoesNotResurrectMissingTables(t *testing.T) {
	c, _, dbMock, cleanup := newSchemaCheck(t)
	defer cleanup()
	views := false
	c.config.Schemas.CollectViews = &views
	c.config.Schemas.MaxTables = 1
	var events []schemaEvent
	c.schemaEmitter = func(payload []byte) {
		var event schemaEvent
		require.NoError(t, json.Unmarshal(payload, &event))
		events = append(events, event)
	}
	dbMock.ExpectQuery(regexp.QuoteMeta(containerNamesQuery)).WillReturnRows(
		sqlmock.NewRows([]string{"CON_ID", "NAME"}).AddRow(3, "APP_PDB"))
	dbMock.ExpectQuery("cdb_users").WillReturnRows(
		sqlmock.NewRows([]string{"CON_ID", "USERNAME", "USER_ID"}).AddRow(3, "APP", 104))
	dbMock.ExpectQuery("SELECT con_id, owner, table_name").WillReturnRows(
		identityRows(tableKey{conID: 3, owner: "APP", table: "GONE"}))
	dbMock.ExpectQuery("cdb_constraints c").WillReturnRows(emptyConstraintRows().
		AddRow(3, "APP", "GONE", "GONE_PK", "P", "-", "-", "ID", nil))
	dbMock.ExpectQuery("WITH ranked_columns").WillReturnRows(emptyTablesRows())
	expectSchemaContainerAvailable(dbMock, 3)
	require.NoError(t, c.SchemaCollection())
	require.NoError(t, dbMock.ExpectationsWereMet())
	require.Len(t, events, 1)
	require.Empty(t, events[0].Metadata[0].Schemas)
	require.Equal(t, 1, events[0].CollectionPayloadsCount)
}

func TestSchemaConstraintWindowFailurePolicy(t *testing.T) {
	for _, failure := range []error{errors.New(oracleErrorTableOrViewDoesNotExist), errors.New("optional metadata failed"), context.DeadlineExceeded} {
		t.Run(failure.Error(), func(t *testing.T) {
			c, db, dbMock, cleanup := newSchemaCheck(t)
			defer cleanup()
			views := false
			c.config.Schemas.CollectViews = &views
			c.config.Schemas.MaxTables = 1
			var events []schemaEvent
			c.schemaEmitter = func(payload []byte) {
				var event schemaEvent
				require.NoError(t, json.Unmarshal(payload, &event))
				events = append(events, event)
			}
			hydrationCalls := 0
			c.schemaQueryer = schemaQueryFunc(func(ctx context.Context, query string, args ...any) (*sqlx.Rows, error) {
				if strings.HasPrefix(query, "WITH ranked_columns") {
					hydrationCalls++
				}
				return db.QueryxContext(ctx, query, args...)
			})
			dbMock.ExpectQuery(regexp.QuoteMeta(containerNamesQuery)).WillReturnRows(
				sqlmock.NewRows([]string{"CON_ID", "NAME"}).AddRow(3, "APP_PDB"))
			dbMock.ExpectQuery("cdb_users").WillReturnRows(
				sqlmock.NewRows([]string{"CON_ID", "USERNAME", "USER_ID"}).AddRow(3, "APP", 104))
			dbMock.ExpectQuery("SELECT con_id, owner, table_name").WillReturnRows(
				identityRows(tableKey{conID: 3, owner: "APP", table: "T"}))
			dbMock.ExpectQuery("cdb_constraints c").WillReturnError(failure)
			if !errors.Is(failure, context.DeadlineExceeded) {
				dbMock.ExpectQuery("WITH ranked_columns").WillReturnRows(addTableRow(emptyTablesRows(), 3, "APP", "T"))
				for _, view := range []string{
					"cdb_blockchain_tables", "cdb_immutable_tables", "cdb_mviews", "cdb_tab_comments", "cdb_col_comments",
					"cdb_tab_cols", "cdb_indexes",
				} {
					dbMock.ExpectQuery("FROM " + view).WillReturnRows(sqlmock.NewRows([]string{"UNUSED"}))
				}
				expectSchemaContainerAvailable(dbMock, 3)
			}
			err := c.SchemaCollection()
			if errors.Is(failure, context.DeadlineExceeded) {
				require.ErrorIs(t, err, failure)
				require.Zero(t, hydrationCalls)
				require.Empty(t, events)
			} else {
				require.NoError(t, err)
				require.Equal(t, 1, hydrationCalls)
				require.Len(t, events, 1)
				require.Len(t, events[0].Metadata[0].Schemas[0].Tables, 1)
				require.Nil(t, events[0].Metadata[0].Schemas[0].Tables[0].Constraints)
				require.Equal(t, 1, events[0].CollectionPayloadsCount)
			}
			require.NoError(t, dbMock.ExpectationsWereMet())
		})
	}
}

func TestSchemaConstraintWindowDiscardsPartialRows(t *testing.T) {
	for _, failure := range []error{errors.New("optional row failed"), context.DeadlineExceeded} {
		t.Run(failure.Error(), func(t *testing.T) {
			c, _, dbMock, cleanup := newSchemaCheck(t)
			defer cleanup()
			allowed := map[tableKey]struct{}{
				{conID: 3, owner: "APP", table: "T"}: {},
				{conID: 3, owner: "APP", table: "U"}: {},
			}
			dbMock.ExpectQuery("cdb_constraints c").WillReturnRows(emptyConstraintRows().
				AddRow(3, "APP", "T", "PK", "P", "-", "-", "ID", nil).
				AddRow(3, "APP", "T", "PK", "P", "-", "-", "VERSION", nil).
				RowError(1, failure))
			details := c.tableConstraintsForPage(context.Background(), allowed, allowed)
			require.NoError(t, dbMock.ExpectationsWereMet())
			for key := range allowed {
				require.NotNil(t, details[key])
				require.Nil(t, details[key].Constraints)
			}
			if errors.Is(failure, context.DeadlineExceeded) {
				require.ErrorIs(t, c.schemaContextError(context.Background()), failure)
			} else {
				require.NoError(t, c.schemaContextError(context.Background()))
			}
		})
	}
}

func TestSchemaConstraintWindowDiscardsEarlierQueryOnFailure(t *testing.T) {
	for _, failure := range []error{errors.New("optional query failed"), context.DeadlineExceeded} {
		t.Run(failure.Error(), func(t *testing.T) {
			c, _, dbMock, cleanup := newSchemaCheck(t)
			defer cleanup()
			allowed := make(map[tableKey]struct{}, schemaConstraintPageSize+1)
			for i := 0; i <= schemaConstraintPageSize; i++ {
				allowed[tableKey{conID: 3, owner: "APP", table: fmt.Sprintf("T%04d", i)}] = struct{}{}
			}
			dbMock.ExpectQuery("cdb_constraints c").WillReturnRows(emptyConstraintRows().
				AddRow(3, "APP", "T0000", "PK", "P", "-", "-", "ID", nil))
			dbMock.ExpectQuery("cdb_constraints c").WillReturnError(failure)
			details := c.tableConstraintsForPage(context.Background(), allowed, allowed)
			require.Len(t, details, len(allowed))
			for key := range allowed {
				require.NotNil(t, details[key])
				require.Nil(t, details[key].Constraints)
			}
			if errors.Is(failure, context.DeadlineExceeded) {
				require.ErrorIs(t, c.schemaContextError(context.Background()), failure)
			} else {
				require.NoError(t, c.schemaContextError(context.Background()))
			}
			require.NoError(t, dbMock.ExpectationsWereMet())
		})
	}
}

func TestSchemaConstraintWindowPreservesSuccessfulEmptyCollections(t *testing.T) {
	c, _, dbMock, cleanup := newSchemaCheck(t)
	defer cleanup()
	allowed := map[tableKey]struct{}{
		{conID: 3, owner: "APP", table: "T"}: {},
		{conID: 3, owner: "APP", table: "U"}: {},
	}
	dbMock.ExpectQuery("cdb_constraints c").WillReturnRows(emptyConstraintRows())
	details := c.tableConstraintsForPage(context.Background(), allowed, allowed)
	for key := range allowed {
		require.NotNil(t, details[key])
		require.NotNil(t, details[key].Constraints)
		require.Empty(t, details[key].Constraints)
	}
	require.NoError(t, dbMock.ExpectationsWereMet())
}

func TestSchemaSparseDefaultQueriesKeepBothVersionPaths(t *testing.T) {
	require.Contains(t, columnDefaultsQuery, "WHERE /*RELATIONS*/ AND c.default_length > 0")
	require.Contains(t, localColumnValuesQuery, "WHERE /*RELATIONS*/ AND c.default_length > 0")
	require.Contains(t, localColumnValuesQuery, "c.data_default")
	require.Contains(t, localColumnValuesQuery, schemaIndexExpressionColumn)
}

func TestSchemaReferencedConstraintColumnsKeepExactTargetIdentities(t *testing.T) {
	c, db, dbMock, cleanup := newSchemaCheck(t)
	defer cleanup()
	allowed := map[tableKey]struct{}{
		{conID: 3, owner: "CHILD", table: "A"}: {},
		{conID: 3, owner: "CHILD", table: "B"}: {},
		{conID: 3, owner: "CHILD", table: "C"}: {},
		{conID: 4, owner: "CHILD", table: "D"}: {},
		{conID: 3, owner: "CHILD", table: "E"}: {},
		{conID: 3, owner: "CHILD", table: "F"}: {},
	}
	selected := make(map[tableKey]struct{})
	for key := range allowed {
		selected[key] = struct{}{}
	}
	for _, key := range []tableKey{
		{conID: 3, owner: "APP", table: "PARENT"},
		{conID: 3, owner: "APP", table: "REPLACEMENT"},
		{conID: 3, owner: "OTHER", table: "PARENT"},
		{conID: 4, owner: "APP", table: "PARENT"},
	} {
		selected[key] = struct{}{}
	}
	dbMock.ExpectQuery("cdb_constraints c").WillReturnRows(emptyConstraintRows().
		AddRow(3, "CHILD", "A", "A_FK", "R", "APP", "PK", "PARENT_ID", nil).
		AddRow(3, "CHILD", "B", "B_FK", "R", "APP", "PK", "PARENT_ID", nil).
		AddRow(3, "CHILD", "C", "C_FK", "R", "OTHER", "PK", "PARENT_ID", nil).
		AddRow(3, "CHILD", "E", "E_FK", "R", "EXCLUDED", "PK", "PARENT_ID", nil).
		AddRow(3, "CHILD", "F", "F_FK", "R", "MISSING", "PK", "PARENT_ID", nil).
		AddRow(4, "CHILD", "D", "D_FK", "R", "APP", "PK", "PARENT_ID", nil))
	dbMock.ExpectQuery("SELECT c.con_id, c.owner, c.constraint_name").WillReturnRows(
		sqlmock.NewRows([]string{"CON_ID", "OWNER", "CONSTRAINT_NAME", "TABLE_NAME"}).
			AddRow(3, "APP", "PK", "PARENT").
			AddRow(3, "EXCLUDED", "PK", "PARENT").
			AddRow(3, "OTHER", "PK", "PARENT").
			AddRow(4, "APP", "PK", "PARENT"))
	dbMock.ExpectQuery("FROM cdb_cons_columns cc").WillReturnRows(
		sqlmock.NewRows([]string{"CON_ID", "OWNER", "CONSTRAINT_NAME", "TABLE_NAME", "COLUMN_NAME"}).
			AddRow(3, "APP", "PK", "REPLACEMENT", "WRONG_TABLE").
			AddRow(3, "APP", "PK", "PARENT", "Z_FIRST").
			AddRow(3, "APP", "PK", "PARENT", "A_SECOND").
			AddRow(3, "APP", "UNREQUESTED", "PARENT", "WRONG_CONSTRAINT").
			AddRow(3, "EXCLUDED", "PK", "PARENT", "WRONG_OWNER").
			AddRow(3, "OTHER", "PK", "PARENT", "OTHER_ID").
			AddRow(4, "APP", "PK", "PARENT", "OTHER_PDB_ID").
			AddRow(5, "APP", "PK", "PARENT", "WRONG_PDB"))
	queries := 0
	c.schemaQueryer = schemaQueryFunc(func(ctx context.Context, query string, args ...any) (*sqlx.Rows, error) {
		queries++
		return db.QueryxContext(ctx, query, args...)
	})
	details := c.tableConstraintsForPage(context.Background(), allowed, selected)
	require.NoError(t, dbMock.ExpectationsWereMet())
	require.Equal(t, 3, queries)
	for key := range allowed {
		require.NotNil(t, details[key])
		require.Len(t, details[key].Constraints, 1)
		fk := details[key].Constraints[0]
		switch key.table {
		case "A", "B":
			require.Equal(t, []string{"Z_FIRST", "A_SECOND"}, fk.ReferencedColumns)
		case "C":
			require.Equal(t, []string{"OTHER_ID"}, fk.ReferencedColumns)
		case "D":
			require.Equal(t, []string{"OTHER_PDB_ID"}, fk.ReferencedColumns)
		case "E", "F":
			require.Empty(t, fk.ReferencedTable)
			require.Empty(t, fk.ReferencedColumns)
			require.Equal(t, "PK", fk.ReferencedConstraint)
			continue
		}
		require.Equal(t, "PARENT", fk.ReferencedTable)
		require.Empty(t, fk.ReferencedConstraint)
	}
}

func TestSchemaReferencedConstraintColumnsKeepFallbackUntilValidRow(t *testing.T) {
	for _, scenario := range []struct {
		name          string
		definitionErr error
		columnErr     error
		partial       bool
		changedTable  bool
	}{
		{name: "missing columns"},
		{name: "definition unavailable", definitionErr: errors.New(oracleErrorTableOrViewDoesNotExist)},
		{name: "definition timeout", definitionErr: context.DeadlineExceeded},
		{name: "definition canceled", definitionErr: context.Canceled},
		{name: "columns unavailable", columnErr: errors.New(oracleErrorInvalidIdentifier)},
		{name: "columns timeout", columnErr: context.DeadlineExceeded},
		{name: "columns canceled", columnErr: context.Canceled},
		{name: "partial ordinary error", columnErr: errors.New("column row failed"), partial: true},
		{name: "partial timeout", columnErr: context.DeadlineExceeded, partial: true},
		{name: "target recreated on another table", changedTable: true},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			c, db, dbMock, cleanup := newSchemaCheck(t)
			defer cleanup()
			child := tableKey{conID: 3, owner: "APP", table: "CHILD"}
			allowed := map[tableKey]struct{}{child: {}}
			selected := map[tableKey]struct{}{child: {}, {conID: 3, owner: "APP", table: "PARENT"}: {}, {conID: 3, owner: "APP", table: "REPLACEMENT"}: {}}
			dbMock.ExpectQuery("cdb_constraints c").WillReturnRows(emptyConstraintRows().
				AddRow(3, "APP", "CHILD", "FK", "R", "APP", "PK", "PARENT_ID", nil))
			definition := dbMock.ExpectQuery("SELECT c.con_id, c.owner, c.constraint_name")
			if scenario.definitionErr != nil {
				definition.WillReturnError(scenario.definitionErr)
			} else {
				definition.WillReturnRows(sqlmock.NewRows([]string{"CON_ID", "OWNER", "CONSTRAINT_NAME", "TABLE_NAME"}).AddRow(3, "APP", "PK", "PARENT"))
				columns := dbMock.ExpectQuery("FROM cdb_cons_columns cc")
				rows := sqlmock.NewRows([]string{"CON_ID", "OWNER", "CONSTRAINT_NAME", "TABLE_NAME", "COLUMN_NAME"})
				switch {
				case scenario.partial:
					columns.WillReturnRows(rows.AddRow(3, "APP", "PK", "PARENT", "FIRST").AddRow(3, "APP", "PK", "PARENT", "SECOND").RowError(1, scenario.columnErr))
				case scenario.columnErr != nil:
					columns.WillReturnError(scenario.columnErr)
				case scenario.changedTable:
					columns.WillReturnRows(rows.AddRow(3, "APP", "PK", "REPLACEMENT", "WRONG_ID"))
				default:
					columns.WillReturnRows(rows)
				}
			}
			queries := 0
			c.schemaQueryer = schemaQueryFunc(func(ctx context.Context, query string, args ...any) (*sqlx.Rows, error) {
				queries++
				return db.QueryxContext(ctx, query, args...)
			})
			details := c.tableConstraintsForPage(context.Background(), allowed, selected)
			require.NoError(t, dbMock.ExpectationsWereMet())
			if scenario.definitionErr != nil {
				require.Equal(t, 2, queries)
			} else {
				require.Equal(t, 3, queries)
			}
			require.NotNil(t, details[child])
			require.Len(t, details[child].Constraints, 1)
			fk := details[child].Constraints[0]
			if scenario.partial {
				require.Equal(t, "PARENT", fk.ReferencedTable)
				require.Equal(t, []string{"FIRST"}, fk.ReferencedColumns)
				require.Empty(t, fk.ReferencedConstraint)
			} else {
				require.Empty(t, fk.ReferencedTable)
				require.Empty(t, fk.ReferencedColumns)
				require.Equal(t, "PK", fk.ReferencedConstraint)
			}
			failure := scenario.definitionErr
			if failure == nil {
				failure = scenario.columnErr
			}
			if errors.Is(failure, context.DeadlineExceeded) || errors.Is(failure, context.Canceled) {
				require.ErrorIs(t, c.schemaContextError(context.Background()), failure)
			} else {
				require.NoError(t, c.schemaContextError(context.Background()))
			}
		})
	}
}

func TestSchemaReferencedConstraintDefinitionsPreservePartialRows(t *testing.T) {
	for _, failure := range []error{errors.New("definition row failed"), context.DeadlineExceeded} {
		t.Run(failure.Error(), func(t *testing.T) {
			c, db, dbMock, cleanup := newSchemaCheck(t)
			defer cleanup()
			first := tableKey{conID: 3, owner: "APP", table: "FIRST"}
			last := tableKey{conID: 3, owner: "APP", table: "LAST"}
			allowed := map[tableKey]struct{}{first: {}, last: {}}
			selected := map[tableKey]struct{}{first: {}, last: {}, {conID: 3, owner: "APP", table: "PARENT"}: {}}
			dbMock.ExpectQuery("cdb_constraints c").WillReturnRows(emptyConstraintRows().
				AddRow(3, "APP", "FIRST", "FIRST_FK", "R", "APP", "FIRST_PK", "PARENT_ID", nil).
				AddRow(3, "APP", "LAST", "LAST_FK", "R", "APP", "LAST_PK", "PARENT_ID", nil))
			dbMock.ExpectQuery("SELECT c.con_id, c.owner, c.constraint_name").WillReturnRows(
				sqlmock.NewRows([]string{"CON_ID", "OWNER", "CONSTRAINT_NAME", "TABLE_NAME"}).
					AddRow(3, "APP", "FIRST_PK", "PARENT").
					AddRow(3, "APP", "LAST_PK", "PARENT").RowError(1, failure))
			if !errors.Is(failure, context.DeadlineExceeded) {
				dbMock.ExpectQuery("FROM cdb_cons_columns cc").WillReturnRows(
					sqlmock.NewRows([]string{"CON_ID", "OWNER", "CONSTRAINT_NAME", "TABLE_NAME", "COLUMN_NAME"}).
						AddRow(3, "APP", "FIRST_PK", "PARENT", "ID"))
			}
			queries := 0
			c.schemaQueryer = schemaQueryFunc(func(ctx context.Context, query string, args ...any) (*sqlx.Rows, error) {
				queries++
				return db.QueryxContext(ctx, query, args...)
			})
			details := c.tableConstraintsForPage(context.Background(), allowed, selected)
			require.NoError(t, dbMock.ExpectationsWereMet())
			require.NotNil(t, details[first])
			require.NotNil(t, details[last])
			require.Len(t, details[first].Constraints, 1)
			require.Len(t, details[last].Constraints, 1)
			require.Equal(t, "LAST_PK", details[last].Constraints[0].ReferencedConstraint)
			require.Empty(t, details[last].Constraints[0].ReferencedColumns)
			if errors.Is(failure, context.DeadlineExceeded) {
				require.Equal(t, 2, queries)
				require.ErrorIs(t, c.schemaContextError(context.Background()), failure)
				require.Equal(t, "FIRST_PK", details[first].Constraints[0].ReferencedConstraint)
			} else {
				require.Equal(t, 3, queries)
				require.NoError(t, c.schemaContextError(context.Background()))
				require.Equal(t, []string{"ID"}, details[first].Constraints[0].ReferencedColumns)
				require.Empty(t, details[first].Constraints[0].ReferencedConstraint)
			}
		})
	}
}

func TestSchemaReferencedConstraintColumnQueriesBoundNamesAndTables(t *testing.T) {
	for _, distinctNames := range []bool{false, true} {
		t.Run(fmt.Sprintf("distinct_names_%t", distinctNames), func(t *testing.T) {
			targets := make(map[constraintKey]tableKey)
			for i := 0; i < maxSchemaRelationsPerQuery+1; i++ {
				owner, name := fmt.Sprintf("OWNER_%04d", i), "SAME_NAME"
				if distinctNames {
					owner, name = "APP", fmt.Sprintf("PK_%04d", i)
				}
				targets[constraintKey{conID: 3, owner: owner, name: name}] = tableKey{conID: 3, owner: owner, table: "PARENT"}
			}
			queries := schemaReferencedConstraintColumnQueries(targets)
			require.Len(t, queries, 2)
			seenNames := make(map[string]int)
			for _, query := range queries {
				require.NotContains(t, query.text, "/*RELATIONS*/")
				require.NotContains(t, query.text, "/*CONSTRAINTS*/")
				require.Contains(t, query.text, "ORDER BY cc.con_id, cc.owner, cc.constraint_name, cc.position, cc.column_name")
				args := make(map[string]any)
				namesInQuery := 0
				for _, value := range query.args {
					arg, ok := value.(sql.NamedArg)
					require.True(t, ok)
					require.NotContains(t, args, arg.Name, "bind namespaces must not collide")
					args[arg.Name] = arg.Value
					if strings.HasPrefix(arg.Name, "targetConstraint") {
						namesInQuery++
						value := arg.Value.(sql.NullString)
						if value.Valid {
							seenNames[value.String]++
						}
					}
				}
				require.LessOrEqual(t, namesInQuery, maxSchemaRelationsPerQuery)
				for _, placeholder := range regexp.MustCompile(`:([A-Za-z0-9_]+)`).FindAllStringSubmatch(query.text, -1) {
					require.Contains(t, args, placeholder[1])
				}
			}
			if distinctNames {
				require.Len(t, seenNames, maxSchemaRelationsPerQuery+1)
				for _, count := range seenNames {
					require.Equal(t, 1, count, "a target name must not overlap name chunks")
				}
			} else {
				require.Equal(t, map[string]int{"SAME_NAME": 2}, seenNames)
			}
		})
	}
	require.Empty(t, schemaReferencedConstraintColumnQueries(nil))
}

func schemaColumnPages(tables, columns int) [][]schemaRowDB {
	var pages [][]schemaRowDB
	for table := range tables {
		if table%schemaRelationPageSize == 0 {
			pages = append(pages, nil)
		}
		for column := range columns {
			pages[len(pages)-1] = append(pages[len(pages)-1], schemaRowDB{
				ConID: 3, Owner: "APP", TableName: fmt.Sprintf("T%04d", table),
				ColumnName: fmt.Sprintf("C%04d", column), ColumnPresent: sql.NullInt64{Int64: 1, Valid: true},
			})
		}
	}
	return pages
}

func expectSchemaPageDetails(mock sqlmock.Sqlmock) {
	for _, view := range []string{"cdb_blockchain_tables", "cdb_immutable_tables", "cdb_mviews", "cdb_tab_comments", "cdb_indexes", "cdb_external_tables", "cdb_part_tables"} {
		mock.ExpectQuery("FROM " + view).WillReturnRows(sqlmock.NewRows([]string{"UNUSED"}))
	}
}

func TestSchemaWideTablesKeepColumnDetails(t *testing.T) {
	for _, version := range []string{"12.1.0.2.0", "19.0.0.0.0", "23.0.0.0.0"} {
		t.Run(version, func(t *testing.T) {
			c, _, mock, cleanup := newSchemaCheck(t)
			defer cleanup()
			c.dbVersion = version
			pages := schemaColumnPages(101, 84)
			mock.MatchExpectationsInOrder(false)
			for _, page := range pages {
				allowed := columnKeysFromRows(page)
				expectSchemaPageDetails(mock)
				filters := columnFilterChunks(allowed, relationColumnNames{conID: "con_id", owner: "owner", relation: "table_name"}, "column_name")
				last := page[len(page)-1]
				for i := range filters {
					rows := sqlmock.NewRows([]string{"CON_ID", "OWNER", "TABLE_NAME", "COLUMN_NAME", "COMMENTS"})
					if i == len(filters)-1 {
						rows.AddRow(last.ConID, last.Owner, last.TableName, last.ColumnName, "last column")
					}
					mock.ExpectQuery("FROM cdb_col_comments").WillReturnRows(rows)
				}
				if c.defaultValueColumn() == "c.data_default_vc" {
					for range schemaColumnDefaultQueries(allowed) {
						mock.ExpectQuery("FROM cdb_tab_cols").WillReturnRows(sqlmock.NewRows([]string{"UNUSED"}))
					}
				} else {
					expectSchemaSession(mock, 3, "APP_PDB")
					for range filters {
						mock.ExpectQuery("FROM dba_tab_cols").WillReturnRows(sqlmock.NewRows([]string{"UNUSED"}))
					}
				}
				details := c.tableDetailsForPage(context.Background(), tableKeysFromRows(page), allowed)
				key := tableKey{conID: last.ConID, owner: last.Owner, table: last.TableName}
				require.NotNil(t, details[key])
				require.Equal(t, "last column", details[key].ColumnComments[last.ColumnName])
			}
			require.NoError(t, mock.ExpectationsWereMet())
		})
	}
}

func TestSchemaColumnDetailsKeepPageMembershipAndValues(t *testing.T) {
	c, _, mock, cleanup := newSchemaCheck(t)
	defer cleanup()
	mock.MatchExpectationsInOrder(false)
	for _, page := range schemaColumnPages(101, 2) {
		key := tableKey{conID: 3, owner: "APP", table: page[0].TableName}
		expectSchemaPageDetails(mock)
		mock.ExpectQuery("FROM cdb_col_comments").WillReturnRows(sqlmock.NewRows([]string{"CON_ID", "OWNER", "TABLE_NAME", "COLUMN_NAME", "COMMENTS"}).
			AddRow(3, "APP", key.table, "C0000", "selected comment").
			AddRow(3, "APP", "GONE", "C0000", "wrong table").
			AddRow(4, "APP", key.table, "C0000", "wrong container").
			AddRow(3, "OTHER", key.table, "C0000", "wrong owner").
			AddRow(3, "APP", key.table, "UNSELECTED", "wrong column"))
		mock.ExpectQuery("FROM cdb_tab_cols").WillReturnRows(sqlmock.NewRows([]string{"CON_ID", "OWNER", "TABLE_NAME", "COLUMN_NAME", "DATA_DEFAULT"}).
			AddRow(3, "APP", key.table, "C0000", strings.Repeat("é", 4001)).
			AddRow(3, "APP", key.table, "C0001", nil).
			AddRow(4, "APP", key.table, "C0000", "wrong container").
			AddRow(3, "OTHER", key.table, "C0000", "wrong owner").
			AddRow(3, "APP", "GONE", "C0000", "wrong table").
			AddRow(3, "APP", key.table, "UNSELECTED", "wrong column"))
		details := c.tableDetailsForPage(context.Background(), tableKeysFromRows(page), columnKeysFromRows(page))
		require.Len(t, details, len(tableKeysFromRows(page)))
		require.NotContains(t, details, tableKey{conID: 3, owner: "APP", table: "GONE"})
		require.Equal(t, map[string]string{"C0000": "selected comment"}, details[key].ColumnComments)
		require.Equal(t, map[string]string{"C0000": strings.Repeat("é", 4000)}, details[key].ColumnDefaults)
		require.NoError(t, mock.ExpectationsWereMet())
	}
}

func TestSchemaHydrationProcessesOnePageBeforeQueryingNext(t *testing.T) {
	c, db, mock, cleanup := newSchemaCheck(t)
	defer cleanup()
	keys := schemaHydrationKeys(500, "APP", 3)
	expectSchemaHydrationRows(mock, keys, func(rows *sqlmock.Rows, i int, key tableKey) {
		if i == 1 {
			addTableWithoutColumns(rows, key.conID, key.owner, key.table)
		} else if i != 2 {
			addTableRow(rows, key.conID, key.owner, key.table)
		}
	})
	queried, processed, rowCount := 0, 0, 0
	c.schemaQueryer = schemaQueryFunc(func(ctx context.Context, query string, args ...any) (*sqlx.Rows, error) {
		require.Equal(t, processed, queried, "finish the previous page before querying another")
		require.Len(t, args, 203)
		require.NotContains(t, query, ":rel0name100")
		queried++
		return db.QueryxContext(ctx, query, args...)
	})
	err := c.hydrateTablePages(context.Background(), keys, 1000, func(rows []schemaRowDB) error {
		require.Equal(t, processed+1, queried)
		require.LessOrEqual(t, len(rows), schemaRelationPageSize)
		for _, row := range rows {
			require.NotEqual(t, keys[2].table, row.TableName)
			if row.TableName == keys[1].table {
				require.Equal(t, sql.NullInt64{Valid: true}, row.ColumnPresent)
			}
		}
		rowCount += len(rows)
		processed++
		return nil
	})
	require.NoError(t, err)
	require.Equal(t, 5, queried)
	require.Equal(t, 5, processed)
	require.Equal(t, 499, rowCount)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestSchemaHydrationStopsOnError(t *testing.T) {
	for _, scenario := range []string{"query", "consume", "cancel"} {
		t.Run(scenario, func(t *testing.T) {

			c, _, mock, cleanup := newSchemaCheck(t)
			defer cleanup()
			keys := schemaHydrationKeys(200, "APP", 3)
			rows := emptyTablesRows()
			addTableRow(rows, 3, "APP", keys[0].table)
			mock.ExpectQuery("WITH ranked_columns").WillReturnRows(rows)
			expected := errors.New("hydration failed")
			if scenario == "query" {
				mock.ExpectQuery("WITH ranked_columns").WillReturnError(expected)
			}
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			consumed := 0
			err := c.hydrateTablePages(ctx, keys, 1000, func([]schemaRowDB) error {
				consumed++
				if scenario == "consume" {
					return expected
				}
				if scenario == "cancel" {
					cancel()
				}
				return nil
			})
			if scenario == "cancel" {
				require.ErrorIs(t, err, context.Canceled)
			} else {
				require.ErrorIs(t, err, expected)
			}
			require.Equal(t, 1, consumed)
			require.NoError(t, mock.ExpectationsWereMet())
		})
	}
}

func TestSchemaColumnDetailsOptionalFailuresStayPageLocal(t *testing.T) {
	for _, scenario := range []string{"empty", "unavailable", "generic_failure", "partial_rows"} {
		t.Run(scenario, func(t *testing.T) {
			c, _, mock, cleanup := newSchemaCheck(t)
			defer cleanup()
			mock.MatchExpectationsInOrder(false)
			for _, page := range schemaColumnPages(101, 2) {
				key := tableKey{conID: 3, owner: "APP", table: page[0].TableName}
				expectSchemaPageDetails(mock)
				for _, family := range []string{"cdb_col_comments", "cdb_tab_cols"} {
					expect := mock.ExpectQuery("FROM " + family)
					switch scenario {
					case "empty":
						expect.WillReturnRows(sqlmock.NewRows([]string{"UNUSED"}))
					case "unavailable":
						expect.WillReturnError(errors.New(oracleErrorTableOrViewDoesNotExist))
					case "generic_failure":
						expect.WillReturnError(errors.New("optional metadata unavailable"))
					case "partial_rows":
						expect.WillReturnRows(sqlmock.NewRows([]string{"CON_ID", "OWNER", "TABLE_NAME", "COLUMN_NAME", "VALUE"}).
							AddRow(3, "APP", key.table, "C0000", "kept").
							AddRow(3, "APP", key.table, "C0001", "lost").RowError(1, errors.New("optional fetch failed")))
					}
				}
				details := c.tableDetailsForPage(context.Background(), tableKeysFromRows(page), columnKeysFromRows(page))
				if scenario == "partial_rows" {
					require.Equal(t, map[string]string{"C0000": "kept"}, details[key].ColumnComments)
					require.Equal(t, map[string]string{"C0000": "kept"}, details[key].ColumnDefaults)
				} else {
					require.Empty(t, details[key].ColumnComments)
					require.Empty(t, details[key].ColumnDefaults)
				}
				require.NoError(t, c.schemaContextError(context.Background()))
				require.NoError(t, mock.ExpectationsWereMet())
			}
		})
	}
}

func TestSchemaHydrationDoesNotResurrectMissingTables(t *testing.T) {
	c, _, mock, cleanup := newSchemaCheck(t)
	defer cleanup()
	keys := schemaHydrationKeys(102, "APP", 3)
	expectSchemaHydrationRows(mock, keys, func(rows *sqlmock.Rows, i int, key tableKey) {
		if i != 2 {
			addTableRow(rows, key.conID, key.owner, key.table)
		}
	})
	var events []schemaEvent
	coordinator := newSchemaSnapshotCoordinator(c.nextSnapshotID, func(payload []byte) {
		var event schemaEvent
		require.NoError(t, json.Unmarshal(payload, &event))
		events = append(events, event)
	})
	err := c.hydrateTablePages(context.Background(), keys, 1000, func(page []schemaRowDB) error {
		collector := newSchemaEventCollector(&c, coordinator.add, nil, map[ownerKey]string{{conID: 3, owner: "APP"}: "1"}, map[int64]string{3: "PDB"})
		for _, row := range page {
			collector.add(row)
		}
		collector.finish()
		return coordinator.err
	})
	require.NoError(t, err)
	require.NoError(t, coordinator.complete())
	require.Len(t, events, 2)
	require.Equal(t, 2, events[1].CollectionPayloadsCount)
	var names []string
	for _, event := range events {
		for _, table := range event.Metadata[0].Schemas[0].Tables {
			names = append(names, table.Name)
		}
	}
	require.Len(t, names, 101)
	require.NotContains(t, names, keys[2].table)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestSchemaColumnDetailsKeepHiddenIndexExpressionLookup(t *testing.T) {
	c, _, mock, cleanup := newSchemaCheck(t)
	defer cleanup()
	key := tableKey{conID: 3, owner: "APP", table: "T"}
	mock.MatchExpectationsInOrder(false)
	mock.ExpectQuery("FROM cdb_col_comments").WillReturnRows(sqlmock.NewRows([]string{"CON_ID", "OWNER", "TABLE_NAME", "COLUMN_NAME", "COMMENTS"}).
		AddRow(3, "APP", "T", "ID", "identifier"))
	mock.ExpectQuery("FROM cdb_tab_cols").WillReturnRows(sqlmock.NewRows([]string{"CON_ID", "OWNER", "TABLE_NAME", "COLUMN_NAME", "DATA_DEFAULT"}).
		AddRow(3, "APP", "T", "ID", "42"))
	for _, view := range []string{"cdb_blockchain_tables", "cdb_immutable_tables", "cdb_mviews", "cdb_tab_comments"} {
		mock.ExpectQuery("FROM " + view).WillReturnRows(sqlmock.NewRows([]string{"UNUSED"}))
	}
	mock.ExpectQuery("FROM cdb_indexes").WillReturnRows(sqlmock.NewRows([]string{"CON_ID", "TABLE_OWNER", "TABLE_NAME", "INDEX_OWNER", "INDEX_NAME", "UNIQUENESS", "INDEX_TYPE", "COLUMN_NAME"}).
		AddRow(3, "APP", "T", "APP", "FUNCTION_INDEX", "NONUNIQUE", "FUNCTION-BASED NORMAL", "SYS_NC00002$"))
	mock.ExpectQuery(`(?s)FROM cdb_tab_cols.*c.hidden_column = 'YES'`).WillReturnRows(sqlmock.NewRows([]string{"CON_ID", "OWNER", "TABLE_NAME", "COLUMN_NAME", "DATA_DEFAULT"}).
		AddRow(3, "APP", "T", "SYS_NC00002$", `UPPER("ID")`))
	for _, view := range []string{"cdb_external_tables", "cdb_part_tables"} {
		mock.ExpectQuery("FROM " + view).WillReturnRows(sqlmock.NewRows([]string{"UNUSED"}))
	}
	details := c.tableDetailsForPage(context.Background(), map[tableKey]struct{}{key: {}}, map[columnKey]struct{}{{tableKey: key, column: "ID"}: {}})
	require.Equal(t, "42", details[key].ColumnDefaults["ID"])
	require.Equal(t, "identifier", details[key].ColumnComments["ID"])
	require.Len(t, details[key].Indexes, 1)
	require.Equal(t, []indexKeyPart{{Expression: `UPPER("ID")`}}, details[key].Indexes[0].Columns)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestSchemaLegacyPagesKeepDefaultsAndIndexExpressions(t *testing.T) {
	for _, version := range []string{"12.1.0.2.0", "19.0.0.0.0", "21.3.0.0.0"} {
		t.Run(version, func(t *testing.T) {
			c, _, mock, cleanup := newSchemaCheck(t)
			defer cleanup()
			c.dbVersion = version
			for _, page := range schemaColumnPages(101, 2) {
				key := tableKey{conID: 3, owner: "APP", table: page[0].TableName}
				for _, view := range []string{"cdb_blockchain_tables", "cdb_immutable_tables", "cdb_mviews", "cdb_tab_comments"} {
					mock.ExpectQuery("FROM " + view).WillReturnRows(sqlmock.NewRows([]string{"UNUSED"}))
				}
				mock.ExpectQuery("FROM cdb_col_comments").WillReturnRows(sqlmock.NewRows([]string{"CON_ID", "OWNER", "TABLE_NAME", "COLUMN_NAME", "COMMENTS"}).
					AddRow(3, "APP", key.table, "C0000", "identifier"))
				mock.ExpectQuery("FROM cdb_indexes").WillReturnRows(sqlmock.NewRows([]string{"CON_ID", "TABLE_OWNER", "TABLE_NAME", "INDEX_OWNER", "INDEX_NAME", "UNIQUENESS", "INDEX_TYPE", "COLUMN_NAME"}).
					AddRow(3, "APP", key.table, "APP", "FUNCTION_INDEX", "NONUNIQUE", "FUNCTION-BASED NORMAL", "SYS_NC00002$"))
				expectSchemaSession(mock, 3, "APP_PDB")
				mock.ExpectQuery("FROM dba_tab_cols").WillReturnRows(sqlmock.NewRows([]string{"CON_ID", "OWNER", "TABLE_NAME", "COLUMN_NAME", "EXPRESSION_COLUMN", "DATA_DEFAULT"}).
					AddRow(3, "APP", key.table, "C0000", "NO", strings.Repeat("界", 4001)).
					AddRow(3, "APP", key.table, "SYS_NC00002$", "YES", "UPPER(\"C0000\")"))
				for _, view := range []string{"cdb_external_tables", "cdb_part_tables"} {
					mock.ExpectQuery("FROM " + view).WillReturnRows(sqlmock.NewRows([]string{"UNUSED"}))
				}
				details := c.tableDetailsForPage(context.Background(), tableKeysFromRows(page), columnKeysFromRows(page))
				require.Equal(t, "identifier", details[key].ColumnComments["C0000"])
				require.Equal(t, strings.Repeat("界", 4000), details[key].ColumnDefaults["C0000"])
				require.Len(t, details[key].Indexes, 1)
				require.Equal(t, []indexKeyPart{{Expression: "UPPER(\"C0000\")"}}, details[key].Indexes[0].Columns)
			}
			require.NoError(t, c.schemaContextError(context.Background()))
			require.NoError(t, mock.ExpectationsWereMet())
		})
	}
}

func TestSchemaColumnDetailsCancellationDoesNotPublishSnapshot(t *testing.T) {
	for _, family := range []string{"comments", "defaults"} {
		t.Run(family, func(t *testing.T) {
			c, _, mock, cleanup := newSchemaCheck(t)
			defer cleanup()
			views := false
			c.config.Schemas.CollectViews = &views
			c.config.Schemas.MaxTables = 101
			var events []schemaEvent
			c.schemaEmitter = func(payload []byte) {
				var event schemaEvent
				require.NoError(t, json.Unmarshal(payload, &event))
				events = append(events, event)
			}
			keys := schemaHydrationKeys(101, "APP", 3)
			mock.ExpectQuery(regexp.QuoteMeta(containerNamesQuery)).WillReturnRows(sqlmock.NewRows([]string{"CON_ID", "NAME"}).AddRow(3, "APP_PDB"))
			mock.ExpectQuery("cdb_users").WillReturnRows(sqlmock.NewRows([]string{"CON_ID", "USERNAME", "USER_ID"}).AddRow(3, "APP", 104))
			mock.ExpectQuery("SELECT con_id, owner, table_name").WillReturnRows(identityRows(keys...))
			mock.ExpectQuery("FROM cdb_constraints").WillReturnRows(emptyConstraintRows())
			expectSchemaHydrationRows(mock, keys[:100], nil)
			for _, view := range []string{"cdb_blockchain_tables", "cdb_immutable_tables", "cdb_mviews", "cdb_tab_comments"} {
				mock.ExpectQuery("FROM " + view).WillReturnRows(sqlmock.NewRows([]string{"UNUSED"}))
			}
			failure := error(context.Canceled)
			if family == "comments" {
				mock.ExpectQuery("FROM cdb_col_comments").WillReturnError(failure)
			} else {
				mock.ExpectQuery("FROM cdb_col_comments").WillReturnRows(sqlmock.NewRows([]string{"UNUSED"}))
				failure = context.DeadlineExceeded
				mock.ExpectQuery("FROM cdb_tab_cols").WillReturnError(failure)
			}
			require.ErrorIs(t, c.SchemaCollection(), failure)
			require.Empty(t, events)
			require.NoError(t, mock.ExpectationsWereMet())
		})
	}
}

func schemaHydrationKeys(count int, owner string, conID int64) []tableKey {
	keys := make([]tableKey, count)
	for i := range keys {
		keys[i] = tableKey{conID: conID, owner: owner, table: fmt.Sprintf("T%04d", i)}
	}
	return keys
}

func TestSchemaHydrationKeepsPageMembershipAndReturnedOrder(t *testing.T) {
	c, _, mock, cleanup := newSchemaCheck(t)
	defer cleanup()
	keys := schemaHydrationKeys(201, "APP", 3)
	keys[0].table, keys[1].table, keys[100].table = "Quoted'Name", "éclair", "Zebra"
	rows := emptyTablesRows()
	addTableWithoutColumns(rows, 3, "APP", "éclair")
	addTableRow(rows, 3, "APP", "Quoted'Name")
	addTableRow(rows, 3, "APP", "Quoted'Name")
	mock.ExpectQuery("WITH ranked_columns").WillReturnRows(rows).RowsWillBeClosed()
	mock.ExpectQuery("WITH ranked_columns").WillReturnRows(addTableRow(emptyTablesRows(), 3, "APP", "Zebra")).RowsWillBeClosed()
	mock.ExpectQuery("WITH ranked_columns").WillReturnRows(emptyTablesRows()).RowsWillBeClosed()
	var names [][]string
	var prior []schemaRowDB
	require.NoError(t, c.hydrateTablePages(context.Background(), keys, 50, func(page []schemaRowDB) error {
		for _, cleared := range prior {
			require.Equal(t, schemaRowDB{}, cleared)
		}
		var pageNames []string
		for _, row := range page {
			pageNames = append(pageNames, row.TableName)
			if row.TableName == "éclair" {
				require.True(t, row.ColumnPresent.Valid)
				require.Zero(t, row.ColumnPresent.Int64)
			}
		}
		names = append(names, pageNames)
		prior = page
		return nil
	}))
	require.Equal(t, [][]string{{"éclair", "Quoted'Name", "Quoted'Name"}, {"Zebra"}, nil}, names)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestSchemaHydrationRejectsInvalidPagesBeforeProcessing(t *testing.T) {
	for _, outcome := range []string{"query", "partial_rows", "unknown_table", "wrong_container", "wrong_owner", "later_page"} {
		t.Run(outcome, func(t *testing.T) {
			c, _, mock, cleanup := newSchemaCheck(t)
			defer cleanup()
			keys := schemaHydrationKeys(201, "APP", 3)
			query := mock.ExpectQuery("WITH ranked_columns")
			wanted := context.DeadlineExceeded
			switch outcome {
			case "query":
				query.WillReturnError(wanted)
			case "partial_rows":
				rows := addTableRow(emptyTablesRows(), 3, "APP", keys[0].table)
				addTableRow(rows, 3, "APP", keys[100].table).RowError(1, wanted)
				query.WillReturnRows(rows).RowsWillBeClosed()
			case "unknown_table":
				query.WillReturnRows(addTableRow(emptyTablesRows(), 3, "APP", "UNKNOWN")).RowsWillBeClosed()
			case "wrong_container":
				query.WillReturnRows(addTableRow(emptyTablesRows(), 4, "APP", keys[0].table)).RowsWillBeClosed()
			case "wrong_owner":
				query.WillReturnRows(addTableRow(emptyTablesRows(), 3, "OTHER", keys[0].table)).RowsWillBeClosed()
			case "later_page":
				query.WillReturnRows(addTableRow(emptyTablesRows(), 3, "APP", keys[100].table)).RowsWillBeClosed()
			}
			calls := 0
			err := c.hydrateTablePages(context.Background(), keys, 50, func([]schemaRowDB) error {
				calls++
				return nil
			})
			if outcome == "query" || outcome == "partial_rows" {
				require.ErrorIs(t, err, wanted)
			} else {
				require.ErrorContains(t, err, "unselected table")
			}
			require.Zero(t, calls)
			require.NoError(t, mock.ExpectationsWereMet())
		})
	}
}

func TestSchemaHydrationClearsRowsAfterProcessing(t *testing.T) {
	for _, outcome := range []string{"success", "error", "panic"} {
		t.Run(outcome, func(t *testing.T) {
			c, _, mock, cleanup := newSchemaCheck(t)
			defer cleanup()
			keys := schemaHydrationKeys(101, "APP", 3)
			expectSchemaHydrationRows(mock, keys[:100], nil)
			if outcome == "success" {
				expectSchemaHydrationRows(mock, keys[100:], nil)
			}
			wanted := errors.New("details failed")
			calls := 0
			var retained []schemaRowDB
			run := func() error {
				return c.hydrateTablePages(context.Background(), keys, 50, func(page []schemaRowDB) error {
					for _, row := range retained {
						require.Equal(t, schemaRowDB{}, row)
					}
					calls++
					retained = page
					if outcome == "panic" {
						panic(wanted)
					}
					if outcome == "error" {
						return wanted
					}
					return nil
				})
			}
			if outcome == "panic" {
				require.PanicsWithValue(t, wanted, func() { _ = run() })
			} else if outcome == "error" {
				require.ErrorIs(t, run(), wanted)
			} else {
				require.NoError(t, run())
			}
			expectedCalls := 1
			if outcome == "success" {
				expectedCalls = 2
			}
			require.Equal(t, expectedCalls, calls)
			for _, row := range retained {
				require.Equal(t, schemaRowDB{}, row)
			}
			require.NoError(t, mock.ExpectationsWereMet())
		})
	}
}

func TestSchemaHydrationKeepsOrderedPayloads(t *testing.T) {
	c, _, mock, cleanup := newSchemaCheck(t)
	defer cleanup()
	keys := schemaHydrationKeys(500, "APP", 3)
	expectSchemaHydrationRows(mock, keys, func(rows *sqlmock.Rows, _ int, key tableKey) {
		addTableWithoutColumns(rows, key.conID, key.owner, key.table)
	})
	collect := func(run func(func([]schemaRowDB) error) error) []schemaEvent {
		var events []schemaEvent
		coordinator := newSchemaSnapshotCoordinator(c.nextSnapshotID, func(payload []byte) {
			var event schemaEvent
			require.NoError(t, json.Unmarshal(payload, &event))
			event.Timestamp, event.CollectionStartedAt = 0, 0
			events = append(events, event)
		})
		require.NoError(t, run(func(page []schemaRowDB) error {
			collector := newSchemaEventCollector(&c, coordinator.add, nil, map[ownerKey]string{{conID: 3, owner: "APP"}: "1"}, map[int64]string{3: "PDB"})
			for _, row := range page {
				collector.add(row)
			}
			collector.finish()
			return coordinator.err
		}))
		require.NoError(t, coordinator.complete())
		return events
	}
	actual := collect(func(process func([]schemaRowDB) error) error {
		return c.hydrateTablePages(context.Background(), keys, 50, func(rows []schemaRowDB) error {
			return process(rows)
		})
	})
	expected := collect(func(process func([]schemaRowDB) error) error {
		return forEachTablePage(keys, schemaRelationPageSize, func(page []tableKey) error {
			var rows []schemaRowDB
			for _, key := range page {
				rows = append(rows, schemaRowDB{ConID: key.conID, Owner: key.owner, TableName: key.table,
					Temporary: "N", Duration: "-", External: "NO", IotType: "-", Partitioned: "NO", ClusterName: "-",
					Clustering: "NO", ReadOnly: "NO", ObjectTypeOwner: "-", ObjectType: "-"})
			}
			return process(rows)
		})
	})
	require.Equal(t, expected, actual)
	require.Len(t, actual, 5)
	for _, event := range actual {
		require.Len(t, event.Metadata[0].Schemas[0].Tables, 100)
		for _, table := range event.Metadata[0].Schemas[0].Tables {
			require.Empty(t, table.Columns)
		}
	}
	require.NoError(t, mock.ExpectationsWereMet())
}

func expectSchemaHydrationRows(mock sqlmock.Sqlmock, keys []tableKey, add func(*sqlmock.Rows, int, tableKey)) {
	for start := 0; start < len(keys); start += schemaRelationPageSize {
		rows := emptyTablesRows()
		for i := start; i < min(start+schemaRelationPageSize, len(keys)); i++ {
			key := keys[i]
			if add == nil {
				addTableRow(rows, key.conID, key.owner, key.table)
			} else {
				add(rows, i, key)
			}
		}
		mock.ExpectQuery("WITH ranked_columns").WillReturnRows(rows).RowsWillBeClosed()
	}
}

func (c *Check) tablePageRows(ctx context.Context, keys []tableKey, maxColumns int) ([]schemaRowDB, error) {
	var pageRows []schemaRowDB
	err := c.queryTablePageRows(ctx, keys, maxColumns, func(row schemaRowDB) error {
		pageRows = append(pageRows, row)
		return nil
	})
	if err != nil {
		return nil, err
	}
	return pageRows, nil
}

func (c *Check) tableDetailsForPage(ctx context.Context, allowed map[tableKey]struct{}, allowedColumns map[columnKey]struct{}) map[tableKey]*tableDetails {
	return c.tableDetailsForPageWithRelations(ctx, allowed, allowedColumns, allowed, allowed)
}
