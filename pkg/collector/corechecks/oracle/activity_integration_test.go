// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build oracle_test

package oracle

import (
	"context"
	"database/sql"
	"fmt"
	"strings"

	"testing"
	"time"

	"github.com/benbjohnson/clock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLongMultibyteQuery(t *testing.T) {
	c, _ := newDefaultCheck(t, "", "")
	defer c.Teardown()

	c.dbmEnabled = true

	// This is to ensure that query samples return rows
	c.config.QuerySamples.IncludeAllSessions = true

	c.statementsLastRun = time.Now().Add(-48 * time.Hour)
	err := c.Run()
	assert.NoError(t, err, "check run")

	// We're just testing the agent's ability to handle a SQL query whose bytes exceed 4k
	// when there's less than 4k characters. Actual result doesn't matter so long as it eventually collects
	// the query
	largeMultibyteString := strings.Repeat("안녕하세요", 200)
	filter := fmt.Sprintf("user='%s'", largeMultibyteString)
	andClause := strings.Repeat(fmt.Sprintf(" and %s", filter), 100)
	filter = filter + andClause
	longQuery := fmt.Sprintf("select 14 from dual where %s", filter)

	// we aren't scanning rows to force the session keep the cursor open, so
	// the test query sql_id will be stored in prev_sql_id
	rows, err := c.db.Query(longQuery)
	defer rows.Close()
	require.NoError(t, err, "failed to execute the test query")

	assert.Equal(t, 4000, c.sqlSubstringLength, "sql substring length should be 4000")
	found := false
sessions:
	for i := 0; i < 10; i++ {
		// Run checks while statement size ramps down until the test query is found
		err = c.SampleSession()
		require.NoError(t, err, "activity sample failed")
		for _, r := range c.lastOracleActivityRows {
			if r.SQLID == "0h2313hkrjx1t" {
				found = true
				break sessions
			}
		}
	}
	assert.True(t, found, "test query not found in samples")
	assert.Equal(t, 1000, c.sqlSubstringLength, "sql substring length should be 1000")
}

func TestCurrentTime(t *testing.T) {
	c, _ := newDefaultCheck(t, "", "")
	defer c.Teardown()

	c.dbmEnabled = true
	// This is to ensure that query samples return rows
	c.config.QuerySamples.IncludeAllSessions = true

	err := c.Run()
	assert.NoError(t, err, "check run")
	mockClock := clock.NewMock()
	mockClock.Now().UTC()
	c.clock = mockClock

	c.config.QuerySamples.ForceDirectQuery = true
	for _, directQuery := range []bool{true, false} {
		c.config.QuerySamples.ForceDirectQuery = directQuery
		err = c.SampleSession()
		require.NoError(t, err, "activity sample failed")

		fetchedNow := c.lastOracleActivityRows[0].Now
		fetchedNowTime, err := time.Parse(time.RFC3339, fetchedNow)
		require.NoErrorf(t, err, "couldn't parse now from activity sample for direct query: %t", directQuery)

		assert.WithinDuration(t, time.Now(), fetchedNowTime, 10*time.Second)
	}
}

func TestIdleWaits(t *testing.T) {
	c, _ := newDefaultCheck(t, "", "")
	defer c.Teardown()

	c.dbmEnabled = true

	err := c.Run()
	assert.NoError(t, err, "check run")

	err = c.SampleSession()
	require.NoError(t, err, "activity sample failed")

	for _, r := range c.lastOracleActivityRows {
		assert.NotEqual(t, r.WaitEventClass, "Idle", "Idle wait class not sampled")
	}
}

func TestActiveSessionHistory(t *testing.T) {
	c, _ := newDefaultCheck(t, "", "")
	defer c.Teardown()

	var err error
	var count uint64
	getWrapper(&c, &count, "SELECT count(*) FROM V$VERSION WHERE BANNER LIKE '%Enterprise%Edition%'")
	if count == 0 {
		t.Skip("Active Session History is only available in Oracle Enterprise Edition")
	}
	c.dbmEnabled = true
	c.config.QuerySamples.ActiveSessionHistory = true

	err = c.Run()
	assert.NoError(t, err, "check run")

	err = c.SampleSession()
	require.NoError(t, err, "activity sample failed")
	prevSamplId := c.lastSampleID

	conn, err := getSysConnection(t)
	require.NoError(t, err)

	tx, err := conn.Begin()
	require.NoError(t, err)
	_, err = tx.Exec("INSERT INTO t VALUES (1)")
	require.NoError(t, err)

	longRunningStatement := `DECLARE
  PRAGMA AUTONOMOUS_TRANSACTION;
  l NUMBER;
BEGIN
  execute immediate 'ALTER SESSION SET DDL_LOCK_TIMEOUT = 3';
  execute immediate 'alter table t add n2 number';
  ROLLBACK;
END;`
	_, err = tx.Exec(longRunningStatement)
	require.Error(t, err)

	err = c.SampleSession()
	require.NoError(t, err, "activity sample failed")

	assert.Greater(t, c.lastSampleID, prevSamplId, "sample id should have increased")

	for _, r := range c.lastOracleActivityRows {
		assert.True(t, isValidSessionType(r.Type), "invalid session type: %s", r.Type)
	}
}

