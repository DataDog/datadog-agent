// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build oracle_test

package oracle

import (
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/benbjohnson/clock"
	"github.com/jmoiron/sqlx"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestBlockerFallbackFiresForEnqWaiter verifies that when a session is waiting on an
// enqueue lock (wait event begins with "enq") and blocking_session is not populated
// by Oracle, the agent queries v$lock/gv$lock and populates BlockingSession/BlockingInstance.
func TestBlockerFallbackFiresForEnqWaiter(t *testing.T) {
	db, dbMock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	c, _ := newDbDoesNotExistCheck(t, "", "")
	c.db = sqlx.NewDb(db, "sqlmock")
	c.clock = clock.New()
	c.sqlSubstringLength = 4000
	c.multitenant = true
	c.config.QuerySamples.ForceDirectQuery = true
	c.config.QuerySamples.IncludeAllSessions = true

	// Main activity query: one row waiting on an enqueue lock with no blocking session.
	activityRows := sqlmock.NewRows([]string{
		"NOW", "UTC_MS", "SID", "SERIAL#", "STATUS", "OP_FLAGS", "EVENT",
	}).AddRow(
		"2024-01-01 00:00:00", 0.0, 100, 1, "ACTIVE", 0,
		"enq: TX - row lock contention",
	)
	dbMock.ExpectQuery("DD_ACTIVITY_SAMPLING").WillReturnRows(activityRows)

	// Fallback query: returns the blocker (instance 2, sid 200).
	fallbackRows := sqlmock.NewRows([]string{"BLOCKER_INST_ID", "BLOCKER_SID"}).
		AddRow(2, 200)
	dbMock.ExpectQuery("blocker_inst_id").WillReturnRows(fallbackRows)

	err = c.SampleSession()
	require.NoError(t, err)
	require.Len(t, c.lastOracleActivityRows, 1)
	assert.Equal(t, uint64(200), c.lastOracleActivityRows[0].BlockingSession)
	assert.Equal(t, uint64(2), c.lastOracleActivityRows[0].BlockingInstance)
	assert.NoError(t, dbMock.ExpectationsWereMet())
}

// TestBlockerFallbackSkippedForNonEnqSession verifies that the v$lock/gv$lock fallback
// is NOT executed for sessions that are not waiting on an enqueue lock (e.g. CPU sessions
// or sessions with other wait event classes).
func TestBlockerFallbackSkippedForNonEnqSession(t *testing.T) {
	db, dbMock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	c, _ := newDbDoesNotExistCheck(t, "", "")
	c.db = sqlx.NewDb(db, "sqlmock")
	c.clock = clock.New()
	c.sqlSubstringLength = 4000
	c.config.QuerySamples.ForceDirectQuery = true
	c.config.QuerySamples.IncludeAllSessions = true

	// Main activity query: one row on CPU with no blocking session.
	// A non-enq wait event should never trigger the v$lock fallback.
	activityRows := sqlmock.NewRows([]string{
		"NOW", "UTC_MS", "SID", "SERIAL#", "STATUS", "OP_FLAGS", "EVENT",
	}).AddRow(
		"2024-01-01 00:00:00", 0.0, 100, 1, "ACTIVE", 0,
		"CPU",
	)
	dbMock.ExpectQuery("DD_ACTIVITY_SAMPLING").WillReturnRows(activityRows)
	// No fallback expectation registered; an unexpected query would cause a test error.

	err = c.SampleSession()
	require.NoError(t, err)
	require.Len(t, c.lastOracleActivityRows, 1)
	assert.Equal(t, uint64(0), c.lastOracleActivityRows[0].BlockingSession)
	// Verify all (and only) the expected queries executed — no surprise fallback query.
	assert.NoError(t, dbMock.ExpectationsWereMet())
}

// TestBlockerFallbackDisabledByConfig verifies that setting
// BlockingSessionFallbackEnabled=false suppresses the v$lock/gv$lock fallback entirely,
// even for sessions that would otherwise qualify (enq wait, no blocking session).
func TestBlockerFallbackDisabledByConfig(t *testing.T) {
	db, dbMock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	c, _ := newDbDoesNotExistCheck(t, "", "")
	c.db = sqlx.NewDb(db, "sqlmock")
	c.clock = clock.New()
	c.sqlSubstringLength = 4000
	c.config.QuerySamples.ForceDirectQuery = true
	c.config.QuerySamples.IncludeAllSessions = true
	c.config.QuerySamples.BlockingSessionFallbackEnabled = false

	// Main activity query: enq waiter with no blocking session — would trigger the fallback
	// if it were enabled.
	activityRows := sqlmock.NewRows([]string{
		"NOW", "UTC_MS", "SID", "SERIAL#", "STATUS", "OP_FLAGS", "EVENT",
	}).AddRow(
		"2024-01-01 00:00:00", 0.0, 100, 1, "ACTIVE", 0,
		"enq: TX - row lock contention",
	)
	dbMock.ExpectQuery("DD_ACTIVITY_SAMPLING").WillReturnRows(activityRows)
	// No fallback expectation — the flag disables it.

	err = c.SampleSession()
	require.NoError(t, err)
	require.Len(t, c.lastOracleActivityRows, 1)
	assert.Equal(t, uint64(0), c.lastOracleActivityRows[0].BlockingSession)
	assert.NoError(t, dbMock.ExpectationsWereMet())
}

// TestBlockerFallbackSkippedForNonCDB verifies that the fallback is NOT executed on
// non-CDB (non-multitenant) databases, since v$lock/gv$lock lack the CON_ID column
// on pre-12c / non-CDB deployments and the bug only manifests on newer CDB releases.
func TestBlockerFallbackSkippedForNonCDB(t *testing.T) {
	db, dbMock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	c, _ := newDbDoesNotExistCheck(t, "", "")
	c.db = sqlx.NewDb(db, "sqlmock")
	c.clock = clock.New()
	c.multitenant = false // non-CDB: fallback must not fire
	c.sqlSubstringLength = 4000
	c.config.QuerySamples.ForceDirectQuery = true
	c.config.QuerySamples.IncludeAllSessions = true

	activityRows := sqlmock.NewRows([]string{
		"NOW", "UTC_MS", "SID", "SERIAL#", "STATUS", "OP_FLAGS", "EVENT",
	}).AddRow(
		"2024-01-01 00:00:00", 0.0, 100, 1, "ACTIVE", 0,
		"enq: TX - row lock contention",
	)
	dbMock.ExpectQuery("DD_ACTIVITY_SAMPLING").WillReturnRows(activityRows)

	err = c.SampleSession()
	require.NoError(t, err)
	require.Len(t, c.lastOracleActivityRows, 1)
	assert.Equal(t, uint64(0), c.lastOracleActivityRows[0].BlockingSession)
	assert.NoError(t, dbMock.ExpectationsWereMet())
}
