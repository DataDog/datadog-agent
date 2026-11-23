// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build oracle_test

package oracle

import (
	"context"
	"fmt"
	"os"
	"runtime"
	"runtime/debug"
	"strings"
	"testing"
	"time"

	"github.com/DataDog/datadog-agent/pkg/aggregator/mocksender"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/oracle/config"
	"github.com/jmoiron/sqlx"
	goora "github.com/sijms/go-ora/v2"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

// Opt in because these fixtures require Oracle 18+ and an independent observer
// with access to v$session. No observation queries run in production.
func deadlineLiveCheck(t *testing.T) (*Check, *sqlx.DB, *mocksender.MockSender) {
	t.Helper()
	if os.Getenv("ORACLE_DEADLINE_LIVE") != "1" {
		t.Skip("set ORACLE_DEADLINE_LIVE=1 and ORACLE_TEST_OBSERVER_USER/PASSWORD")
	}
	c, sender := operationCheck(t)
	c.config.ConnectionConfig = getConnectData(t, useDefaultUser)
	c.config.QueryTimeout = 1
	user, password := os.Getenv("ORACLE_TEST_OBSERVER_USER"), os.Getenv("ORACLE_TEST_OBSERVER_PASSWORD")
	require.NotEmpty(t, user)
	require.NotEmpty(t, password)
	observer, err := sqlx.Connect("oracle", goora.BuildUrl(c.config.Server, c.config.Port, c.config.ServiceName, user, password, map[string]string{"TIMEOUT": "10"}))
	require.NoError(t, err)
	observer.SetMaxOpenConns(1)
	t.Cleanup(func() { require.NoError(t, observer.Close()) })
	return c, observer, sender
}

func deadlineSession(t *testing.T, c *Check) string {
	t.Helper()
	var identity string
	require.NoError(t, getWrapper(c, &identity, `SELECT sid||','||serial# FROM v$session WHERE sid=SYS_CONTEXT('USERENV','SID')`))
	return identity
}

func deadlineObserveCompletion(t *testing.T, observer *sqlx.DB, identity string, start time.Time) {
	t.Helper()
	var count int
	require.NoError(t, observer.Get(&count, `SELECT COUNT(*) FROM v$session WHERE sid||','||serial#=:1`, identity))
	t.Logf("client returned at %s; exact old session %s present=%d (socket interruption does not prove Oracle completion)", time.Since(start), identity, count)
	require.Eventually(t, func() bool {
		err := observer.Get(&count, `SELECT COUNT(*) FROM v$session WHERE sid||','||serial#=:1`, identity)
		return err == nil && count == 0
	}, 15*time.Second, 50*time.Millisecond)
	t.Logf("observer confirmed exact old session gone by %s", time.Since(start))
}

func deadlineSlowQuery(fetch bool) string {
	expression, source := "pause_at_row(1000)", "dual"
	if fetch {
		expression, source = "pause_at_row(level)", "dual CONNECT BY level<=1500"
	}
	return `WITH FUNCTION pause_at_row(n NUMBER) RETURN NUMBER IS
BEGIN IF n=1000 THEN DBMS_SESSION.SLEEP(6); END IF; RETURN n; END;
SELECT ` + expression + ` value FROM ` + source
}

func TestOperationLiveExecutionAndRecovery(t *testing.T) {
	for _, failedReconnect := range []bool{false, true} {
		t.Run(fmt.Sprint("failed_reconnect=", failedReconnect), func(t *testing.T) {
			c, observer, _ := deadlineLiveCheck(t)
			identity := deadlineSession(t, c)
			service := c.config.ServiceName
			if failedReconnect {
				c.config.ServiceName = "SDBM3098_DOES_NOT_EXIST"
			}
			var value int
			start := time.Now()
			err := getWrapper(c, &value, deadlineSlowQuery(false))
			elapsed := time.Since(start)
			require.ErrorIs(t, err, context.DeadlineExceeded)
			require.Less(t, elapsed, 4*time.Second, "must interrupt the 6-second query")
			if failedReconnect {
				require.Nil(t, c.db)
			}
			c.config.ServiceName = service
			require.NoError(t, getWrapper(c, &value, "SELECT 42 FROM dual"))
			require.Equal(t, 42, value, "must not receive the abandoned query response")
			require.NotEqual(t, identity, deadlineSession(t, c))
			t.Logf("execution deadline=1s socket=2s client_elapsed=%s later_query=42 failed_reconnect=%t", elapsed, failedReconnect)
			deadlineObserveCompletion(t, observer, identity, start)
		})
	}
}

func TestOperationLiveLaterFetch(t *testing.T) {
	c, observer, sender := deadlineLiveCheck(t)
	identity := deadlineSession(t, c)
	// First exercise Next directly to prove QueryContext returned before the wait.
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	start := time.Now()
	rows, err := c.db.QueryxContext(ctx, deadlineSlowQuery(true))
	require.NoError(t, err)
	delivered := 0
	for rows.Next() {
		var value int
		require.NoError(t, rows.Scan(&value))
		delivered++
	}
	err = operationError(ctx, rows.Err())
	closeErr := rows.Close()
	require.ErrorIs(t, err, context.DeadlineExceeded)
	require.Greater(t, delivered, 0)
	require.Less(t, delivered, 1500)
	require.Less(t, time.Since(start), 4*time.Second)
	t.Logf("later fetch deadline: delivered=%d elapsed=%s rows_error=%v close_error=%v", delivered, time.Since(start), err, closeErr)
	require.Error(t, handleError(c, &c.db, err))
	var value int
	require.NoError(t, getWrapper(c, &value, "SELECT 42 FROM dual"))
	require.Equal(t, 42, value)
	deadlineObserveCompletion(t, observer, identity, start)
	// Then exercise the actual custom-query path, including later work in Run.
	c.dbCustomQueries, err = c.Connect()
	require.NoError(t, err)
	var customIdentity string
	require.NoError(t, c.dbCustomQueries.Get(&customIdentity, `SELECT sid||','||serial# FROM v$session WHERE sid=SYS_CONTEXT('USERENV','SID')`))
	c.config.InstanceConfig.CustomQueries = []config.CustomQuery{
		{Query: deadlineSlowQuery(true), MetricPrefix: "deadline_failed", Columns: []config.CustomQueryColumns{{Name: "value", Type: "gauge"}}},
		{Query: "SELECT 42 value FROM dual", MetricPrefix: "deadline_later", Columns: []config.CustomQueryColumns{{Name: "value", Type: "gauge"}}},
	}
	// A failed fetch emits no partial metrics; the later query still runs.
	start = time.Now()
	require.ErrorIs(t, c.Run(), context.DeadlineExceeded)
	sender.AssertCalled(t, "Gauge", "deadline_later.value", float64(42), mock.Anything, mock.Anything)
	sender.AssertNotCalled(t, "Gauge", "deadline_failed.value", mock.Anything, mock.Anything, mock.Anything)
	require.Less(t, time.Since(start), 4*time.Second)
	require.NotNil(t, c.dbCustomQueries)
	require.NoError(t, c.dbCustomQueries.Get(&value, "SELECT 42 FROM dual"))
	require.Equal(t, 42, value)
	t.Logf("custom-query Run returned after %s with its recovered pool usable", time.Since(start))
	deadlineObserveCompletion(t, observer, customIdentity, start)
}

func TestOperationLiveFailedOpens(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("socket counts require /proc/self/fd")
	}
	c, _, _ := deadlineLiveCheck(t)
	c.config.QueryTimeout = 5
	c.config.Username = "SDBM3098_DOES_NOT_EXIST"
	sockets := func() int {
		entries, err := os.ReadDir("/proc/self/fd")
		require.NoError(t, err)
		count := 0
		for _, entry := range entries {
			link, _ := os.Readlink("/proc/self/fd/" + entry.Name())
			if strings.HasPrefix(link, "socket:") {
				count++
			}
		}
		return count
	}
	old := debug.SetGCPercent(-1)
	defer debug.SetGCPercent(old)
	baseline := sockets()
	for i := 0; i < 8; i++ {
		db, err := c.Connect()
		require.Error(t, err)
		require.Nil(t, db)
	}
	require.Equal(t, baseline, sockets())
	t.Logf("eight rejected logins with GC disabled sockets_before=%d sockets_after=%d", baseline, sockets())
}

