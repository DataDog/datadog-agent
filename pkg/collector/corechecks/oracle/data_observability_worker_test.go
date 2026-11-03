// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build oracle

package oracle

import (
	"context"
	"encoding/json"
	"errors"
	"regexp"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/benbjohnson/clock"
	"github.com/jmoiron/sqlx"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/aggregator/mocksender"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/oracle/config"
)

type dataObservabilityWorkerTestClock struct {
	*clock.Mock
	timers chan time.Duration
}

func (c *dataObservabilityWorkerTestClock) Timer(d time.Duration) *clock.Timer {
	timer := c.Mock.Timer(d)
	c.timers <- d
	return timer
}

func newDataObservabilityWorkerCheck(t *testing.T) (*Check, *dataObservabilityWorkerTestClock) {
	t.Helper()
	c := newCheck().(*Check)
	clk := &dataObservabilityWorkerTestClock{Mock: clock.NewMock(), timers: make(chan time.Duration, 10)}
	clk.Set(time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC))
	c.clock = clk
	c.config = &config.CheckConfig{}
	c.config.DataObservability = config.DataObservabilityConfig{
		Enabled: true, CollectionInterval: 2,
		Queries: []config.DataObservabilityQuery{{IntervalSeconds: 1}},
	}
	c.dataObservabilityQueries = newDataObservabilityScheduledQueries(c.config.DataObservability.Queries)
	newDataObservabilityTestSender(t, c)
	t.Cleanup(c.Cancel)
	return c, clk
}

func dataObservabilityWorkerDone(c *Check) <-chan struct{} {
	c.dataObservabilityWorkerMu.Lock()
	defer c.dataObservabilityWorkerMu.Unlock()
	return c.dataObservabilityWorkerDone
}

func waitForDataObservabilityWorker(t *testing.T, done <-chan struct{}) {
	t.Helper()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for worker")
	}
}

func TestDataObservabilityWorkerDisabledOrEmpty(t *testing.T) {
	c, _ := newDataObservabilityWorkerCheck(t)
	run := func(context.Context) error {
		t.Error("unexpected execution")
		return nil
	}
	c.config.DataObservability.Enabled = false
	require.NoError(t, c.runDataObservabilityJob(run))
	assert.Nil(t, dataObservabilityWorkerDone(c))
	c.config.DataObservability.Enabled = true
	c.dataObservabilityQueries = nil
	require.NoError(t, c.runDataObservabilityJob(run))
	assert.Nil(t, dataObservabilityWorkerDone(c))
}

func TestDataObservabilityWorkerIndependentPolling(t *testing.T) {
	c, clk := newDataObservabilityWorkerCheck(t)
	calls := make(chan time.Time, 10)
	run := func(context.Context) error {
		calls <- clk.Now()
		return nil
	}
	require.NoError(t, c.runDataObservabilityJob(run))
	start := clk.Now()
	assert.Equal(t, start, <-calls)
	assert.Equal(t, start, <-calls)
	assert.Equal(t, 2*time.Second, <-clk.timers)
	clk.Add(time.Second)
	select {
	case <-calls:
		t.Fatal("worker polled before the collection interval elapsed")
	default:
	}
	clk.Add(time.Second)
	assert.Equal(t, start.Add(2*time.Second), <-calls)
	assert.Equal(t, 2*time.Second, <-clk.timers)
	c.Cancel()
	waitForDataObservabilityWorker(t, dataObservabilityWorkerDone(c))
}

func TestDataObservabilityWorkerDoesNotOverlap(t *testing.T) {
	c, clk := newDataObservabilityWorkerCheck(t)
	started := make(chan struct{})
	require.NoError(t, c.runDataObservabilityJob(func(ctx context.Context) error {
		close(started)
		<-ctx.Done()
		return ctx.Err()
	}))
	waitForDataObservabilityWorker(t, started)
	done := dataObservabilityWorkerDone(c)
	clk.Add(time.Minute)
	require.NoError(t, c.runDataObservabilityJob(func(context.Context) error {
		t.Error("overlapping job")
		return nil
	}))
	assert.Equal(t, done, dataObservabilityWorkerDone(c))
	c.Cancel()
	waitForDataObservabilityWorker(t, done)
	assert.True(t, c.dataObservabilityWorkerStopped)
	assert.False(t, c.dataObservabilityWorkerRunning)
	require.NoError(t, c.runDataObservabilityJob(func(context.Context) error {
		t.Error("job started after cancellation")
		return nil
	}))
}

