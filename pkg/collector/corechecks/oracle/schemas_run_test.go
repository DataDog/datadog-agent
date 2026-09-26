// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build oracle_test

package oracle

import (
	"context"
	"encoding/json"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/DataDog/datadog-agent/pkg/aggregator/mocksender"
	"github.com/jmoiron/sqlx"
	"github.com/stretchr/testify/require"
)

func waitForSchemaWorker(t *testing.T, c *Check) {
	t.Helper()
	c.schemaWorkerMu.Lock()
	done := c.schemaWorkerDone
	c.schemaWorkerMu.Unlock()
	if done != nil {
		select {
		case <-done:
		case <-time.After(5 * time.Second):
			t.Fatal("schema collection did not finish")
		}
	}
}

func prepareSchemaRun(c *Check) {
	c.initialized = true
	c.dbmEnabled = true
	c.config.QuerySamples.Enabled = false
	c.metricLastRun = time.Now()
	c.dbInstanceLastRun = c.metricLastRun
	c.tablespaceLastRun = c.metricLastRun
}

func TestSchemaRunDoesNotWaitForConnection(t *testing.T) {
	c, pool, dbMock, cleanup := newSchemaCheck(t)
	defer cleanup()
	defer c.Cancel()
	prepareSchemaRun(&c)
	pool.SetMaxOpenConns(1)
	held, err := pool.Connx(context.Background())
	require.NoError(t, err)
	defer held.Close()
	dbMock.ExpectQuery(`v\$containers`).WillReturnRows(sqlmock.NewRows([]string{"CON_ID", "NAME"}).AddRow(3, "PDB"))
	dbMock.ExpectQuery("cdb_users").WillReturnRows(sqlmock.NewRows([]string{"CON_ID", "USERNAME", "USER_ID"}))
	expectSchemaContainerAvailable(dbMock, 3)

	returned := make(chan error, 1)
	go func() { returned <- c.Run() }()
	select {
	case err := <-returned:
		require.NoError(t, err)
	case <-time.After(5 * time.Second):
		t.Fatal("Run waited for the schema connection")
	}
	rawSender, err := c.GetRawSender()
	require.NoError(t, err)
	sender := rawSender.(*mocksender.MockSender)
	sender.AssertNumberOfCalls(t, "Commit", 1)
	sender.AssertNotCalled(t, "EventPlatformEvent")
	require.NoError(t, held.Close())
	waitForSchemaWorker(t, &c)
	sender.AssertNumberOfCalls(t, "EventPlatformEvent", 1)
	sender.AssertNumberOfCalls(t, "Commit", 1)
	require.Zero(t, pool.Stats().InUse)
	require.NoError(t, dbMock.ExpectationsWereMet())
}

func TestSchemaRunTracingWaitsBeforeDisablingTrace(t *testing.T) {
	started := make(chan struct{})
	release := make(chan struct{})
	db, dbMock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherFunc(func(expected, actual string) error {
		if strings.Contains(actual, "v$containers") && !strings.Contains(actual, " AND con_id = ") {
			close(started)
			<-release
		}
		return sqlmock.QueryMatcherRegexp.Match(expected, actual)
	})))
	require.NoError(t, err)
	defer db.Close()
	c, sender := newDbDoesNotExistCheck(t, "", "")
	c.db = sqlx.NewDb(db, "sqlmock")
	c.db.SetMaxOpenConns(1)
	c.dbVersion = "23.0.0.0.0"
	c.config.Schemas.Enabled = true
	c.config.AgentSQLTrace.Enabled = true
	c.config.AgentSQLTrace.TracedRuns = 1
	prepareSchemaRun(&c)
	dbMock.ExpectQuery(`v\$containers`).WillReturnRows(sqlmock.NewRows([]string{"CON_ID", "NAME"}).AddRow(3, "PDB"))
	dbMock.ExpectQuery("cdb_users").WillReturnRows(sqlmock.NewRows([]string{"CON_ID", "USERNAME", "USER_ID"}))
	expectSchemaContainerAvailable(dbMock, 3)
	dbMock.ExpectExec(regexp.QuoteMeta("BEGIN dbms_monitor.session_trace_disable; END;")).WillReturnResult(sqlmock.NewResult(0, 0))
	returned := make(chan error, 1)
	go func() { returned <- c.Run() }()
	select {
	case <-started:
	case <-time.After(5 * time.Second):
		close(release)
		c.Cancel()
		t.Fatal("traced schema collection did not start")
	}
	select {
	case <-returned:
		close(release)
		t.Fatal("tracing Run returned before schema collection finished")
	default:
	}
	sender.AssertNotCalled(t, "Commit")
	close(release)
	select {
	case err := <-returned:
		require.NoError(t, err)
	case <-time.After(5 * time.Second):
		t.Fatal("tracing Run did not finish")
	}
	c.Cancel()
	require.False(t, c.config.AgentSQLTrace.Enabled)
	require.Equal(t, MAX_OPEN_CONNECTIONS, c.db.Stats().MaxOpenConnections)
	sender.AssertNumberOfCalls(t, "EventPlatformEvent", 1)
	sender.AssertNumberOfCalls(t, "Commit", 1)
	require.NoError(t, dbMock.ExpectationsWereMet())
}

func TestSchemaRunKeepsReservedConnectionDuringPoolReplacement(t *testing.T) {
	started := make(chan struct{})
	release := make(chan struct{})
	db, dbMock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherFunc(func(expected, actual string) error {
		if strings.Contains(actual, "v$containers") && !strings.Contains(actual, " AND con_id = ") {
			close(started)
			<-release
		}
		return sqlmock.QueryMatcherRegexp.Match(expected, actual)
	})))
	require.NoError(t, err)
	defer db.Close()
	c, sender := newDbDoesNotExistCheck(t, "", "")
	c.db = sqlx.NewDb(db, "sqlmock")
	c.dbVersion = "23.0.0.0.0"
	c.config.Schemas.Enabled = true
	c.tags = []string{"original"}
	prepareSchemaRun(&c)
	expectEmptySchemaSnapshot(dbMock)
	dbMock.ExpectClose()
	require.NoError(t, c.collectSchemasIfDue())
	select {
	case <-started:
	case <-time.After(5 * time.Second):
		close(release)
		c.Cancel()
		t.Fatal("schema collection did not reserve its connection")
	}
	oldPool := c.db
	c.db = nil
	c.tags[0] = "changed"
	err = oldPool.Close()
	close(release)
	waitForSchemaWorker(t, &c)
	c.Cancel()
	require.NoError(t, err)
	require.NoError(t, dbMock.ExpectationsWereMet())
	sender.AssertNumberOfCalls(t, "EventPlatformEvent", 1)
	for _, call := range sender.Calls {
		if call.Method == "EventPlatformEvent" {
			var event schemaEvent
			require.NoError(t, json.Unmarshal(call.Arguments.Get(0).([]byte), &event))
			require.Equal(t, []string{"original"}, event.Tags)
			require.Equal(t, 1, event.CollectionPayloadsCount)
		}
	}
}
