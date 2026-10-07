// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build oracle

package oracle

import (
	"context"
	"database/sql/driver"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"regexp"
	"strings"
	"testing"
	"testing/iotest"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/godror/godror"
	"github.com/jmoiron/sqlx"
	"github.com/sijms/go-ora/v2/network"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/aggregator/mocksender"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/oracle/common"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/oracle/config"
)

func newDataObservabilityTestSender(t *testing.T, c *Check) *mocksender.MockSender {
	t.Helper()
	manager := mocksender.CreateDefaultDemultiplexer(t)
	require.NoError(t, c.CommonConfigure(manager, nil, nil, "file", "file"))
	sender := mocksender.NewMockSenderWithSenderManager(c.ID(), manager)
	sender.SetupAcceptAll()
	return sender
}

func TestDataObservabilityEventPayload(t *testing.T) {
	query := dataObservabilityTestQuery("orcl.SALES", `SELECT * FROM "APP"."ORDERS"`)
	query.MonitorID = 123
	query.Entity = &config.DataObservabilityEntity{Platform: "oracle", Account: "account", Database: "orcl.SALES", Schema: "APP", Table: "ORDERS"}
	metricID, entityID := int64(0), ""
	query.CustomSQLSelectFields = &config.DataObservabilityCustomSQLSelectFields{MetricConfigID: &metricID, EntityID: &entityID}
	c, dbMock := newDataObservabilityExecutionCheck(t, query)
	c.connectedToPdb = true
	c.dbHostname, c.config.Port = "db-host", 1521
	c.config.DataObservability.ConfigID = "remote-id"
	c.tags = []string{"env:test", "dd.internal.resource:test", "dd.internal.other"}
	sender := newDataObservabilityTestSender(t, c)
	var err error
	c.dataObservabilityEmitter, err = c.newDataObservabilityEmitter()
	require.NoError(t, err)
	c.tags[0] = "changed"
	c.dbHostname, c.config.Port, c.config.DataObservability.ConfigID = "changed", 999, "changed"
	expectDataObservabilityConnection(dbMock, "SALES")
	rows := sqlmock.NewRowsWithColumnDefinition(
		sqlmock.NewColumn("N").OfType("NUMBER", ""),
		sqlmock.NewColumn("TEXT").OfType("CLOB", ""),
		sqlmock.NewColumn("BYTES").OfType("BLOB", []byte{}),
		sqlmock.NewColumn("NULL").OfType("VARCHAR2", ""),
		sqlmock.NewColumn("DATE").OfType("DATE", time.Time{}),
	).AddRow("42.25", "value", []byte("café"), nil, time.Date(2026, 1, 2, 3, 4, 5, 0, time.FixedZone("server", 3600)))
	dbMock.ExpectQuery(regexp.QuoteMeta(query.Query)).WillReturnRows(rows).RowsWillBeClosed()
	require.NoError(t, c.collectDataObservability(context.Background()))
	var payload []byte
	for _, call := range sender.Calls {
		if call.Method == "EventPlatformEvent" {
			assert.Equal(t, "do-query-results", call.Arguments.String(1))
			payload = call.Arguments.Get(0).([]byte)
		} else if call.Method == "Count" || call.Method == "Gauge" {
			for _, tag := range call.Arguments.Get(3).([]string) {
				assert.NotContains(t, tag, "dd.internal")
				assert.NotEqual(t, "changed", tag)
			}
		}
	}
	require.NotEmpty(t, payload)
	assert.JSONEq(t, `{
		"timestamp":1767268800000,"config_id":"remote-id","db_type":"oracle","db_host":"db-host","db_port":1521,
		"db_name":"orcl.SALES","monitor_id":123,"query":"SELECT * FROM \"APP\".\"ORDERS\"",
		"entity":{"platform":"oracle","account":"account","database":"orcl.SALES","schema":"APP","table":"ORDERS"},
		"custom_sql_select_fields":{"metric_config_id":0,"entity_id":""},
		"status":"success","columns":["N","TEXT","BYTES","NULL","DATE"],
		"rows":[[42.25,"value","café",null,"2026-01-02T03:04:05"]],"row_count":1,"duration_s":0,"error":null
	}`, string(payload))
	sender.AssertMetric(t, "Count", "dd.oracle.data_observability.query_executions", 1, "db-host", []string{"env:test", "config_id:remote-id", "db_type:oracle", "monitor_id:123", "status:success"})
	sender.AssertNotCalled(t, "Commit")
}