func TestDataObservabilityWorkerRunSync(t *testing.T) {
	c, clk := newDataObservabilityWorkerCheck(t)
	c.config.DataObservability.RunSync = true
	calls := 0
	run := func(context.Context) error {
		calls++
		return nil
	}
	require.NoError(t, c.runDataObservabilityJob(run))
	assert.Equal(t, 1, calls)
	assert.False(t, c.dataObservabilityWorkerRunning)
	require.NoError(t, c.runDataObservabilityJob(run))
	assert.Equal(t, 1, calls)
	clk.Add(2 * time.Second)
	require.NoError(t, c.runDataObservabilityJob(run))
	assert.Equal(t, 2, calls)
	clk.Add(2 * time.Second)
	failure := errors.New("connection failed")
	require.ErrorIs(t, c.runDataObservabilityJob(func(context.Context) error { return failure }), failure)
	require.NoError(t, c.runDataObservabilityJob(run))
	assert.Equal(t, 2, calls)
}

func TestDataObservabilityWorkerFailureAndPanicAllowRestart(t *testing.T) {
	for _, panicJob := range []bool{false, true} {
		t.Run(map[bool]string{false: "error", true: "panic"}[panicJob], func(t *testing.T) {
			c, _ := newDataObservabilityWorkerCheck(t)
			require.NoError(t, c.runDataObservabilityJob(func(context.Context) error {
				if panicJob {
					panic("execution panic")
				}
				return errors.New("connection failed")
			}))
			waitForDataObservabilityWorker(t, dataObservabilityWorkerDone(c))
			assert.False(t, c.dataObservabilityWorkerRunning)
			assert.False(t, c.dataObservabilityWorkerStopped)
			started := make(chan struct{})
			require.NoError(t, c.runDataObservabilityJob(func(ctx context.Context) error {
				close(started)
				<-ctx.Done()
				return ctx.Err()
			}))
			waitForDataObservabilityWorker(t, started)
		})
	}
}

func TestDataObservabilityWorkerInactivityAndHeartbeat(t *testing.T) {
	c, clk := newDataObservabilityWorkerCheck(t)
	c.config.DataObservability.CollectionInterval = 10
	calls := make(chan struct{}, 10)
	run := func(context.Context) error {
		calls <- struct{}{}
		return nil
	}
	require.NoError(t, c.runDataObservabilityJob(run))
	<-calls
	<-calls
	<-clk.timers
	done := dataObservabilityWorkerDone(c)
	clk.Add(10 * time.Second)
	<-calls
	<-clk.timers
	require.NoError(t, c.runDataObservabilityJob(run))
	assert.Equal(t, done, dataObservabilityWorkerDone(c))
	for range 2 {
		clk.Add(10 * time.Second)
		<-calls
		<-clk.timers
	}
	clk.Add(10 * time.Second)
	waitForDataObservabilityWorker(t, done)
	assert.False(t, c.dataObservabilityWorkerStopped)
	require.NoError(t, c.runDataObservabilityJob(run))
	<-calls
	<-clk.timers
}

func TestDataObservabilityWorkerTeardownAllowsRestart(t *testing.T) {
	c, _ := newDataObservabilityWorkerCheck(t)
	started := make(chan struct{})
	run := func(ctx context.Context) error {
		close(started)
		<-ctx.Done()
		return ctx.Err()
	}
	require.NoError(t, c.runDataObservabilityJob(run))
	waitForDataObservabilityWorker(t, started)
	c.Teardown()
	assert.False(t, c.dataObservabilityWorkerRunning)
	assert.False(t, c.dataObservabilityWorkerStopped)
	started = make(chan struct{})
	require.NoError(t, c.runDataObservabilityJob(run))
	waitForDataObservabilityWorker(t, started)
	c.Stop()
	assert.False(t, c.dataObservabilityWorkerRunning)
	assert.True(t, c.dataObservabilityWorkerStopped)
}

func TestDataObservabilityWorkerReconfigure(t *testing.T) {
	c, clk := newDataObservabilityWorkerCheck(t)
	started := make(chan struct{})
	require.NoError(t, c.runDataObservabilityJob(func(ctx context.Context) error {
		close(started)
		<-ctx.Done()
		return ctx.Err()
	}))
	waitForDataObservabilityWorker(t, started)
	done := dataObservabilityWorkerDone(c)
	require.NoError(t, c.Configure(nil, 1, []byte(`username: datadog
data_observability:
  enabled: true
  run_sync: true
  queries:
    - dbname: PDB2
      query: SELECT 1 FROM dual
      query_timeout: 1000
      schedule: '0 * * * *'
      entity: {}
`), nil, "file", "file"))
	waitForDataObservabilityWorker(t, done)
	assert.False(t, c.dataObservabilityWorkerRunning)
	assert.False(t, c.dataObservabilityWorkerStopped)
	require.Len(t, c.dataObservabilityQueries, 1)
	assert.Equal(t, "PDB2", c.dataObservabilityQueries[0].query.DBName)
	assert.True(t, c.dataObservabilityLastEvent.IsZero())
	assert.Nil(t, c.dataObservabilityEmitter)
	newDataObservabilityTestSender(t, c)
	c.clock = clk
	calls := 0
	require.NoError(t, c.runDataObservabilityJob(func(context.Context) error {
		calls++
		return nil
	}))
	assert.Equal(t, 1, calls)
}