// TestActivityInPLSQL samples a session blocked in DBMS_LOCK.REQUEST, called from an anonymous block.
// DBMS_LOCK is the PL/SQL entry object of that session. The test holds the user lock in another
// session and releases it once both query modes have sampled the waiting session.
func TestActivityInPLSQL(t *testing.T) {
	c, _ := newDefaultCheck(t, "", "")
	defer c.Teardown()

	c.dbmEnabled = true

	err := c.Run()
	require.NoError(t, err, "check run")

	db, err := getSysConnection(t)
	require.NoError(t, err)
	defer db.Close()

	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()

	// DBMS_LOCK user lock ids range from 0 to 1073741823.
	const lockID = 1073741001
	const module = "dd_in_plsql_test"

	holder, err := db.Conn(ctx)
	require.NoError(t, err)
	defer holder.Close()
	var holderStatus int
	_, err = holder.ExecContext(ctx, fmt.Sprintf(`BEGIN :1 := DBMS_LOCK.REQUEST(%d, DBMS_LOCK.X_MODE, 0); END;`, lockID), sql.Out{Dest: &holderStatus})
	require.NoError(t, err)
	require.Equal(t, 0, holderStatus, "the holder session must get the user lock")
	released := false
	release := func() {
		if !released {
			released = true
			_, err := holder.ExecContext(ctx, fmt.Sprintf(`BEGIN :1 := DBMS_LOCK.RELEASE(%d); END;`, lockID), sql.Out{Dest: &holderStatus})
			assert.NoError(t, err, "failed to release the user lock")
		}
	}
	defer release()

	waiter, err := db.Conn(ctx)
	require.NoError(t, err)
	defer waiter.Close()
	waiterDone := make(chan error, 1)
	go func() {
		// The request timeout bounds the wait if the holder never releases the lock.
		_, err := waiter.ExecContext(ctx, fmt.Sprintf(`DECLARE
  status INTEGER;
BEGIN
  DBMS_APPLICATION_INFO.SET_MODULE('%s', NULL);
  status := DBMS_LOCK.REQUEST(%d, DBMS_LOCK.X_MODE, 30);
  status := DBMS_LOCK.RELEASE(%d);
END;`, module, lockID, lockID))
		waiterDone <- err
	}()

	for _, directQuery := range []bool{true, false} {
		c.config.QuerySamples.ForceDirectQuery = directQuery
		var plsqlRow *OracleActivityRow
		for i := 0; i < 20 && plsqlRow == nil; i++ {
			require.NoError(t, c.SampleSession(), "activity sample failed")
			for _, r := range c.lastOracleActivityRows {
				if r.PreviousSQL {
					assert.Nilf(t, r.InPLSQL, "in_plsql must be unknown for the previous SQL of sid %d", r.SessionID)
				}
				if r.Module == module && !r.PreviousSQL {
					plsqlRow = &r
				}
			}
			if plsqlRow == nil {
				// The waiter session may not have reached DBMS_LOCK.REQUEST yet.
				time.Sleep(200 * time.Millisecond)
			}
		}
		require.NotNilf(t, plsqlRow, "session in PL/SQL not sampled, direct query: %t", directQuery)
		assert.NotNilf(t, plsqlRow.PLSQLEntryObjectID, "direct query: %t", directQuery)
		require.NotNilf(t, plsqlRow.InPLSQL, "direct query: %t", directQuery)
		assert.Truef(t, *plsqlRow.InPLSQL, "direct query: %t", directQuery)
	}

	release()
	select {
	case err := <-waiterDone:
		require.NoError(t, err)
	case <-ctx.Done():
		t.Fatal("the PL/SQL session did not finish after the user lock was released")
	}
}

func isValidSessionType(sessionType string) bool {
	return sessionType == "USER" || sessionType == "BACKGROUND"
}
