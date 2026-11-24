// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build oracle

package oracle

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/integration"
	"github.com/DataDog/datadog-agent/pkg/aggregator/mocksender"
	"github.com/jmoiron/sqlx"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func statementMetricsTestCheck(t *testing.T, db *sqlx.DB) (*Check, *mocksender.MockSender) {
	t.Helper()
	c := newCheck().(*Check)
	manager := mocksender.CreateDefaultDemultiplexer(t)
	require.NoError(t, c.Configure(manager, integration.FakeConfigHash, []byte(`server: 127.0.0.1
port: 1521
service_name: unused
username: unused
password: unused
dbm: true
propagate_agent_tags: false
query_metrics:
  enabled: true
execution_plans:
  enabled: true
`), []byte("{}"), "test", ""))
	sender := mocksender.NewMockSenderWithSenderManager(c.ID(), manager)
	sender.SetupAcceptAll()
	c.db = db
	c.dbVersion = "19.0"
	c.initialized = true
	t.Cleanup(c.Teardown)
	return c, sender
}

var statementMetricsTestColumns = []string{
	"CON_ID", "PDB_NAME", "SQL_ID", "FORCE_MATCHING_SIGNATURE", "PLAN_HASH_VALUE",
	"SQL_TEXT", "SQL_TEXT_LENGTH",
	"PARSE_CALLS", "EXECUTIONS", "ELAPSED_TIME",
	"VERSION_COUNT", "SHARABLE_MEM", "TYPECHECK_MEM",
}

type recordingConnector struct {
	drv     driver.Driver
	dsn     string
	queries *[]string
}

func (rc recordingConnector) Connect(context.Context) (driver.Conn, error) {
	conn, err := rc.drv.Open(rc.dsn)
	if err != nil {
		return nil, err
	}
	return &recordingConn{Conn: conn, queries: rc.queries}, nil
}

func (rc recordingConnector) Driver() driver.Driver { return rc.drv }

type recordingConn struct {
	driver.Conn
	queries *[]string
}

func (rc *recordingConn) QueryContext(ctx context.Context, query string, args []driver.NamedValue) (driver.Rows, error) {
	*rc.queries = append(*rc.queries, query)
	if qc, ok := rc.Conn.(driver.QueryerContext); ok {
		return qc.QueryContext(ctx, query, args)
	}
	return nil, driver.ErrSkip
}

func (rc *recordingConn) PrepareContext(ctx context.Context, query string) (driver.Stmt, error) {
	*rc.queries = append(*rc.queries, query)
	if pc, ok := rc.Conn.(driver.ConnPrepareContext); ok {
		return pc.PrepareContext(ctx, query)
	}
	return rc.Conn.Prepare(query)
}

func newRecordingSQLMock(t *testing.T) (*sqlx.DB, sqlmock.Sqlmock, *[]string) {
	t.Helper()
	dsn := fmt.Sprintf("recording_%s_%d", t.Name(), time.Now().UnixNano())
	rawDB, dbMock, err := sqlmock.NewWithDSN(dsn)
	require.NoError(t, err)
	t.Cleanup(func() { _ = rawDB.Close() })

	var queries []string
	wrapped := sql.OpenDB(recordingConnector{
		drv:     rawDB.Driver(),
		dsn:     dsn,
		queries: &queries,
	})
	t.Cleanup(func() { _ = wrapped.Close() })
	return sqlx.NewDb(wrapped, "sqlmock"), dbMock, &queries
}

