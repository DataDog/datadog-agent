// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build oracle

package oracle

import (
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/jmoiron/sqlx"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/integration"
	"github.com/DataDog/datadog-agent/pkg/aggregator/mocksender"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/oracle/common"
)

func newSamplingCheck(t *testing.T) (*Check, sqlmock.Sqlmock, *mocksender.MockSender) {
	t.Helper()
	db, dbMock, err := sqlmock.New()
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	c := &Check{}
	senderManager := mocksender.CreateDefaultDemultiplexer(t)
	require.NoError(t, c.Configure(senderManager, integration.FakeConfigHash, []byte("dbm: true\nserver: localhost\nservice_name: test\nusername: test\npassword: test"), nil, "oracle_test", ""))
	sender := mocksender.NewMockSenderWithSenderManager(c.ID(), senderManager)
	sender.SetupAcceptAll()
	c.db = sqlx.NewDb(db, "sqlmock")
	c.dbVersion = "23.0.0.0.0"
	c.config.QueryMetrics.CollectionInterval = 0
	c.config.QuerySamples.ForceDirectQuery = true
	c.statementMetricsMonotonicCountsPrevious = make(map[StatementMetricsKeyDB]StatementMetricsMonotonicCountDB)
	t.Cleanup(func() {
		if c.obfuscator != nil {
			c.obfuscator.Stop()
		}
	})
	return c, dbMock, sender
}

func expectStatementMetrics(dbMock sqlmock.Sqlmock, statements []StatementMetricsDB, executions int) {
	columns := []string{"CON_ID", "PDB_NAME", "SQL_ID", "FORCE_MATCHING_SIGNATURE", "PLAN_HASH_VALUE", "SQL_TEXT", "EXECUTIONS"}
	rows := sqlmock.NewRows(columns)
	for _, statement := range statements {
		rows.AddRow(statement.ConID, statement.PDBName, statement.SQLID, statement.ForceMatchingSignature, statement.PlanHashValue, statement.SQLText, executions)
	}
	dbMock.ExpectQuery("SELECT").WillReturnRows(rows)
	dbMock.ExpectQuery("SELECT").WillReturnRows(sqlmock.NewRows(columns))
}

// Queries sharing a plan shape must each receive a plan, including in separate PDBs;
// changing the representative SQL ID of an FMS group must not bypass deduplication.
func TestStatementPlanCacheIdentity(t *testing.T) {
	c, dbMock, sender := newSamplingCheck(t)
	statements := []StatementMetricsDB{
		{StatementMetricsKeyDB: StatementMetricsKeyDB{ConID: 3, PDBName: "pdb1", SQLID: "sql1", ForceMatchingSignature: "100", PlanHashValue: 42}, SQLText: "SELECT a FROM dual"},
		{StatementMetricsKeyDB: StatementMetricsKeyDB{ConID: 3, PDBName: "pdb1", SQLID: "sql2", ForceMatchingSignature: "200", PlanHashValue: 42}, SQLText: "SELECT b FROM dual"},
		{StatementMetricsKeyDB: StatementMetricsKeyDB{ConID: 4, PDBName: "pdb2", SQLID: "sql3", ForceMatchingSignature: "100", PlanHashValue: 42}, SQLText: "SELECT a FROM dual"},
	}
	for run := 1; run <= 3; run++ {
		if run == 3 {
			statements[0].SQLID = "replacement"
		}
		expectStatementMetrics(dbMock, statements, run)
		if run == 2 {
			for _, statement := range statements {
				dbMock.ExpectQuery("FROM v\\$sql_plan_statistics_all").WithArgs(statement.SQLID, statement.PlanHashValue, statement.ConID).
					WillReturnRows(sqlmock.NewRows([]string{"CHILD_NUMBER", "OPERATION"}).AddRow(0, "SELECT STATEMENT"))
			}
		}
		_, err := c.StatementMetrics()
		require.NoError(t, err)
	}
	plans := make(map[string]bool)
	for _, call := range sender.Calls {
		if call.Method != "EventPlatformEvent" {
			continue
		}
		var payload PlanPayload
		require.NoError(t, json.Unmarshal(call.Arguments.Get(0).([]byte), &payload))
		if payload.DBMType == "plan" {
			key := fmt.Sprintf("%s:%s", payload.Tags, payload.PlanDB.QuerySignature)
			assert.False(t, plans[key], "duplicate plan event")
			plans[key] = true
		}
	}
	assert.Len(t, plans, 3)
	assert.NoError(t, dbMock.ExpectationsWereMet())
}

