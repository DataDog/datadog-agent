// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build oracle_test

package oracle

import (
	"fmt"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/benbjohnson/clock"
	"github.com/jmoiron/sqlx"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/aggregator/mocksender"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/oracle/config"
)

func TestCustomQueries(t *testing.T) {
	db, dbMock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("an error '%s' was not expected when opening a stub database connection", err)
	}
	defer db.Close()

	dbMock.ExpectExec("alter.*").WillReturnResult(sqlmock.NewResult(1, 1))

	rows := sqlmock.NewRows([]string{"c1", "c2"}).
		AddRow(1, "A").
		AddRow(2, "B")
	c1 := config.CustomQueryColumns{"c1", "gauge"}
	c2 := config.CustomQueryColumns{"c2", "tag"}
	columns := []config.CustomQueryColumns{c1, c2}
	dbMock.ExpectQuery("SELECT c1, c2 FROM t").WillReturnRows(rows)
	q := config.CustomQuery{
		MetricPrefix: "oracle.custom_query.test",
		Query:        "SELECT c1, c2 FROM t",
		Columns:      columns,
	}

	chk, sender := newDbDoesNotExistCheck(t, "", "")
	chk.Run()

	sender.SetupAcceptAll()
	sender.On("Gauge", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return()

	chk.config.InstanceConfig.CustomQueries = []config.CustomQuery{q}
	chk.dbCustomQueries = sqlx.NewDb(db, "sqlmock")

	err = chk.CustomQueries()
	assert.NoError(t, err, "failed to execute custom query")
	sender.AssertMetricTaggedWith(t, "Gauge", "oracle.custom_query.test.c1", []string{"c2:A"})
}