func TestStatementMetricsSkipsZeroDeltaRows(t *testing.T) {
	db, dbMock, executedQueries := newRecordingSQLMock(t)

	c, sender := statementMetricsTestCheck(t, db)

	// Seed previous cache as if a prior collection interval already observed these rows.
	idleKey := StatementMetricsKeyDB{
		ConID:                  1,
		PDBName:                sql.NullString{String: "PDB1", Valid: true},
		ForceMatchingSignature: "1001",
		PlanHashValue:          11111,
	}
	activeKey := StatementMetricsKeyDB{
		ConID:                  1,
		PDBName:                sql.NullString{String: "PDB1", Valid: true},
		ForceMatchingSignature: "1002",
		PlanHashValue:          0, // skip plan lookup for the active row to isolate assertions
	}
	parseOnlyKey := StatementMetricsKeyDB{
		ConID:                  1,
		PDBName:                sql.NullString{String: "PDB1", Valid: true},
		SQLID:                  "sql_parse_only",
		ForceMatchingSignature: "",
		PlanHashValue:          0,
	}
	negativeKey := StatementMetricsKeyDB{
		ConID:                  1,
		PDBName:                sql.NullString{String: "PDB1", Valid: true},
		SQLID:                  "sql_neg",
		ForceMatchingSignature: "",
		PlanHashValue:          0,
	}

	c.statementMetricsMonotonicCountsPrevious = map[StatementMetricsKeyDB]StatementMetricsMonotonicCountDB{
		idleKey:      {ParseCalls: 10, Executions: 20, ElapsedTime: 5000},
		activeKey:    {ParseCalls: 5, Executions: 10, ElapsedTime: 1000},
		parseOnlyKey: {ParseCalls: 2, Executions: 2, ElapsedTime: 100},
		negativeKey:  {ParseCalls: 5, Executions: 50, ElapsedTime: 2000},
	}

	// Interval 1:
	// - idleKey: identical monotonic counts, non-zero gauges, SQL_TEXT_LENGTH == MaxSQLFullTextVSQLStats,
	//   and non-zero PLAN_HASH_VALUE. Must be skipped before getFullSQLText, obfuscation, FQT, and plan queries.
	// - activeKey: executions and elapsed_time increased -> must be emitted.
	// - parseOnlyKey: only parse_calls increased -> must be emitted.
	// - negativeKey: executions decreased -> must be skipped.
	fmsRows := sqlmock.NewRows(statementMetricsTestColumns).
		AddRow(1, "PDB1", "sql_idle", "1001", uint64(11111), "SELECT * FROM idle_table WHERE id = 1", MaxSQLFullTextVSQLStats, 10.0, 20.0, 5000.0, 5.0, 1048576.0, 4096.0).
		AddRow(1, "PDB1", "sql_active", "1002", uint64(0), "SELECT * FROM active_table WHERE id = 42", 40, 5.0, 13.0, 1750.0, 1.0, 65536.0, 1024.0)
	sqlIDRows := sqlmock.NewRows(statementMetricsTestColumns).
		AddRow(1, "PDB1", "sql_parse_only", "", uint64(0), "SELECT * FROM parse_only_table", 30, 3.0, 2.0, 100.0, 1.0, 8192.0, 512.0).
		AddRow(1, "PDB1", "sql_neg", "", uint64(0), "SELECT * FROM reset_table", 25, 5.0, 10.0, 2000.0, 1.0, 8192.0, 512.0)

	dbMock.ExpectQuery("DD_QM_FMS").WillReturnRows(fmsRows)
	dbMock.ExpectQuery("DD_QM_SQLID").WillReturnRows(sqlIDRows)

	c.statementsLastRun = time.Now().Add(-2 * time.Hour)
	sqlCount, err := c.StatementMetrics()
	require.NoError(t, err)
	assert.Equal(t, 4, sqlCount)
	require.NoError(t, dbMock.ExpectationsWereMet())
	// Explicitly verify only the 2 statement-metrics queries ran (no swallowed getFullSQLText or plan queries).
	require.Len(t, *executedQueries, 2)
	assert.Contains(t, (*executedQueries)[0], "DD_QM_FMS")
	assert.Contains(t, (*executedQueries)[1], "DD_QM_SQLID")

	require.Len(t, c.lastOracleRows, 2)
	rowsByHash := make(map[string]OracleRow, len(c.lastOracleRows))
	for _, row := range c.lastOracleRows {
		rowsByHash[row.QueryHash] = row
	}
	assert.NotContains(t, rowsByHash, "1001", "idle zero-delta row must be filtered out")
	assert.NotContains(t, rowsByHash, "sql_neg", "negative-delta row must be filtered out")

	require.Contains(t, rowsByHash, "1002")
	assert.Equal(t, float64(3), rowsByHash["1002"].Executions)
	assert.Equal(t, float64(750), rowsByHash["1002"].ElapsedTime)
	assert.Equal(t, float64(65536), rowsByHash["1002"].SharableMem)

	require.Contains(t, rowsByHash, "sql_parse_only")
	assert.Equal(t, float64(1), rowsByHash["sql_parse_only"].ParseCalls)
	assert.Equal(t, float64(0), rowsByHash["sql_parse_only"].Executions)

	// Verify emitted payloads: no FQT for idle row, and dbm-metrics contains only the 2 active rows.
	var fqtStatements []string
	var metricsPayloads []MetricsPayload
	for _, call := range sender.Calls {
		if call.Method != "EventPlatformEvent" {
			continue
		}
		raw := call.Arguments.Get(0).([]byte)
		eventType := call.Arguments.String(1)
		switch eventType {
		case "dbm-samples":
			var fqt FQTPayload
			require.NoError(t, json.Unmarshal(raw, &fqt))
			if fqt.DBMType == "fqt" {
				fqtStatements = append(fqtStatements, fqt.FQTDB.Statement)
			}
		case "dbm-metrics":
			var mp MetricsPayload
			require.NoError(t, json.Unmarshal(raw, &mp))
			metricsPayloads = append(metricsPayloads, mp)
		}
	}
	assert.Len(t, fqtStatements, 2)
	for _, stmt := range fqtStatements {
		assert.NotContains(t, stmt, "idle_table")
	}
	require.Len(t, metricsPayloads, 1)
	assert.Len(t, metricsPayloads[0].OracleRows, 2)

	// Verify the idle row stayed in the cache: when it runs in the next interval, its delta is emitted.
	require.Contains(t, c.statementMetricsMonotonicCountsPrevious, idleKey)
	fmsRowsNext := sqlmock.NewRows(statementMetricsTestColumns).
		AddRow(1, "PDB1", "sql_idle", "1001", uint64(11111), "SELECT * FROM idle_table WHERE id = 1", 37, 11.0, 22.0, 5200.0, 5.0, 1048576.0, 4096.0)
	sqlIDRowsNext := sqlmock.NewRows(statementMetricsTestColumns)

	// Plan query is now expected because the previously-idle row became active and has PlanHashValue != 0.
	dbMock.ExpectQuery("DD_QM_FMS").WillReturnRows(fmsRowsNext)
	dbMock.ExpectQuery("DD_QM_SQLID").WillReturnRows(sqlIDRowsNext)
	dbMock.ExpectQuery("v\\$sql_plan").WillReturnRows(sqlmock.NewRows([]string{"SQL_ID"}))

	c.statementsLastRun = time.Now().Add(-2 * time.Hour)
	sqlCount, err = c.StatementMetrics()
	require.NoError(t, err)
	assert.Equal(t, 1, sqlCount)
	require.NoError(t, dbMock.ExpectationsWereMet())

	require.Len(t, c.lastOracleRows, 1)
	assert.Equal(t, "1001", c.lastOracleRows[0].QueryHash)
	assert.Equal(t, float64(1), c.lastOracleRows[0].ParseCalls)
	assert.Equal(t, float64(2), c.lastOracleRows[0].Executions)
	assert.Equal(t, float64(200), c.lastOracleRows[0].ElapsedTime)

	// Interval 3 (all-idle): zero rows survive, no full-text/plan queries run, and an empty dbm-metrics
	// payload carrying oracle_version is still emitted to preserve DBM host presence/billing.
	queriesBeforeIdle := len(*executedQueries)
	callsBeforeIdle := len(sender.Calls)
	dbMock.ExpectQuery("DD_QM_FMS").WillReturnRows(
		sqlmock.NewRows(statementMetricsTestColumns).
			AddRow(1, "PDB1", "sql_idle", "1001", uint64(11111), "SELECT * FROM idle_table WHERE id = 1", MaxSQLFullTextVSQLStats, 11.0, 22.0, 5200.0, 5.0, 1048576.0, 4096.0),
	)
	dbMock.ExpectQuery("DD_QM_SQLID").WillReturnRows(sqlmock.NewRows(statementMetricsTestColumns))

	c.statementsLastRun = time.Now().Add(-2 * time.Hour)
	sqlCount, err = c.StatementMetrics()
	require.NoError(t, err)
	assert.Equal(t, 1, sqlCount)
	require.NoError(t, dbMock.ExpectationsWereMet())
	assert.Len(t, (*executedQueries)[queriesBeforeIdle:], 2, "all-idle interval must not attempt full-text or plan queries")
	assert.Empty(t, c.lastOracleRows)

	var idleMetricsPayloads []MetricsPayload
	for _, call := range sender.Calls[callsBeforeIdle:] {
		if call.Method == "EventPlatformEvent" && call.Arguments.String(1) == "dbm-metrics" {
			var mp MetricsPayload
			require.NoError(t, json.Unmarshal(call.Arguments.Get(0).([]byte), &mp))
			idleMetricsPayloads = append(idleMetricsPayloads, mp)
		}
	}
	require.Len(t, idleMetricsPayloads, 1)
	assert.Empty(t, idleMetricsPayloads[0].OracleRows)
	assert.Equal(t, "19.0", idleMetricsPayloads[0].OracleVersion)
}