func TestDataObservabilityEventOptionalMetadataAndQueryError(t *testing.T) {
	for _, tt := range []struct {
		name   string
		entity *config.DataObservabilityEntity
		fields *config.DataObservabilityCustomSQLSelectFields
		want   string
	}{
		{"absent", nil, nil, `{"entity":{},"custom_sql_select_fields":null}`},
		{"empty", &config.DataObservabilityEntity{}, &config.DataObservabilityCustomSQLSelectFields{}, `{"entity":{"platform":"","account":"","database":"","schema":"","table":""},"custom_sql_select_fields":{}}`},
	} {
		t.Run(tt.name, func(t *testing.T) {
			c, _ := newDataObservabilityWorkerCheck(t)
			sender := newDataObservabilityTestSender(t, c)
			emit, err := c.newDataObservabilityEmitter()
			require.NoError(t, err)
			message := "ORA-00942: table or view does not exist"
			due := dataObservabilityDueQuery{scheduledQuery: &dataObservabilityScheduledQuery{query: config.DataObservabilityQuery{Entity: tt.entity, CustomSQLSelectFields: tt.fields}}}
			emit(due, dataObservabilityQueryResult{Status: "error", Columns: []string{}, Rows: [][]any{}, Error: &message})
			for _, call := range sender.Calls {
				if call.Method != "EventPlatformEvent" {
					continue
				}
				var payload map[string]any
				require.NoError(t, json.Unmarshal(call.Arguments.Get(0).([]byte), &payload))
				assert.Equal(t, "", payload["config_id"])
				assert.Equal(t, "error", payload["status"])
				assert.Equal(t, message, payload["error"])
				assert.Equal(t, []any{}, payload["columns"])
				assert.Equal(t, []any{}, payload["rows"])
				assert.Equal(t, float64(0), payload["row_count"])
				metadata, err := json.Marshal(map[string]any{"entity": payload["entity"], "custom_sql_select_fields": payload["custom_sql_select_fields"]})
				require.NoError(t, err)
				assert.JSONEq(t, tt.want, string(metadata))
			}
			sender.AssertNumberOfCalls(t, "EventPlatformEvent", 1)
			sender.AssertMetric(t, "Count", "dd.oracle.data_observability.query_executions", 1, "", []string{"status:error"})
			sender.AssertMetricNotTaggedWith(t, "Count", "dd.oracle.data_observability.query_executions", []string{"config_id:"})
		})
	}
}