func TestCustomQueriesRespectIndependentCollectionIntervals(t *testing.T) {
	db, dbMock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	shortInterval := int64(30)
	longInterval := int64(60)
	columns := []config.CustomQueryColumns{{Name: "value", Type: "gauge"}}
	queries := []config.CustomQuery{
		{
			MetricPrefix:       "oracle.short_interval",
			Query:              "SELECT 1 FROM short_interval",
			Columns:            columns,
			CollectionInterval: &shortInterval,
		},
		{
			MetricPrefix:       "oracle.long_interval",
			Query:              "SELECT 1 FROM long_interval",
			Columns:            columns,
			CollectionInterval: &longInterval,
		},
	}

	expectQuery := func(query string) {
		dbMock.ExpectExec("alter.*").WillReturnResult(sqlmock.NewResult(1, 1))
		dbMock.ExpectQuery(query).WillReturnRows(sqlmock.NewRows([]string{"value"}).AddRow(1))
	}

	// Both queries run immediately, the short query runs after 30 seconds, and
	// both run after 60 seconds.
	expectQuery("SELECT 1 FROM short_interval")
	expectQuery("SELECT 1 FROM long_interval")
	expectQuery("SELECT 1 FROM short_interval")
	expectQuery("SELECT 1 FROM short_interval")
	expectQuery("SELECT 1 FROM long_interval")

	chk, sender := newDbDoesNotExistCheck(t, "", "")
	sender.SetupAcceptAll()
	sender.On("Gauge", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return()
	sender.On("Commit").Return()
	chk.config.InstanceConfig.CustomQueries = queries
	chk.dbCustomQueries = sqlx.NewDb(db, "sqlmock")
	mockClock := clock.NewMock()
	chk.clock = mockClock

	require.NoError(t, chk.CustomQueries())
	mockClock.Add(29 * time.Second)
	require.NoError(t, chk.CustomQueries())
	mockClock.Add(time.Second)
	require.NoError(t, chk.CustomQueries())
	mockClock.Add(30 * time.Second)
	require.NoError(t, chk.CustomQueries())

	assert.NoError(t, dbMock.ExpectationsWereMet())
	sender.AssertNumberOfCalls(t, "Gauge", 5)
}

func TestFailedCustomQueryAttemptConsumesCollectionInterval(t *testing.T) {
	db, dbMock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	collectionInterval := int64(30)
	query := config.CustomQuery{
		MetricPrefix:       "oracle.failed_interval",
		Query:              "SELECT 1 FROM failed_interval",
		Columns:            []config.CustomQueryColumns{{Name: "value", Type: "gauge"}},
		CollectionInterval: &collectionInterval,
	}

	dbMock.ExpectExec("alter.*").WillReturnResult(sqlmock.NewResult(1, 1))
	dbMock.ExpectQuery(query.Query).WillReturnError(fmt.Errorf("query failed"))
	dbMock.ExpectExec("alter.*").WillReturnResult(sqlmock.NewResult(1, 1))
	dbMock.ExpectQuery(query.Query).WillReturnRows(sqlmock.NewRows([]string{"value"}).AddRow(1))

	chk, sender := newDbDoesNotExistCheck(t, "", "")
	sender.SetupAcceptAll()
	sender.On("Gauge", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return()
	sender.On("Commit").Return()
	chk.config.InstanceConfig.CustomQueries = []config.CustomQuery{query}
	chk.dbCustomQueries = sqlx.NewDb(db, "sqlmock")
	mockClock := clock.NewMock()
	chk.clock = mockClock

	require.Error(t, chk.CustomQueries())
	mockClock.Add(29 * time.Second)
	require.NoError(t, chk.CustomQueries())
	mockClock.Add(time.Second)
	require.NoError(t, chk.CustomQueries())

	assert.NoError(t, dbMock.ExpectationsWereMet())
	sender.AssertNumberOfCalls(t, "Gauge", 1)
}

func TestMultipleCustomQueriesNoMetricLeak(t *testing.T) {
	db, dbMock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	// First query returns 2 rows
	dbMock.ExpectExec("alter.*").WillReturnResult(sqlmock.NewResult(1, 1))
	rows1 := sqlmock.NewRows([]string{"val", "tag"}).
		AddRow(10, "A").
		AddRow(20, "B")
	dbMock.ExpectQuery("SELECT val, tag FROM t1").WillReturnRows(rows1)

	// Second query returns 1 row
	dbMock.ExpectExec("alter.*").WillReturnResult(sqlmock.NewResult(1, 1))
	rows2 := sqlmock.NewRows([]string{"val", "tag"}).
		AddRow(99, "Z")
	dbMock.ExpectQuery("SELECT val, tag FROM t2").WillReturnRows(rows2)

	columns := []config.CustomQueryColumns{
		{Name: "val", Type: "gauge"},
		{Name: "tag", Type: "tag"},
	}
	q1 := config.CustomQuery{
		MetricPrefix: "oracle.q1",
		Query:        "SELECT val, tag FROM t1",
		Columns:      columns,
	}
	q2 := config.CustomQuery{
		MetricPrefix: "oracle.q2",
		Query:        "SELECT val, tag FROM t2",
		Columns:      columns,
	}

	chk, sender := newDbDoesNotExistCheck(t, "", "")
	chk.Run()

	sender.SetupAcceptAll()
	sender.On("Gauge", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return()
	sender.On("Commit").Return()

	chk.config.InstanceConfig.CustomQueries = []config.CustomQuery{q1, q2}
	chk.dbCustomQueries = sqlx.NewDb(db, "sqlmock")

	err = chk.CustomQueries()
	assert.NoError(t, err)

	// q1 should send 2 Gauge calls, q2 should send 1 = 3 total.
	// Before the fix, metricRows accumulated across queries so q2 would
	// re-send q1's metrics, resulting in 5 Gauge calls instead of 3.
	sender.AssertNumberOfCalls(t, "Gauge", 3)
	sender.AssertMetricTaggedWith(t, "Gauge", "oracle.q1.val", []string{"tag:A"})
	sender.AssertMetricTaggedWith(t, "Gauge", "oracle.q1.val", []string{"tag:B"})
	sender.AssertMetricTaggedWith(t, "Gauge", "oracle.q2.val", []string{"tag:Z"})
}

const customQueryTestConfig = `- metric_prefix: oracle.custom_query.test
  query: |
    select 'TAG1', 1.012345 value from dual
  columns:
    - name: name
      type: tag
    - name: value
      type: gauge
`

func assertCustomQuery(t *testing.T, c *Check, s *mocksender.MockSender) {
	err := c.Run()
	require.NoError(t, err)
	s.On("Gauge", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return()
	s.AssertMetricTaggedWith(t, "Gauge", "oracle.custom_query.test.value", []string{"name:TAG1"})
	s.AssertMetric(t, "Gauge", "oracle.custom_query.test.value", 1.012345, c.dbHostname, []string{})
}

func TestCustomQueriesWithoutIntervalRunEveryCheck(t *testing.T) {
	c, s := newDefaultCheck(t, `only_custom_queries: true
custom_queries:
  - metric_prefix: oracle.custom_query.every_check
    query: select 1 value from dual
    columns:
      - name: value
        type: gauge
`, "")
	defer c.Teardown()

	require.NoError(t, c.Run())
	require.NoError(t, c.Run())

	s.AssertNumberOfCalls(t, "Gauge", 2)
}

func TestFloat(t *testing.T) {
	c, s := newDefaultCheck(t, `custom_queries:
  - metric_prefix: oracle.custom_query.test
    query: |
      select 'TAG1', 1.012345 value from dual
    columns:
      - name: name
        type: tag
      - name: value
        type: gauge
`, "")
	defer c.Teardown()
	err := c.Run()
	require.NoError(t, err)
	assertConnectionCount(t, &c, expectedSessionsWithCustomQueries)
	s.On("Gauge", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return()
	s.AssertMetricTaggedWith(t, "Gauge", "oracle.custom_query.test.value", []string{"name:TAG1"})
	s.AssertMetric(t, "Gauge", "oracle.custom_query.test.value", 1.012345, c.dbHostname, []string{})
}

func TestConcatenateTypeErrorPreservesAccumulatedErrors(t *testing.T) {
	db, dbMock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	columns := []config.CustomQueryColumns{
		{Name: "val", Type: "gauge"},
	}

	// First query returns a non-numeric string for a gauge column
	dbMock.ExpectExec("alter.*").WillReturnResult(sqlmock.NewResult(1, 1))
	rows1 := sqlmock.NewRows([]string{"val"}).AddRow("not_a_number")
	dbMock.ExpectQuery("SELECT val FROM t1").WillReturnRows(rows1)

	// Second query also returns a non-numeric string
	dbMock.ExpectExec("alter.*").WillReturnResult(sqlmock.NewResult(1, 1))
	rows2 := sqlmock.NewRows([]string{"val"}).AddRow("also_bad")
	dbMock.ExpectQuery("SELECT val FROM t2").WillReturnRows(rows2)

	q1 := config.CustomQuery{
		MetricPrefix: "oracle.q1",
		Query:        "SELECT val FROM t1",
		Columns:      columns,
	}
	q2 := config.CustomQuery{
		MetricPrefix: "oracle.q2",
		Query:        "SELECT val FROM t2",
		Columns:      columns,
	}

	chk, sender := newDbDoesNotExistCheck(t, "", "")
	chk.Run()

	sender.SetupAcceptAll()
	sender.On("Commit").Return()

	chk.config.InstanceConfig.CustomQueries = []config.CustomQuery{q1, q2}
	chk.dbCustomQueries = sqlx.NewDb(db, "sqlmock")

	err = chk.CustomQueries()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "oracle.q1", "error from first query should be preserved")
	assert.Contains(t, err.Error(), "oracle.q2", "error from second query should be preserved")
}

func TestNullMetricColumnReportsError(t *testing.T) {
	db, dbMock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	dbMock.ExpectExec("alter.*").WillReturnResult(sqlmock.NewResult(1, 1))
	rows := sqlmock.NewRows([]string{"val", "tag"}).AddRow(nil, "A")
	dbMock.ExpectQuery("SELECT val, tag FROM t").WillReturnRows(rows)

	columns := []config.CustomQueryColumns{
		{Name: "val", Type: "gauge"},
		{Name: "tag", Type: "tag"},
	}
	q := config.CustomQuery{
		MetricPrefix: "oracle.nulltest",
		Query:        "SELECT val, tag FROM t",
		Columns:      columns,
	}

	chk, sender := newDbDoesNotExistCheck(t, "", "")
	chk.Run()

	sender.SetupAcceptAll()
	sender.On("Commit").Return()

	chk.config.InstanceConfig.CustomQueries = []config.CustomQuery{q}
	chk.dbCustomQueries = sqlx.NewDb(db, "sqlmock")

	err = chk.CustomQueries()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "NULL value for metric column val")
	sender.AssertNotCalled(t, "Gauge")
}

func TestColumnCountMismatchMoreColumns(t *testing.T) {
	db, dbMock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	dbMock.ExpectExec("alter.*").WillReturnResult(sqlmock.NewResult(1, 1))
	rows := sqlmock.NewRows([]string{"a", "b", "c"}).AddRow(1, 2, 3)
	dbMock.ExpectQuery("SELECT a, b, c FROM t").WillReturnRows(rows)

	q := config.CustomQuery{
		MetricPrefix: "oracle.mismatch",
		Query:        "SELECT a, b, c FROM t",
		Columns: []config.CustomQueryColumns{
			{Name: "a", Type: "gauge"},
		},
	}

	chk, sender := newDbDoesNotExistCheck(t, "", "")
	chk.Run()
	sender.SetupAcceptAll()
	sender.On("Commit").Return()

	chk.config.InstanceConfig.CustomQueries = []config.CustomQuery{q}
	chk.dbCustomQueries = sqlx.NewDb(db, "sqlmock")

	err = chk.CustomQueries()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "column count mismatch")
	assert.Contains(t, err.Error(), "3 columns but 1 mappings")
}

func TestColumnCountMismatchFewerColumns(t *testing.T) {
	db, dbMock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	dbMock.ExpectExec("alter.*").WillReturnResult(sqlmock.NewResult(1, 1))
	rows := sqlmock.NewRows([]string{"a"}).AddRow(1)
	dbMock.ExpectQuery("SELECT a FROM t").WillReturnRows(rows)

	q := config.CustomQuery{
		MetricPrefix: "oracle.mismatch",
		Query:        "SELECT a FROM t",
		Columns: []config.CustomQueryColumns{
			{Name: "a", Type: "gauge"},
			{Name: "b", Type: "gauge"},
			{Name: "c", Type: "tag"},
		},
	}

	chk, sender := newDbDoesNotExistCheck(t, "", "")
	chk.Run()
	sender.SetupAcceptAll()
	sender.On("Commit").Return()

	chk.config.InstanceConfig.CustomQueries = []config.CustomQuery{q}
	chk.dbCustomQueries = sqlx.NewDb(db, "sqlmock")

	err = chk.CustomQueries()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "column count mismatch")
	assert.Contains(t, err.Error(), "1 columns but 3 mappings")
}

func TestPdbNameInjectionPrevented(t *testing.T) {
	db, dbMock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherEqual))
	require.NoError(t, err)
	defer db.Close()

	maliciousPdb := `x; DROP TABLE users--`

	dbMock.ExpectExec(`alter session set container = "x; DROP TABLE users--"`).
		WillReturnResult(sqlmock.NewResult(1, 1))
	rows := sqlmock.NewRows([]string{"val"}).AddRow(1)
	dbMock.ExpectQuery("SELECT val FROM t").WillReturnRows(rows)

	q := config.CustomQuery{
		MetricPrefix: "oracle.injection",
		Pdb:          maliciousPdb,
		Query:        "SELECT val FROM t",
		Columns:      []config.CustomQueryColumns{{Name: "val", Type: "gauge"}},
	}

	chk, sender := newDbDoesNotExistCheck(t, "", "")
	chk.Run()
	sender.SetupAcceptAll()
	sender.On("Gauge", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return()
	sender.On("Commit").Return()

	chk.config.InstanceConfig.CustomQueries = []config.CustomQuery{q}
	chk.dbCustomQueries = sqlx.NewDb(db, "sqlmock")

	err = chk.CustomQueries()
	assert.NoError(t, err)
	assert.NoError(t, dbMock.ExpectationsWereMet())
}

func TestGlobalCustomQueries(t *testing.T) {
	globalCustomQueries := fmt.Sprintf("global_custom_queries:\n%s", customQueryTestConfig)
	c, s := newDefaultCheck(t, "", globalCustomQueries)
	defer c.Teardown()
	assertCustomQuery(t, &c, s)
}