func TestStatementMetricsEnforcesMaxRunTimeForPlans(t *testing.T) {
	t.Run("stops before querying plan once deadline exceeded after prior plan fetch", func(t *testing.T) {
		var c *Check
		matcher := sqlmock.QueryMatcherFunc(func(expected, actual string) error {
			if err := sqlmock.QueryMatcherRegexp.Match(expected, actual); err != nil {
				return err
			}
			// Simulate MaxRunTime expiring as soon as the first plan query executes.
			if strings.Contains(actual, "v$sql_plan_statistics_all") {
				c.config.QueryMetrics.MaxRunTime = 0
			}
			return nil
		})
		db, dbMock, err := sqlmock.New(sqlmock.QueryMatcherOption(matcher))
		require.NoError(t, err)
		defer db.Close()

		c, _ = statementMetricsTestCheck(t, sqlx.NewDb(db, "sqlmock"))
		c.config.QueryMetrics.MaxRunTime = 60

		// 2 idle rows followed by 3 active rows (indices 2, 3, 4 — none satisfy (i+1)%10 == 0).
		c.statementMetricsMonotonicCountsPrevious = map[StatementMetricsKeyDB]StatementMetricsMonotonicCountDB{
			{ConID: 1, PDBName: sql.NullString{String: "PDB1", Valid: true}, ForceMatchingSignature: "idle1", PlanHashValue: 1001}:   {Executions: 5},
			{ConID: 1, PDBName: sql.NullString{String: "PDB1", Valid: true}, ForceMatchingSignature: "idle2", PlanHashValue: 1002}:   {Executions: 5},
			{ConID: 1, PDBName: sql.NullString{String: "PDB1", Valid: true}, ForceMatchingSignature: "active1", PlanHashValue: 2001}: {Executions: 5},
			{ConID: 1, PDBName: sql.NullString{String: "PDB1", Valid: true}, ForceMatchingSignature: "active2", PlanHashValue: 2002}: {Executions: 5},
			{ConID: 1, PDBName: sql.NullString{String: "PDB1", Valid: true}, ForceMatchingSignature: "active3", PlanHashValue: 2003}: {Executions: 5},
		}

		fmsRows := sqlmock.NewRows(statementMetricsTestColumns).
			AddRow(1, "PDB1", "s_idle1", "idle1", uint64(1001), "SELECT 1 FROM dual", 18, 0.0, 5.0, 0.0, 1.0, 1024.0, 0.0).
			AddRow(1, "PDB1", "s_idle2", "idle2", uint64(1002), "SELECT 2 FROM dual", 18, 0.0, 5.0, 0.0, 1.0, 1024.0, 0.0).
			AddRow(1, "PDB1", "s_act1", "active1", uint64(2001), "SELECT * FROM t1", 16, 0.0, 6.0, 100.0, 1.0, 1024.0, 0.0).
			AddRow(1, "PDB1", "s_act2", "active2", uint64(2002), "SELECT * FROM t2", 16, 0.0, 7.0, 200.0, 1.0, 1024.0, 0.0).
			AddRow(1, "PDB1", "s_act3", "active3", uint64(2003), "SELECT * FROM t3", 16, 0.0, 8.0, 300.0, 1.0, 1024.0, 0.0)
		sqlIDRows := sqlmock.NewRows(statementMetricsTestColumns)

		dbMock.ExpectQuery("DD_QM_FMS").WillReturnRows(fmsRows)
		dbMock.ExpectQuery("DD_QM_SQLID").WillReturnRows(sqlIDRows)
		// Only active1 should query for its plan; active2 and active3 must skip plan collection.
		dbMock.ExpectQuery("v\\$sql_plan_statistics_all").
			WithArgs("s_act1", uint64(2001), 1).
			WillReturnRows(sqlmock.NewRows([]string{"SQL_ID"}))

		c.statementsLastRun = time.Now().Add(-2 * time.Hour)
		_, err = c.StatementMetrics()
		require.NoError(t, err)
		require.NoError(t, dbMock.ExpectationsWereMet())

		// Statement metrics for all 3 active rows are still emitted even after plan fetching stops.
		require.Len(t, c.lastOracleRows, 3)
	})

	t.Run("skips plan query on first active row when deadline already elapsed", func(t *testing.T) {
		db, dbMock, err := sqlmock.New()
		require.NoError(t, err)
		defer db.Close()

		c, _ := statementMetricsTestCheck(t, sqlx.NewDb(db, "sqlmock"))
		c.config.QueryMetrics.MaxRunTime = 0
		c.statementMetricsMonotonicCountsPrevious = map[StatementMetricsKeyDB]StatementMetricsMonotonicCountDB{
			{ConID: 1, PDBName: sql.NullString{String: "PDB1", Valid: true}, ForceMatchingSignature: "active1", PlanHashValue: 3001}: {Executions: 1},
		}

		fmsRows := sqlmock.NewRows(statementMetricsTestColumns).
			AddRow(1, "PDB1", "s_act1", "active1", uint64(3001), "SELECT * FROM t1", 16, 0.0, 2.0, 50.0, 1.0, 1024.0, 0.0)
		dbMock.ExpectQuery("DD_QM_FMS").WillReturnRows(fmsRows)
		dbMock.ExpectQuery("DD_QM_SQLID").WillReturnRows(sqlmock.NewRows(statementMetricsTestColumns))
		// No plan query expected.

		c.statementsLastRun = time.Now().Add(-2 * time.Hour)
		_, err = c.StatementMetrics()
		require.NoError(t, err)
		require.NoError(t, dbMock.ExpectationsWereMet())
		require.Len(t, c.lastOracleRows, 1)
	})
}