func TestDataObservabilityWorkerConcurrentCancelAndLaunch(t *testing.T) {
	for range 25 {
		c, _ := newDataObservabilityWorkerCheck(t)
		start := make(chan struct{})
		launched := make(chan error, 1)
		canceled := make(chan struct{})
		go func() {
			<-start
			launched <- c.runDataObservabilityJob(func(ctx context.Context) error {
				<-ctx.Done()
				return ctx.Err()
			})
		}()
		go func() {
			<-start
			c.Cancel()
			close(canceled)
		}()
		close(start)
		require.NoError(t, <-launched)
		waitForDataObservabilityWorker(t, canceled)
		assert.True(t, c.dataObservabilityWorkerStopped)
		assert.False(t, c.dataObservabilityWorkerRunning)
	}
}

func TestDataObservabilityRunWithDBMDisabled(t *testing.T) {
	c := newCheck().(*Check)
	senderManager := mocksender.CreateDefaultDemultiplexer(t)
	require.NoError(t, c.Configure(senderManager, 1, []byte(`username: datadog
only_custom_queries: true
data_observability:
  enabled: true
  run_sync: true
  queries:
    - dbname: orcl.SALES
      query: SELECT 1 FROM dual
      query_timeout: 1000
      interval_seconds: 60
      entity: {}
`), nil, "file", "file"))
	sender := mocksender.NewMockSenderWithSenderManager(c.ID(), senderManager)
	sender.SetupAcceptAll()
	regularDB, regularMock, err := sqlmock.New()
	require.NoError(t, err)
	defer regularDB.Close()
	c.db = sqlx.NewDb(regularDB, "sqlmock")
	c.db.SetMaxOpenConns(1)
	regularMock.ExpectQuery(`FROM v\$instance`).WillReturnRows(sqlmock.NewRows([]string{"HOST_NAME", "INSTANCE_NAME", "VERSION_FULL"}).AddRow("db-host", "orcl", "12.1.0.2.0"))
	regularMock.ExpectQuery(`FROM v\$database`).WillReturnRows(sqlmock.NewRows([]string{"NAME", "CDB"}).AddRow("orcl", "YES"))
	regularMock.ExpectQuery(`sys_context\('USERENV','CON_ID'\)`).WillReturnRows(sqlmock.NewRows([]string{"TYPE"}).AddRow("PDB"))
	regularMock.ExpectQuery("SELECT 1 FROM dual").WillReturnRows(sqlmock.NewRows([]string{"N"}).AddRow(1))
	db, mock, err := sqlmock.New(sqlmock.MonitorPingsOption(true))
	require.NoError(t, err)
	c.dataObservabilityDB = sqlx.NewDb(db, "sqlmock")
	expectDataObservabilityConnection(mock, "SALES")
	mock.ExpectQuery(regexp.QuoteMeta("SELECT 1 FROM dual")).WillReturnRows(sqlmock.NewRows([]string{"N"}).AddRow(1)).RowsWillBeClosed()
	mock.ExpectClose()
	defer c.Cancel()
	require.NoError(t, c.Run())
	assert.False(t, c.dbmEnabled)
	sender.AssertNumberOfCalls(t, "EventPlatformEvent", 1)
	sender.AssertMetric(t, "Count", "dd.oracle.data_observability.query_executions", 1, "db-host", []string{
		"status:success", "db_type:oracle", "monitor_id:0", "cdb:orcl", "real_hostname:db-host",
		"oracle_version:12.1.0.2.0", "hosting_type:self-managed", "database_instance:" + c.dbInstanceIdentifier,
	})
	assert.Nil(t, c.dataObservabilityDB)
	sender.AssertNumberOfCalls(t, "Commit", 2)
	require.NoError(t, mock.ExpectationsWereMet())
	require.NoError(t, regularMock.ExpectationsWereMet())
}