func TestNormalizeDataObservabilityValues(t *testing.T) {
	timestamp := time.Date(2026, 1, 2, 3, 4, 5, 123456000, time.FixedZone("offset", -5*3600))
	for _, tt := range []struct {
		name, databaseType string
		value, want        any
	}{
		{"null", "NUMBER", nil, nil},
		{"integer", "NUMBER", int64(42), int64(42)},
		{"decimal godror", "NUMBER", godror.Number("42.25"), 42.25},
		{"decimal go-ora", "NUMBER", "42.25", 42.25},
		{"numeric text", "VARCHAR2", "42.25", "42.25"},
		{"null varchar godror", "VARCHAR2", "", nil},
		{"null nvarchar godror", "NVARCHAR2", "", nil},
		{"null char godror", "CHAR", "", nil},
		{"null nchar godror", "NCHAR", "", nil},
		{"null long godror", "LONG", "", nil},
		{"null varchar go-ora", "NCHAR", nil, nil},
		{"whitespace", "CHAR", " ", " "},
		{"empty clob", "CLOB", "", ""},
		{"empty nclob", "NCLOB", "", ""},
		{"bytes", "RAW", []byte("café"), "café"},
		{"empty bytes", "RAW", []byte{}, ""},
		{"boolean", "BOOLEAN", true, true},
		{"nan", "BINARY_DOUBLE", math.NaN(), nil},
		{"infinity", "BINARY_DOUBLE", math.Inf(1), nil},
		{"negative infinity", "BINARY_FLOAT", float32(math.Inf(-1)), nil},
		{"date", "DATE", timestamp.Truncate(time.Second), "2026-01-02T03:04:05"},
		{"timestamp", "TIMESTAMP", timestamp, "2026-01-02T03:04:05.123456"},
		{"timestamp go-ora", "TimeStampDTY", timestamp, "2026-01-02T03:04:05.123456"},
		{"timezone godror", "TIMESTAMP WITH TIME ZONE", timestamp, "2026-01-02T03:04:05.123456-05:00"},
		{"timezone go-ora", "TimeStampTZ", timestamp, "2026-01-02T03:04:05.123456-05:00"},
		{"timezone go-ora dty", "TimeStampTZ_DTY", timestamp, "2026-01-02T03:04:05.123456-05:00"},
		{"local timezone godror", "TIMESTAMP WITH LOCAL TIME ZONE", timestamp, "2026-01-02T03:04:05.123456-05:00"},
		{"local timezone go-ora", "TimeStampeLTZ", timestamp, "2026-01-02T03:04:05.123456-05:00"},
		{"local timezone go-ora dty", "TimeStampLTZ_DTY", timestamp, "2026-01-02T03:04:05.123456-05:00"},
		{"utc offset", "TIMESTAMP WITH TIME ZONE", timestamp.UTC(), "2026-01-02T08:04:05.123456+00:00"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			got, err := normalizeDataObservabilityValue(tt.value, tt.databaseType)
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
	for _, value := range []any{[]byte{0xff}, godror.Number("invalid"), time.Second, struct{ Value string }{"object"}} {
		_, err := normalizeDataObservabilityValue(value, "")
		require.Error(t, err)
	}
}

type dataObservabilityLobConverter struct{}

func (dataObservabilityLobConverter) ConvertValue(value any) (driver.Value, error) {
	switch value.(type) {
	case godror.Option:
		return "lob_as_reader", nil
	case *godror.Lob:
		return value, nil
	default:
		return driver.DefaultParameterConverter.ConvertValue(value)
	}
}

type dataObservabilityLobReader struct {
	io.Reader
	closed   bool
	closeErr error
}

func (r *dataObservabilityLobReader) Close() error {
	r.closed = true
	return r.closeErr
}

func TestDataObservabilityLobFetching(t *testing.T) {
	readErr := errors.New("LOB fetch failed")
	closeErr := errors.New("LOB close failed")
	for _, tt := range []struct {
		name, columnType string
		reader           io.Reader
		closeErr         error
		want             any
		wantErr          error
	}{
		{"clob", "CLOB", strings.NewReader("café"), nil, "café", nil},
		{"nclob", "NCLOB", strings.NewReader("日本語"), nil, "日本語", nil},
		{"empty clob", "CLOB", strings.NewReader(""), nil, "", nil},
		{"empty nclob", "NCLOB", strings.NewReader(""), nil, "", nil},
		{"blob", "BLOB", strings.NewReader("\xff"), nil, []byte{0xff}, nil},
		{"empty blob", "BLOB", strings.NewReader(""), nil, []byte{}, nil},
		{"read failure", "CLOB", iotest.ErrReader(readErr), nil, nil, readErr},
		{"close failure", "CLOB", strings.NewReader("value"), closeErr, nil, closeErr},
	} {
		t.Run(tt.name, func(t *testing.T) {
			c, _ := newDataObservabilityWorkerCheck(t)
			db, dbMock, err := sqlmock.New(sqlmock.MonitorPingsOption(true), sqlmock.ValueConverterOption(dataObservabilityLobConverter{}))
			require.NoError(t, err)
			c.dataObservabilityDB = sqlx.NewDb(db, common.Godror)
			c.cdbName, c.connectedToPdb = "orcl", true
			t.Cleanup(func() {
				dbMock.ExpectClose()
				c.closeDataObservabilityConnection()
				assert.NoError(t, dbMock.ExpectationsWereMet())
			})
			expectDataObservabilityConnection(dbMock, "SALES")
			require.NoError(t, c.connectDataObservability(context.Background()))
			query := dataObservabilityTestQuery("orcl.SALES", "SELECT LOB_VALUE FROM T")
			reader := &dataObservabilityLobReader{Reader: tt.reader, closeErr: tt.closeErr}
			lob := &godror.Lob{Reader: reader, IsClob: tt.columnType != "BLOB"}
			rows := dbMock.NewRowsWithColumnDefinition(sqlmock.NewColumn("LOB_VALUE").OfType(tt.columnType, "")).AddRow(nil).AddRow(lob)
			dbMock.ExpectQuery(query.Query).WithArgs("lob_as_reader").WillReturnRows(rows).RowsWillBeClosed()
			result, err := c.executeDataObservabilityQuery(context.Background(), query, "SALES")
			if tt.wantErr != nil {
				require.ErrorIs(t, err, tt.wantErr)
				assert.Equal(t, "error", result.Status)
				assert.Empty(t, result.Rows)
			} else {
				require.NoError(t, err)
				assert.Equal(t, "success", result.Status)
				assert.Equal(t, [][]any{{nil}, {tt.want}}, result.Rows)
			}
			assert.True(t, reader.closed)
		})
	}
}

func TestDataObservabilityEmissionFailureCompletesAndContinues(t *testing.T) {
	queries := []config.DataObservabilityQuery{
		dataObservabilityTestQuery("orcl.SALES", "SELECT BAD_BYTES FROM T"),
		dataObservabilityTestQuery("orcl.SALES", "SELECT 1 FROM dual"),
	}
	queries[0].MonitorID, queries[1].MonitorID = 1, 2
	c, dbMock := newDataObservabilityExecutionCheck(t, queries...)
	c.connectedToPdb = true
	clk := c.clock.(*dataObservabilityWorkerTestClock)
	start := clk.Now()
	for _, entry := range c.dataObservabilityQueries {
		entry.lastExecution = start.Add(-90 * time.Second)
	}
	sender := newDataObservabilityTestSender(t, c)
	emit, err := c.newDataObservabilityEmitter()
	require.NoError(t, err)
	c.dataObservabilityEmitter = func(due dataObservabilityDueQuery, result dataObservabilityQueryResult) {
		assert.Equal(t, clk.Now(), due.scheduledQuery.lastExecution)
		clk.Add(2 * time.Second)
		emit(due, result)
	}
	expectDataObservabilityConnection(dbMock, "SALES")
	dbMock.ExpectQuery(queries[0].Query).WillReturnRows(sqlmock.NewRows([]string{"BAD_BYTES"}).AddRow([]byte{0xff}))
	dbMock.ExpectQuery(queries[1].Query).WillReturnRows(sqlmock.NewRows([]string{"N"}).AddRow(1))
	require.NoError(t, c.collectDataObservability(context.Background()))
	require.NoError(t, c.collectDataObservability(context.Background()))
	sender.AssertNumberOfCalls(t, "EventPlatformEvent", 1)
	sender.AssertMetric(t, "Count", "dd.oracle.data_observability.emit_failures", 1, "", []string{"monitor_id:1", "exc_class:UnsupportedValueError"})
	sender.AssertMetric(t, "Count", "dd.oracle.data_observability.query_executions", 1, "", []string{"monitor_id:1", "status:success"})
	sender.AssertMetric(t, "Gauge", "dd.oracle.data_observability.query_fire_lateness_seconds", 30, "", []string{"monitor_id:1", "mode:interval"})
	sender.AssertMetric(t, "Gauge", "dd.oracle.data_observability.query_fire_lateness_seconds", 32, "", []string{"monitor_id:2", "mode:interval"})
	sender.AssertMetric(t, "Gauge", "dd.oracle.data_observability.query_execution_time", 0, "", []string{"monitor_id:2"})
	assert.Equal(t, start, c.dataObservabilityQueries[0].lastExecution)
	assert.Equal(t, start.Add(2*time.Second), c.dataObservabilityQueries[1].lastExecution)
}

func TestDataObservabilityCronLateness(t *testing.T) {
	for _, delay := range []time.Duration{-time.Second, 25 * time.Second} {
		t.Run(delay.String(), func(t *testing.T) {
			c, _ := newDataObservabilityWorkerCheck(t)
			sender := newDataObservabilityTestSender(t, c)
			emit, err := c.newDataObservabilityEmitter()
			require.NoError(t, err)
			queries := newDataObservabilityScheduledQueries([]config.DataObservabilityQuery{{Schedule: "* * * * *"}})
			due := dataObservabilityDueQuery{scheduledQuery: queries[0], scheduledTime: c.clock.Now().Add(-delay)}
			emit(due, dataObservabilityQueryResult{Status: "success", startedAt: c.clock.Now(), Duration: 5})
			sender.AssertMetric(t, "Gauge", "dd.oracle.data_observability.query_fire_lateness_seconds", max(0, delay.Seconds()), "", []string{"mode:cron"})
			sender.AssertMetric(t, "Gauge", "dd.oracle.data_observability.query_execution_time", 5, "", []string{"monitor_id:0"})
		})
	}
}

func newDataObservabilityExecutionCheck(t *testing.T, queries ...config.DataObservabilityQuery) (*Check, sqlmock.Sqlmock) {
	t.Helper()
	c, _ := newDataObservabilityWorkerCheck(t)
	db, mock, err := sqlmock.New(sqlmock.MonitorPingsOption(true))
	require.NoError(t, err)
	c.dataObservabilityDB = sqlx.NewDb(db, "sqlmock")
	c.dataObservabilityDB.SetMaxOpenConns(1)
	c.cdbName = "orcl"
	c.multitenant = true
	c.config.DataObservability.Queries = queries
	c.dataObservabilityQueries = newDataObservabilityScheduledQueries(queries)
	t.Cleanup(func() {
		if c.dataObservabilityDB != nil {
			mock.ExpectClose()
		}
		c.Cancel()
		c.closeDataObservabilityConnection()
		assert.NoError(t, mock.ExpectationsWereMet())
	})
	return c, mock
}

func dataObservabilityTestQuery(dbname, query string) config.DataObservabilityQuery {
	return config.DataObservabilityQuery{
		DBName: dbname, Query: query, IntervalSeconds: 60, QueryTimeout: 1000,
		Entity: &config.DataObservabilityEntity{Schema: "METADATA_ONLY"},
	}
}

type dataObservabilityTestOracleError int

func (e dataObservabilityTestOracleError) Code() int { return int(e) }

func (e dataObservabilityTestOracleError) Error() string {
	return fmt.Sprintf("ORA-%05d", e.Code())
}

func expectDataObservabilityConnection(mock sqlmock.Sqlmock, container string) {
	mock.ExpectPing()
	mock.ExpectQuery(regexp.QuoteMeta(dataObservabilityCurrentContainerQuery)).
		WillReturnRows(sqlmock.NewRows([]string{"CON_NAME"}).AddRow(container)).RowsWillBeClosed()
}

func expectDataObservabilityContainers(mock sqlmock.Sqlmock, containers ...string) {
	mock.ExpectExec(regexp.QuoteMeta(`ALTER SESSION SET CONTAINER = "CDB$ROOT"`)).WillReturnResult(sqlmock.NewResult(0, 0))
	rows := sqlmock.NewRows([]string{"NAME"})
	for _, container := range containers {
		rows.AddRow(container)
	}
	mock.ExpectQuery(regexp.QuoteMeta(dataObservabilityContainersQuery)).WillReturnRows(rows).RowsWillBeClosed()
}

func TestDataObservabilityRoutesCatalogIdentities(t *testing.T) {
	queries := []config.DataObservabilityQuery{
		dataObservabilityTestQuery(`orcl.SALES.West"1`, `SELECT COUNT(*) FROM "APP"."ORDERS"`),
		dataObservabilityTestQuery("orcl.CDB$ROOT", "SELECT 1 FROM dual"),
	}
	c, mock := newDataObservabilityExecutionCheck(t, queries...)
	expectDataObservabilityConnection(mock, "CDB$ROOT")
	expectDataObservabilityContainers(mock, "CDB$ROOT", `SALES.West"1`)
	mock.ExpectExec(regexp.QuoteMeta(`ALTER SESSION SET CONTAINER = "CDB$ROOT"`)).WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectQuery(regexp.QuoteMeta(queries[1].Query)).WillReturnRows(sqlmock.NewRows([]string{"N"}).AddRow(1)).RowsWillBeClosed()
	mock.ExpectExec(regexp.QuoteMeta(`ALTER SESSION SET CONTAINER = "SALES.West""1"`)).WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectQuery(regexp.QuoteMeta(queries[0].Query)).WillReturnRows(sqlmock.NewRows([]string{"COUNT(*)"}).AddRow(42)).RowsWillBeClosed()
	var results []dataObservabilityQueryResult
	c.dataObservabilityEmitter = func(due dataObservabilityDueQuery, result dataObservabilityQueryResult) {
		assert.False(t, due.scheduledQuery.lastExecution.IsZero())
		results = append(results, result)
	}
	require.NoError(t, c.collectDataObservability(context.Background()))
	require.Len(t, results, 2)
	assert.Equal(t, [][]any{{int64(42)}}, results[1].Rows)
	assert.Equal(t, "success", results[1].Status)
	assert.Equal(t, 1, results[1].RowCount)
	assert.Nil(t, results[1].Error)
	require.NoError(t, c.collectDataObservability(context.Background()), "no due queries must not touch the connection")
}

func TestDataObservabilityDirectPDBRejectsOtherTargets(t *testing.T) {
	queries := []config.DataObservabilityQuery{
		dataObservabilityTestQuery("OTHER", "SELECT 0 FROM dual"),
		dataObservabilityTestQuery("orcl.OTHER", "SELECT 2 FROM dual"),
		dataObservabilityTestQuery("orcl.SALES", "SELECT 1 FROM dual"),
		dataObservabilityTestQuery("wrong.SALES", "SELECT 3 FROM dual"),
	}
	c, mock := newDataObservabilityExecutionCheck(t, queries...)
	c.connectedToPdb = true
	expectDataObservabilityConnection(mock, "SALES")
	mock.ExpectQuery(regexp.QuoteMeta(queries[2].Query)).WillReturnRows(sqlmock.NewRows([]string{"N"}).AddRow(1)).RowsWillBeClosed()
	results := make(map[string]dataObservabilityQueryResult)
	c.dataObservabilityEmitter = func(due dataObservabilityDueQuery, result dataObservabilityQueryResult) {
		results[due.scheduledQuery.query.DBName] = result
	}
	require.NoError(t, c.collectDataObservability(context.Background()))
	assert.Equal(t, "success", results["orcl.SALES"].Status)
	for _, dbname := range []string{"OTHER", "orcl.OTHER", "wrong.SALES"} {
		result := results[dbname]
		require.NotNil(t, result.Error)
		assert.Contains(t, *result.Error, "not reachable")
		assert.Empty(t, result.Rows)
	}
}

func TestDataObservabilityQueryResults(t *testing.T) {
	for _, tt := range []struct {
		name    string
		rows    *sqlmock.Rows
		failure error
		wantErr string
		wantLen int
	}{
		{"empty", sqlmock.NewRows([]string{"N"}), nil, "", 0},
		{"values", sqlmock.NewRows([]string{"N", "TEXT", "BYTES", "NULL", "DATE"}).AddRow(42, "value", []byte{1, 2}, nil, time.Unix(0, 0)), nil, "", 1},
		{"no result set", sqlmock.NewRows(nil), nil, "only SELECT statements are supported", 0},
		{"query error", nil, errors.New("ORA-00942: table or view does not exist"), "ORA-00942", 0},
		{"partial fetch error", sqlmock.NewRows([]string{"N"}).AddRow(1).AddRow(2).RowError(1, errors.New("fetch failed")), nil, "fetch failed", 0},
	} {
		t.Run(tt.name, func(t *testing.T) {
			query := dataObservabilityTestQuery("orcl.SALES", "SELECT * FROM T")
			if tt.name == "no result set" {
				query.Query = "UPDATE T SET N = 1"
			}
			c, mock := newDataObservabilityExecutionCheck(t, query)
			c.connectedToPdb = true
			expectDataObservabilityConnection(mock, "SALES")
			require.NoError(t, c.connectDataObservability(context.Background()))
			expect := mock.ExpectQuery(regexp.QuoteMeta(query.Query))
			if tt.failure != nil {
				expect.WillReturnError(tt.failure)
			} else {
				expect.WillReturnRows(tt.rows).RowsWillBeClosed()
			}
			result, err := c.executeDataObservabilityQuery(context.Background(), query, "SALES")
			if tt.wantErr != "" {
				require.ErrorContains(t, err, tt.wantErr)
				assert.Equal(t, "error", result.Status)
				require.NotNil(t, result.Error)
				assert.Contains(t, *result.Error, tt.wantErr)
				assert.Empty(t, result.Columns)
			} else {
				require.NoError(t, err)
				assert.Equal(t, "success", result.Status)
				assert.Nil(t, result.Error)
			}
			assert.Len(t, result.Rows, tt.wantLen)
			assert.Equal(t, tt.wantLen, result.RowCount)
			assert.NotNil(t, result.Rows)
			if tt.name == "values" {
				assert.Equal(t, []any{int64(42), "value", []byte{1, 2}, nil, time.Unix(0, 0)}, result.Rows[0])
			}
		})
	}
}

func TestDataObservabilityResultRowLimit(t *testing.T) {
	query := dataObservabilityTestQuery("orcl.SALES", "SELECT N FROM T")
	c, mock := newDataObservabilityExecutionCheck(t, query)
	c.connectedToPdb = true
	expectDataObservabilityConnection(mock, "SALES")
	require.NoError(t, c.connectDataObservability(context.Background()))
	rows := sqlmock.NewRows([]string{"N"})
	for i := 0; i <= dataObservabilityMaxResultRows; i++ {
		rows.AddRow(i)
	}
	rows.RowError(dataObservabilityMaxResultRows, errors.New("read beyond the row limit"))
	mock.ExpectQuery(regexp.QuoteMeta(query.Query)).WillReturnRows(rows).RowsWillBeClosed()
	result, err := c.executeDataObservabilityQuery(context.Background(), query, "SALES")
	require.NoError(t, err)
	assert.Equal(t, dataObservabilityMaxResultRows, result.RowCount)
	assert.Len(t, result.Rows, dataObservabilityMaxResultRows)
}

func TestDataObservabilityQueryErrorCompletesAndContinues(t *testing.T) {
	for _, tt := range []struct {
		name    string
		failure error
	}{
		{"message", errors.New("ORA-00904: invalid identifier")},
		{"godror", dataObservabilityTestOracleError(904)},
		{"go-ora", network.NewOracleError(904)},
	} {
		t.Run(tt.name, func(t *testing.T) {
			queries := []config.DataObservabilityQuery{
				dataObservabilityTestQuery("orcl.SALES", "SELECT INVALID FROM dual"),
				dataObservabilityTestQuery("orcl.SALES", "SELECT 1 FROM dual"),
			}
			c, mock := newDataObservabilityExecutionCheck(t, queries...)
			c.connectedToPdb = true
			expectDataObservabilityConnection(mock, "SALES")
			mock.ExpectQuery(regexp.QuoteMeta(queries[0].Query)).WillReturnError(tt.failure)
			mock.ExpectQuery(regexp.QuoteMeta(queries[1].Query)).WillReturnRows(sqlmock.NewRows([]string{"N"}).AddRow(1))
			var statuses []string
			c.dataObservabilityEmitter = func(_ dataObservabilityDueQuery, result dataObservabilityQueryResult) {
				statuses = append(statuses, result.Status)
			}
			require.NoError(t, c.collectDataObservability(context.Background()))
			assert.Equal(t, []string{"error", "success"}, statuses)
			assert.Empty(t, getDueDataObservabilityQueries(c.dataObservabilityQueries, c.clock.Now()))
		})
	}
}

func TestDataObservabilityConnectionFailureRetriesUnfinishedQueries(t *testing.T) {
	for _, schedule := range []string{"", "0 * * * *"} {
		for _, tt := range []struct {
			name    string
			stage   string
			failure error
		}{
			{"query", "query", errors.New("ORA-01012: not logged on")},
			{"godror fetch 03113", "fetch", fmt.Errorf("Next: %w", dataObservabilityTestOracleError(3113))},
			{"godror fetch 03114", "fetch", fmt.Errorf("Next: %w", dataObservabilityTestOracleError(3114))},
			{"godror fetch 03135", "fetch", fmt.Errorf("Next: %w", dataObservabilityTestOracleError(3135))},
			{"go-ora container 03113", "container", network.NewOracleError(3113)},
			{"go-ora container 03114", "container", network.NewOracleError(3114)},
			{"go-ora container 03135", "container", network.NewOracleError(3135)},
		} {
			t.Run(tt.name+"/"+schedule, func(t *testing.T) {
				queries := []config.DataObservabilityQuery{
					dataObservabilityTestQuery("orcl.SALES", "SELECT 1 FROM dual"),
					dataObservabilityTestQuery("orcl.SALES", "SELECT 2 FROM dual"),
					dataObservabilityTestQuery("orcl.SALES", "SELECT 3 FROM dual"),
				}
				for i := range queries {
					queries[i].Schedule = schedule
				}
				c, mock := newDataObservabilityExecutionCheck(t, queries...)
				c.connectedToPdb = tt.stage != "container"
				if c.connectedToPdb {
					expectDataObservabilityConnection(mock, "SALES")
				} else {
					expectDataObservabilityConnection(mock, "CDB$ROOT")
					expectDataObservabilityContainers(mock, "CDB$ROOT", "SALES")
					mock.ExpectExec(regexp.QuoteMeta(`ALTER SESSION SET CONTAINER = "SALES"`)).WillReturnResult(sqlmock.NewResult(0, 0))
				}
				mock.ExpectQuery(regexp.QuoteMeta(queries[0].Query)).WillReturnRows(sqlmock.NewRows([]string{"N"}).AddRow(1))
				switch tt.stage {
				case "query":
					mock.ExpectQuery(regexp.QuoteMeta(queries[1].Query)).WillReturnError(tt.failure)
				case "fetch":
					mock.ExpectQuery(regexp.QuoteMeta(queries[1].Query)).WillReturnRows(
						sqlmock.NewRows([]string{"N"}).AddRow(1).AddRow(2).RowError(1, tt.failure)).RowsWillBeClosed()
				case "container":
					mock.ExpectExec(regexp.QuoteMeta(`ALTER SESSION SET CONTAINER = "SALES"`)).WillReturnError(tt.failure)
				}
				mock.ExpectClose()
				var results []dataObservabilityQueryResult
				c.dataObservabilityEmitter = func(_ dataObservabilityDueQuery, result dataObservabilityQueryResult) {
					results = append(results, result)
				}
				require.ErrorIs(t, c.collectDataObservability(context.Background()), tt.failure)
				assert.Nil(t, c.dataObservabilityConn)
				assert.Nil(t, c.dataObservabilityDB)
				require.Len(t, results, 1)
				assert.Equal(t, "success", results[0].Status)
				assert.Nil(t, c.dataObservabilityQueries[0].pendingRetry)
				for _, pending := range c.dataObservabilityQueries[1:] {
					require.NotNil(t, pending.pendingRetry)
					assert.Equal(t, c.clock.Now(), pending.pendingRetry.scheduledTime)
				}
				due := getDueDataObservabilityQueries(c.dataObservabilityQueries, c.clock.Now().Add(time.Second))
				require.Len(t, due, 2)
				assert.Equal(t, queries[1].Query, due[0].scheduledQuery.query.Query)
				assert.Equal(t, queries[2].Query, due[1].scheduledQuery.query.Query)
			})
		}
	}
}

func TestDataObservabilityConnectionSetupFailureRetriesAllQueries(t *testing.T) {
	query := dataObservabilityTestQuery("orcl.SALES", "SELECT 1 FROM dual")
	query.Schedule = "0 * * * *"
	c, mock := newDataObservabilityExecutionCheck(t, query, query)
	mock.ExpectPing().WillReturnError(driver.ErrBadConn)
	mock.ExpectClose()
	require.Error(t, c.collectDataObservability(context.Background()))
	assert.Nil(t, c.dataObservabilityDB)
	assert.Len(t, getDueDataObservabilityQueries(c.dataObservabilityQueries, c.clock.Now().Add(time.Second)), 2)
}

func TestDataObservabilityQueryDeadline(t *testing.T) {
	query := dataObservabilityTestQuery("orcl.SALES", "SELECT 1 FROM dual")
	query.QueryTimeout = 1
	c, mock := newDataObservabilityExecutionCheck(t, query)
	c.connectedToPdb = true
	expectDataObservabilityConnection(mock, "SALES")
	mock.ExpectQuery(regexp.QuoteMeta(query.Query)).WillDelayFor(time.Hour).
		WillReturnRows(sqlmock.NewRows([]string{"N"}).AddRow(1))
	mock.ExpectClose()
	var result dataObservabilityQueryResult
	c.dataObservabilityEmitter = func(_ dataObservabilityDueQuery, r dataObservabilityQueryResult) { result = r }
	require.NoError(t, c.collectDataObservability(context.Background()))
	assert.Equal(t, "error", result.Status)
	require.NotNil(t, result.Error)
	assert.Contains(t, *result.Error, "context deadline exceeded")
	assert.Empty(t, getDueDataObservabilityQueries(c.dataObservabilityQueries, c.clock.Now()))
	assert.Nil(t, c.dataObservabilityConn)
}

func TestDataObservabilityCanceledBeforeSQL(t *testing.T) {
	query := dataObservabilityTestQuery("orcl.SALES", "SELECT 1 FROM dual")
	c, _ := newDataObservabilityExecutionCheck(t, query)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := c.executeDataObservabilityQuery(ctx, query, "SALES")
	require.ErrorIs(t, err, context.Canceled)
}

func TestDataObservabilityIntervalSchedules(t *testing.T) {
	queries := newDataObservabilityScheduledQueries([]config.DataObservabilityQuery{
		{MonitorID: 0, IntervalSeconds: 30},
		{MonitorID: 0, IntervalSeconds: 60},
	})
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	due := getDueDataObservabilityQueries(queries, now)
	require.Len(t, due, 2)
	assert.Equal(t, now, due[0].scheduledTime)
	assert.Equal(t, now, due[1].scheduledTime)
	due[0].complete(now.Add(5 * time.Second))
	due[1].complete(now.Add(10 * time.Second))

	due = getDueDataObservabilityQueries(queries, now.Add(30*time.Second))
	assert.Empty(t, due)
	due = getDueDataObservabilityQueries(queries, now.Add(35*time.Second))
	require.Len(t, due, 1)
	assert.Same(t, queries[0], due[0].scheduledQuery)
	assert.Equal(t, now.Add(35*time.Second), due[0].scheduledTime)
	due[0].complete(now.Add(40 * time.Second))

	due = getDueDataObservabilityQueries(queries, now.Add(70*time.Second))
	require.Len(t, due, 2)
	for _, query := range due {
		assert.Equal(t, now.Add(70*time.Second), query.scheduledTime)
	}
}

func TestDataObservabilityCronStartup(t *testing.T) {
	now := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	for _, tt := range []struct {
		name  string
		delay time.Duration
		want  bool
	}{
		{"on boundary", 0, true},
		{"within lookback", 299 * time.Second, true},
		{"at lookback minus offset", 300*time.Second - time.Millisecond, true},
		{"outside lookback with offset", 300 * time.Second, false},
		{"outside lookback", 301 * time.Second, false},
	} {
		t.Run(tt.name, func(t *testing.T) {
			queries := newDataObservabilityScheduledQueries([]config.DataObservabilityQuery{{Schedule: "0 * * * *", IntervalSeconds: 1}})
			due := getDueDataObservabilityQueries(queries, now.Add(tt.delay))
			if tt.want {
				require.Len(t, due, 1)
				assert.Equal(t, now, due[0].scheduledTime)
				due[0].complete(now.Add(tt.delay))
			} else {
				assert.Empty(t, due)
			}
			due = getDueDataObservabilityQueries(queries, now.Add(tt.delay+time.Second))
			assert.Empty(t, due)
		})
	}
}

func TestDataObservabilityCronCollapsesMissedTicks(t *testing.T) {
	queries := newDataObservabilityScheduledQueries([]config.DataObservabilityQuery{{Schedule: "* * * * *"}})
	now := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	due := getDueDataObservabilityQueries(queries, now)
	require.Len(t, due, 1)
	due[0].complete(now)
	due = getDueDataObservabilityQueries(queries, now.Add(5*time.Minute+20*time.Second))
	require.Len(t, due, 1)
	assert.Equal(t, now.Add(5*time.Minute), due[0].scheduledTime)
	due = getDueDataObservabilityQueries(queries, now.Add(5*time.Minute+30*time.Second))
	assert.Empty(t, due)
}

func TestDataObservabilityCronUsesUTC(t *testing.T) {
	queries := newDataObservabilityScheduledQueries([]config.DataObservabilityQuery{{Schedule: "0 9 * * *"}})
	now := time.Date(2026, 1, 1, 4, 0, 0, 0, time.FixedZone("EST", -5*60*60))
	due := getDueDataObservabilityQueries(queries, now)
	require.Len(t, due, 1)
	assert.Equal(t, time.Date(2026, 1, 1, 9, 0, 0, 0, time.UTC), due[0].scheduledTime)
	assert.Empty(t, getDueDataObservabilityQueries(queries, now.Add(time.Second)))
}

func TestDataObservabilityCronNoNextTick(t *testing.T) {
	for _, tt := range []struct {
		schedule string
		wantDue  int
	}{
		{"0 0 31 2 *", 0},
		{"0 0 29 2 *", 1},
	} {
		t.Run(tt.schedule, func(t *testing.T) {
			queries := newDataObservabilityScheduledQueries([]config.DataObservabilityQuery{{Schedule: tt.schedule}})
			now := time.Date(2096, 2, 29, 0, 0, 10, 0, time.UTC)
			due := getDueDataObservabilityQueries(queries, now)
			require.Len(t, due, tt.wantDue)
			assert.True(t, queries[0].nextTick.IsZero())
			assert.Empty(t, getDueDataObservabilityQueries(queries, now.Add(time.Second)))
		})
	}
}

func TestDataObservabilityRetries(t *testing.T) {
	queries := newDataObservabilityScheduledQueries([]config.DataObservabilityQuery{
		{MonitorID: 42, Schedule: "* * * * *"},
		{MonitorID: 42, Schedule: "0 * * * *"},
		{MonitorID: 42, IntervalSeconds: 60},
	})
	now := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	due := getDueDataObservabilityQueries(queries, now)
	require.Len(t, due, 3)
	for _, query := range due {
		query.retry()
	}
	due = getDueDataObservabilityQueries(queries, now.Add(10*time.Second))
	require.Len(t, due, 3)
	assert.Equal(t, now, due[0].scheduledTime)
	assert.Equal(t, now, due[1].scheduledTime)
	assert.Equal(t, now.Add(10*time.Second), due[2].scheduledTime)
	for _, query := range due {
		query.retry()
	}
	due = getDueDataObservabilityQueries(queries, now.Add(time.Minute))
	require.Len(t, due, 3)
	assert.Equal(t, now.Add(time.Minute), due[0].scheduledTime)
	assert.Equal(t, now, due[1].scheduledTime)
	assert.Equal(t, now.Add(time.Minute), due[2].scheduledTime)
	for _, query := range due {
		query.complete(now.Add(time.Minute))
	}
	due = getDueDataObservabilityQueries(queries, now.Add(70*time.Second))
	assert.Empty(t, due)
}

func TestDataObservabilityCompletedQueriesDoNotRetryWithFailedBatch(t *testing.T) {
	queries := newDataObservabilityScheduledQueries([]config.DataObservabilityQuery{
		{Schedule: "0 * * * *"},
		{Schedule: "0 * * * *"},
		{IntervalSeconds: 60},
	})
	now := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	due := getDueDataObservabilityQueries(queries, now)
	require.Len(t, due, 3)
	due[0].complete(now)
	for _, query := range due[1:] {
		query.retry()
	}
	due = getDueDataObservabilityQueries(queries, now.Add(10*time.Second))
	require.Len(t, due, 2)
	assert.Same(t, queries[1], due[0].scheduledQuery)
	assert.Same(t, queries[2], due[1].scheduledQuery)
}
