// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build oracle

package oracle

import (
	"regexp"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/oracle/config"
	"github.com/jmoiron/sqlx"
	"github.com/stretchr/testify/require"
)

func TestSQLTraceExpiresWithoutPool(t *testing.T) {
	c := Check{config: &config.CheckConfig{}}
	c.config.AgentSQLTrace.Enabled = true
	c.config.AgentSQLTrace.TracedRuns = 2

	// A failed reconnect can leave the pool nil before tracing expires.
	c.finishSQLTraceRun()
	require.True(t, c.config.AgentSQLTrace.Enabled)
	require.NotPanics(t, c.finishSQLTraceRun)
	require.False(t, c.config.AgentSQLTrace.Enabled)

	// Subsequent runs must leave tracing disabled.
	c.finishSQLTraceRun()
	require.Equal(t, 2, c.sqlTraceRunsCount)
}

func TestSQLTraceExpiresWithPool(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	db.SetMaxOpenConns(1)
	c := Check{config: &config.CheckConfig{}, db: sqlx.NewDb(db, "sqlmock")}
	c.config.AgentSQLTrace.Enabled = true
	c.config.AgentSQLTrace.TracedRuns = 1
	mock.ExpectExec(regexp.QuoteMeta("BEGIN dbms_monitor.session_trace_disable; END;")).
		WillReturnResult(sqlmock.NewResult(0, 0))

	c.finishSQLTraceRun()

	require.False(t, c.config.AgentSQLTrace.Enabled)
	require.Equal(t, MAX_OPEN_CONNECTIONS, db.Stats().MaxOpenConnections)
	require.NoError(t, mock.ExpectationsWereMet())
}
