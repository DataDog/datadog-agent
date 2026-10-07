// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build oracle

package oracle

import (
	"context"
	"database/sql/driver"
	"errors"
	"fmt"
	"regexp"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/jmoiron/sqlx"
	"github.com/sijms/go-ora/v2/network"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/oracle/config"
)

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
