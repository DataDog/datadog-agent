// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build oracle

package oracle

import (
	"encoding/json"
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

func TestStatementMetricsSkipsZeroDeltaRows(t *testing.T) {
	db, dbMock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	c, sender := statementMetricsTestCheck(t, sqlx.NewDb(db, "sqlmock"))

	// Seed previous cache as if a prior collection interval already observed these rows.
	idleKey := StatementMetricsKeyDB{
		ConID:                  1,
		PDBName:                "PDB1",
		ForceMatchingSignature: "1001",
		PlanHashValue:          11111,
	}
	activeKey := StatementMetricsKeyDB{
		ConID:                  1,
		PDBName:                "PDB1",
		ForceMatchingSignature: "1002",
		PlanHashValue:          0, // skip plan lookup for the active row to isolate assertions
	}
	parseOnlyKey := StatementMetricsKeyDB{
		ConID:                  1,
		PDBName:                "PDB1",
		SQLID:                  "sql_parse_only",
		ForceMatchingSignature: "",
		PlanHashValue:          0,
	}
	negativeKey := StatementMetricsKeyDB{
		ConID:                  1,
		PDBName:                "PDB1",
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
	// No getFullSQLText or plan query expectations: any query for the idle row fails dbMock.ExpectationsWereMet().

	c.statementsLastRun = time.Now().Add(-2 * time.Hour)
	sqlCount, err := c.StatementMetrics()
	require.NoError(t, err)
	assert.Equal(t, 4, sqlCount)
	require.NoError(t, dbMock.ExpectationsWereMet())

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
}