func TestStatementMetricsBaselinesCacheOnFirstRun(t *testing.T) {
	db, dbMock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	c, _ := statementMetricsTestCheck(t, sqlx.NewDb(db, "sqlmock"))
	require.Nil(t, c.statementMetricsMonotonicCountsPrevious)

	// Run 1 (cold cache): should baseline previous counts from the first query instead of discarding them.
	dbMock.ExpectQuery("DD_QM_FMS").WillReturnRows(
		sqlmock.NewRows(statementMetricsTestColumns).
			AddRow(1, "PDB1", "sql_fms", "1001", uint64(0), "SELECT * FROM orders WHERE id = 1", 33, 2.0, 10.0, 500.0, 1.0, 4096.0, 0.0),
	)
	dbMock.ExpectQuery("DD_QM_SQLID").WillReturnRows(
		sqlmock.NewRows(statementMetricsTestColumns).
			AddRow(1, "PDB1", "sql_only", "", uint64(0), "SELECT * FROM dual", 18, 1.0, 3.0, 150.0, 1.0, 2048.0, 0.0),
	)

	sqlCount, err := c.StatementMetrics()
	require.NoError(t, err)
	assert.Equal(t, 2, sqlCount)
	assert.Empty(t, c.lastOracleRows, "first run must only establish a baseline, not emit cumulative totals")
	assert.Len(t, c.statementMetricsMonotonicCountsPrevious, 2)
	require.NoError(t, dbMock.ExpectationsWereMet())

	// Run 2: should immediately emit deltas against Run 1 (rather than requiring a 3rd run).
	dbMock.ExpectQuery("DD_QM_FMS").WillReturnRows(
		sqlmock.NewRows(statementMetricsTestColumns).
			AddRow(1, "PDB1", "sql_fms", "1001", uint64(0), "SELECT * FROM orders WHERE id = 1", 33, 3.0, 14.0, 900.0, 1.0, 4096.0, 0.0),
	)
	dbMock.ExpectQuery("DD_QM_SQLID").WillReturnRows(
		sqlmock.NewRows(statementMetricsTestColumns).
			AddRow(1, "PDB1", "sql_only", "", uint64(0), "SELECT * FROM dual", 18, 1.0, 3.0, 150.0, 1.0, 2048.0, 0.0),
	)

	c.statementsLastRun = time.Now().Add(-2 * time.Hour)
	sqlCount, err = c.StatementMetrics()
	require.NoError(t, err)
	assert.Equal(t, 2, sqlCount)
	require.NoError(t, dbMock.ExpectationsWereMet())

	require.Len(t, c.lastOracleRows, 1)
	assert.Equal(t, "1001", c.lastOracleRows[0].QueryHash)
	assert.Equal(t, float64(1), c.lastOracleRows[0].ParseCalls)
	assert.Equal(t, float64(4), c.lastOracleRows[0].Executions)
	assert.Equal(t, float64(400), c.lastOracleRows[0].ElapsedTime)
}