func TestDataObservabilityWorkerCancelDuringExecution(t *testing.T) {
	query := dataObservabilityTestQuery("orcl.SALES", "SELECT 1 FROM dual")
	c, _ := newDataObservabilityWorkerCheck(t)
	c.cdbName = "orcl"
	c.connectedToPdb = true
	c.dataObservabilityQueries = newDataObservabilityScheduledQueries([]config.DataObservabilityQuery{query})
	started := make(chan struct{})
	matcher := sqlmock.QueryMatcherFunc(func(expected, actual string) error {
		if actual == query.Query {
			close(started)
		}
		return sqlmock.QueryMatcherRegexp.Match(expected, actual)
	})
	db, mock, err := sqlmock.New(sqlmock.MonitorPingsOption(true), sqlmock.QueryMatcherOption(matcher))
	require.NoError(t, err)
	c.dataObservabilityDB = sqlx.NewDb(db, "sqlmock")
	expectDataObservabilityConnection(mock, "SALES")
	mock.ExpectQuery(regexp.QuoteMeta(query.Query)).WillDelayFor(time.Hour).
		WillReturnRows(sqlmock.NewRows([]string{"N"}).AddRow(1))
	mock.ExpectClose()
	require.NoError(t, c.runDataObservabilityJob(c.collectDataObservability))
	waitForDataObservabilityWorker(t, started)
	c.Cancel()
	waitForDataObservabilityWorker(t, dataObservabilityWorkerDone(c))
	assert.Nil(t, c.dataObservabilityConn)
	assert.Nil(t, c.dataObservabilityDB)
	assert.NotNil(t, c.dataObservabilityQueries[0].pendingRetry)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestDataObservabilityBackgroundEmission(t *testing.T) {
	query := dataObservabilityTestQuery("orcl.SALES", "SELECT 1 FROM dual")
	query.IntervalSeconds = 2
	c, clk := newDataObservabilityWorkerCheck(t)
	c.cdbName = "orcl"
	c.dataObservabilityQueries = newDataObservabilityScheduledQueries([]config.DataObservabilityQuery{query})
	c.connectedToPdb = true
	c.dbHostname = "db-host"
	c.tags = []string{"env:test", "database_role:primary"}
	c.dataObservabilityEmitter = nil
	sender := newDataObservabilityTestSender(t, c)
	started, release := make(chan struct{}), make(chan struct{})
	released, firstQuery := false, true
	defer func() {
		if !released {
			close(release)
		}
	}()
	db, dbMock, err := sqlmock.New(sqlmock.MonitorPingsOption(true), sqlmock.QueryMatcherOption(sqlmock.QueryMatcherFunc(func(expected, actual string) error {
		if actual == query.Query && firstQuery {
			firstQuery = false
			close(started)
			<-release
		}
		return sqlmock.QueryMatcherRegexp.Match(expected, actual)
	})))
	require.NoError(t, err)
	c.dataObservabilityDB = sqlx.NewDb(db, "sqlmock")
	expectDataObservabilityConnection(dbMock, "SALES")
	dbMock.ExpectQuery(query.Query).WillReturnRows(sqlmock.NewRows([]string{"N"}).AddRow(1))
	dbMock.ExpectQuery(query.Query).WillReturnRows(sqlmock.NewRows([]string{"N"}).AddRow(2))
	dbMock.ExpectClose()
	require.NoError(t, c.runDataObservabilityJob(c.collectDataObservability))
	waitForDataObservabilityWorker(t, started)
	done := dataObservabilityWorkerDone(c)
	c.tags[0] = "changed"
	c.tags = []string{"env:test", "database_role:physical_standby", "dd.internal.resource:test"}
	require.NoError(t, c.runDataObservabilityJob(c.collectDataObservability))
	assert.Equal(t, done, dataObservabilityWorkerDone(c))
	close(release)
	released = true
	assert.Equal(t, 2*time.Second, <-clk.timers)
	clk.Add(2 * time.Second)
	assert.Equal(t, 2*time.Second, <-clk.timers)
	c.Cancel()
	waitForDataObservabilityWorker(t, dataObservabilityWorkerDone(c))
	sender.AssertNumberOfCalls(t, "EventPlatformEvent", 2)
	sender.AssertMetric(t, "Count", "dd.oracle.data_observability.query_executions", 1, "db-host", []string{"env:test", "status:success", "database_role:primary"})
	sender.AssertMetric(t, "Count", "dd.oracle.data_observability.query_executions", 1, "db-host", []string{"env:test", "status:success", "database_role:physical_standby"})
	sender.AssertMetricNotTaggedWith(t, "Count", "dd.oracle.data_observability.query_executions", []string{"dd.internal.resource:test"})
	sender.AssertNotCalled(t, "Commit")
	var rows []any
	for _, call := range sender.Calls {
		if call.Method == "EventPlatformEvent" {
			var payload map[string]any
			require.NoError(t, json.Unmarshal(call.Arguments.Get(0).([]byte), &payload))
			assert.Equal(t, "db-host", payload["db_host"])
			rows = append(rows, payload["rows"])
		}
	}
	assert.Equal(t, []any{[]any{[]any{float64(1)}}, []any{[]any{float64(2)}}}, rows)
	require.NoError(t, dbMock.ExpectationsWereMet())
}