func TestOperationLiveFetchErrorRecovery(t *testing.T) {
	c, observer, sender := deadlineLiveCheck(t)
	c.config.QueryTimeout = 5
	var err error
	c.dbCustomQueries, err = c.Connect()
	require.NoError(t, err)
	old := c.dbCustomQueries
	var identity string
	require.NoError(t, old.Get(&identity, `SELECT sid||','||serial# FROM v$session WHERE sid=SYS_CONTEXT('USERENV','SID')`))
	query := `WITH FUNCTION fail_at_row(n NUMBER) RETURN NUMBER IS
 e EXCEPTION; PRAGMA EXCEPTION_INIT(e,-6413);
 BEGIN IF n=1000 THEN RAISE e; END IF; RETURN n; END;
 SELECT fail_at_row(level) value FROM dual CONNECT BY level<=1500`
	c.config.InstanceConfig.CustomQueries = []config.CustomQuery{
		{Query: query, MetricPrefix: "fetch_failed", Columns: []config.CustomQueryColumns{{Name: "value", Type: "gauge"}}},
		{Query: "SELECT 42 value FROM dual", MetricPrefix: "fetch_later", Columns: []config.CustomQueryColumns{{Name: "value", Type: "gauge"}}},
	}
	start := time.Now()
	require.ErrorContains(t, c.CustomQueries(), "ORA-06413")
	require.NotNil(t, c.dbCustomQueries)
	require.NotSame(t, old, c.dbCustomQueries)
	sender.AssertCalled(t, "Gauge", "fetch_later.value", float64(42), mock.Anything, mock.Anything)
	sender.AssertNotCalled(t, "Gauge", "fetch_failed.value", mock.Anything, mock.Anything, mock.Anything)
	deadlineObserveCompletion(t, observer, identity, start)
	t.Log("injected server ORA-06413 during later fetch reached recovery, emitted no partial metrics, and the later custom query returned 42")
}