func TestStatementMetricsReplacesCacheAndCombinesQueries(t *testing.T) {
	db, dbMock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	c, _ := statementMetricsTestCheck(t, sqlx.NewDb(db, "sqlmock"))

	fmsKey := StatementMetricsKeyDB{ConID: 1, PDBName: sql.NullString{String: "PDB1", Valid: true}, ForceMatchingSignature: "2001"}
	sqlIDKey := StatementMetricsKeyDB{ConID: 1, PDBName: sql.NullString{String: "PDB1", Valid: true}, SQLID: "sql_only"}
	staleKey := StatementMetricsKeyDB{ConID: 1, PDBName: sql.NullString{String: "PDB1", Valid: true}, ForceMatchingSignature: "stale_fms"}

	c.statementMetricsMonotonicCountsPrevious = map[StatementMetricsKeyDB]StatementMetricsMonotonicCountDB{
		fmsKey:   {Executions: 10},
		sqlIDKey: {Executions: 5},
		staleKey: {Executions: 99},
	}

	// Interval 1: both FMS and SQL_ID queries return active rows; staleKey is absent and must be evicted.
	dbMock.ExpectQuery("DD_QM_FMS").WillReturnRows(
		sqlmock.NewRows(statementMetricsTestColumns).
			AddRow(1, "PDB1", "sql_fms", "2001", uint64(0), "SELECT * FROM fms_tbl", 21, 0.0, 12.0, 100.0, 1.0, 1024.0, 0.0),
	)
	dbMock.ExpectQuery("DD_QM_SQLID").WillReturnRows(
		sqlmock.NewRows(statementMetricsTestColumns).
			AddRow(1, "PDB1", "sql_only", "", uint64(0), "SELECT * FROM sqlid_tbl", 23, 0.0, 8.0, 200.0, 1.0, 1024.0, 0.0),
	)

	c.statementsLastRun = time.Now().Add(-2 * time.Hour)
	sqlCount, err := c.StatementMetrics()
	require.NoError(t, err)
	assert.Equal(t, 2, sqlCount)
	require.NoError(t, dbMock.ExpectationsWereMet())

	require.Len(t, c.lastOracleRows, 2)
	rowsByHash := make(map[string]OracleRow, len(c.lastOracleRows))
	for _, row := range c.lastOracleRows {
		rowsByHash[row.QueryHash] = row
	}
	assert.Equal(t, float64(2), rowsByHash["2001"].Executions)
	assert.Equal(t, float64(3), rowsByHash["sql_only"].Executions)

	assert.Contains(t, c.statementMetricsMonotonicCountsPrevious, fmsKey)
	assert.Contains(t, c.statementMetricsMonotonicCountsPrevious, sqlIDKey)
	assert.NotContains(t, c.statementMetricsMonotonicCountsPrevious, staleKey, "stale keys absent from lookback must be evicted")

	// Interval 2: SQL_ID query returns 0 rows; FMS row still computes accurately and sqlIDKey is evicted.
	dbMock.ExpectQuery("DD_QM_FMS").WillReturnRows(
		sqlmock.NewRows(statementMetricsTestColumns).
			AddRow(1, "PDB1", "sql_fms", "2001", uint64(0), "SELECT * FROM fms_tbl", 21, 0.0, 15.0, 300.0, 1.0, 1024.0, 0.0),
	)
	dbMock.ExpectQuery("DD_QM_SQLID").WillReturnRows(sqlmock.NewRows(statementMetricsTestColumns))

	c.statementsLastRun = time.Now().Add(-2 * time.Hour)
	sqlCount, err = c.StatementMetrics()
	require.NoError(t, err)
	assert.Equal(t, 1, sqlCount)
	require.NoError(t, dbMock.ExpectationsWereMet())

	require.Len(t, c.lastOracleRows, 1)
	assert.Equal(t, "2001", c.lastOracleRows[0].QueryHash)
	assert.Equal(t, float64(3), c.lastOracleRows[0].Executions)
	assert.Len(t, c.statementMetricsMonotonicCountsPrevious, 1)
	assert.Contains(t, c.statementMetricsMonotonicCountsPrevious, fmsKey)
}