// SQL that cannot be obfuscated must never leak its literals through metrics, full query
// text, plan or activity payloads; other statements and session data remain useful.
func TestSamplingObfuscationFailure(t *testing.T) {
	c, dbMock, sender := newSamplingCheck(t)
	// Oracle permits q-quoted literals containing single quotes, but the legacy
	// parser rejects this one. Select that supported mode to exercise its error path.
	c.config.ObfuscatorOptions.ObfuscationMode = ""
	invalidSQL := "SELECT q'[DBM_SYNTHETIC_O'Reilly]' FROM dual"
	_, err := c.LazyInitObfuscator().ObfuscateSQLString(invalidSQL)
	require.Error(t, err, "fixture must exercise an obfuscation failure")
	statements := []StatementMetricsDB{
		{StatementMetricsKeyDB: StatementMetricsKeyDB{ConID: 3, SQLID: "invalid", PlanHashValue: 42}, SQLText: invalidSQL},
		{StatementMetricsKeyDB: StatementMetricsKeyDB{ConID: 3, SQLID: "valid"}, SQLText: "SELECT 1 FROM dual"},
	}
	for run := 1; run <= 2; run++ {
		expectStatementMetrics(dbMock, statements, run)
		_, err = c.StatementMetrics()
		assert.NoError(t, err)
	}
	require.Len(t, c.lastOracleRows, 2)
	assert.Equal(t, float64(1), c.lastOracleRows[0].Executions)
	assert.NotContains(t, c.lastOracleRows[0].SQLText, "DBM_SYNTHETIC_O")
	assert.NotEmpty(t, c.lastOracleRows[1].QuerySignature)

	dbMock.ExpectQuery("DD_ACTIVITY_SAMPLING").WillReturnRows(sqlmock.NewRows([]string{"NOW", "UTC_MS", "SID", "SERIAL#", "STATUS", "SQL_ID", "SQL_FULLTEXT", "PREV_SQL_ID", "PREV_SQL_FULLTEXT"}).
		AddRow("2024-01-01 00:00:00", float64(time.Now().UnixMilli()), 100, 1, "ACTIVE", "invalid", invalidSQL, nil, nil).
		AddRow("2024-01-01 00:00:00", float64(time.Now().UnixMilli()), 101, 1, "ACTIVE", nil, nil, "invalid", invalidSQL))
	require.NoError(t, c.SampleSession())
	require.Len(t, c.lastOracleActivityRows, 2)
	assert.Equal(t, uint64(100), c.lastOracleActivityRows[0].SessionID)
	assert.NotContains(t, c.lastOracleActivityRows[0].Statement, "DBM_SYNTHETIC_O")
	assert.True(t, c.lastOracleActivityRows[1].PreviousSQL)
	assert.NotContains(t, c.lastOracleActivityRows[1].Statement, "DBM_SYNTHETIC_O")

	var fqtCount, activityCount int
	for _, call := range sender.Calls {
		if call.Method != "EventPlatformEvent" {
			continue
		}
		data := call.Arguments.Get(0).([]byte)
		assert.NotContains(t, string(data), "DBM_SYNTHETIC_O")
		if call.Arguments.String(1) == "dbm-activity" {
			activityCount++
			continue
		}
		var payload FQTPayload
		require.NoError(t, json.Unmarshal(data, &payload))
		if payload.DBMType == "fqt" {
			fqtCount++
			assert.Equal(t, common.GetQuerySignature("SELECT ? FROM dual"), payload.FQTDB.QuerySignature)
		}
	}
	assert.Equal(t, 1, fqtCount)
	assert.Equal(t, 1, activityCount)
	assert.NoError(t, dbMock.ExpectationsWereMet())
}