func TestGetFullPDBName(t *testing.T) {
	c := &Check{cdbName: "ORCLCDB"}
	assert.Equal(t, "ORCLCDB", c.getFullPDBName(""))
	assert.Equal(t, "ORCLCDB.PDB1", c.getFullPDBName("PDB1"))
}

func TestStatementMetricsNullPDBName(t *testing.T) {
	db, dbMock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	c, sender := statementMetricsTestCheck(t, sqlx.NewDb(db, "sqlmock"))
	c.cdbName = "ORCLCDB"

	// Run 1 (baseline): NULL PDB_NAME in both FMS and SQL_ID queries must scan cleanly without error.
	dbMock.ExpectQuery("DD_QM_FMS").WillReturnRows(
		sqlmock.NewRows(statementMetricsTestColumns).
			AddRow(0, nil, "sql_root", "5001", uint64(9001), "SELECT * FROM cdb_root_tbl", 26, 1.0, 5.0, 100.0, 1.0, 4096.0, 0.0).
			AddRow(3, "PDB1", "sql_pdb", "5002", uint64(9002), "SELECT * FROM pdb_tbl", 21, 1.0, 5.0, 100.0, 1.0, 4096.0, 0.0),
	)
	dbMock.ExpectQuery("DD_QM_SQLID").WillReturnRows(
		sqlmock.NewRows(statementMetricsTestColumns).
			AddRow(0, nil, "sql_noncdb", "", uint64(0), "SELECT * FROM noncdb_tbl", 24, 1.0, 2.0, 50.0, 1.0, 2048.0, 0.0),
	)

	sqlCount, err := c.StatementMetrics()
	require.NoError(t, err)
	assert.Equal(t, 3, sqlCount)
	assert.Empty(t, c.lastOracleRows)
	require.NoError(t, dbMock.ExpectationsWereMet())

	nullRootKey := StatementMetricsKeyDB{
		ConID:                  0,
		PDBName:                sql.NullString{},
		ForceMatchingSignature: "5001",
		PlanHashValue:          9001,
	}
	assert.Contains(t, c.statementMetricsMonotonicCountsPrevious, nullRootKey)

	// Run 2 (active): NULL PDB_NAME rows match cached keys, emit c.cdbName without trailing dot,
	// and omit empty "pdb:" tag from plan payloads while non-NULL PDB_NAME rows keep "pdb:PDB1".
	dbMock.ExpectQuery("DD_QM_FMS").WillReturnRows(
		sqlmock.NewRows(statementMetricsTestColumns).
			AddRow(0, nil, "sql_root", "5001", uint64(9001), "SELECT * FROM cdb_root_tbl", 26, 1.0, 8.0, 400.0, 1.0, 4096.0, 0.0).
			AddRow(3, "PDB1", "sql_pdb", "5002", uint64(9002), "SELECT * FROM pdb_tbl", 21, 1.0, 7.0, 300.0, 1.0, 4096.0, 0.0),
	)
	dbMock.ExpectQuery("DD_QM_SQLID").WillReturnRows(
		sqlmock.NewRows(statementMetricsTestColumns).
			AddRow(0, nil, "sql_noncdb", "", uint64(0), "SELECT * FROM noncdb_tbl", 24, 1.0, 4.0, 150.0, 1.0, 2048.0, 0.0),
	)

	planCols := []string{"CHILD_NUMBER", "OPERATION", "ID", "PARENT_ID", "DEPTH"}
	dbMock.ExpectQuery("v\\$sql_plan_statistics_all").
		WithArgs("sql_root", uint64(9001), 0).
		WillReturnRows(sqlmock.NewRows(planCols).AddRow(0, "SELECT STATEMENT", 0, 0, 0))
	dbMock.ExpectQuery("v\\$sql_plan_statistics_all").
		WithArgs("sql_pdb", uint64(9002), 3).
		WillReturnRows(sqlmock.NewRows(planCols).AddRow(0, "SELECT STATEMENT", 0, 0, 0))

	callsBefore := len(sender.Calls)
	c.statementsLastRun = time.Now().Add(-2 * time.Hour)
	sqlCount, err = c.StatementMetrics()
	require.NoError(t, err)
	assert.Equal(t, 3, sqlCount)
	require.NoError(t, dbMock.ExpectationsWereMet())

	require.Len(t, c.lastOracleRows, 3)
	rowsByHash := make(map[string]OracleRow, len(c.lastOracleRows))
	for _, row := range c.lastOracleRows {
		rowsByHash[row.QueryHash] = row
	}
	assert.Equal(t, "ORCLCDB", rowsByHash["5001"].PDBName)
	assert.Equal(t, float64(3), rowsByHash["5001"].Executions)
	assert.Equal(t, "ORCLCDB.PDB1", rowsByHash["5002"].PDBName)
	assert.Equal(t, float64(2), rowsByHash["5002"].Executions)
	assert.Equal(t, "ORCLCDB", rowsByHash["sql_noncdb"].PDBName)
	assert.Equal(t, float64(2), rowsByHash["sql_noncdb"].Executions)

	plansBySQLID := make(map[string]PlanPayload)
	for _, call := range sender.Calls[callsBefore:] {
		if call.Method != "EventPlatformEvent" || call.Arguments.String(1) != "dbm-samples" {
			continue
		}
		var pp PlanPayload
		require.NoError(t, json.Unmarshal(call.Arguments.Get(0).([]byte), &pp))
		if pp.DBMType == "plan" {
			plansBySQLID[pp.OraclePlan.SQLID] = pp
		}
	}
	require.Len(t, plansBySQLID, 2)

	rootPlan := plansBySQLID["sql_root"]
	assert.Empty(t, rootPlan.OraclePlan.PDBName)
	assert.NotContains(t, rootPlan.Tags, "pdb:", "NULL PDB_NAME must not emit an empty pdb: tag")

	pdbPlan := plansBySQLID["sql_pdb"]
	assert.Equal(t, "PDB1", pdbPlan.OraclePlan.PDBName)
	assert.Contains(t, strings.Split(pdbPlan.Tags, ","), "pdb:PDB1")
}
