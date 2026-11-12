// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build oracle_test

package oracle

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/oracle/config"
	"github.com/stretchr/testify/require"
)

func TestCancelDuringOracleFetch(t *testing.T) {
	c, _ := newDefaultCheck(t, "only_custom_queries: true", "")
	t.Cleanup(func() { c.Cancel(); <-c.lifecycle.done })
	var version string
	require.NoError(t, c.db.Get(&version, "SELECT version FROM v$instance"))
	major, err := strconv.Atoi(strings.Split(version, ".")[0])
	require.NoError(t, err)
	if major < 18 {
		t.Skip("this cancellation fixture requires DBMS_SESSION.SLEEP (Oracle 18+)")
	}
	c.initialized, c.connectedToPdb = true, true
	c.config.QueryTimeout = 60
	observer, err := c.Connect()
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, observer.Close()) })
	marker := fmt.Sprintf("WITH /* DD cancel fetch %d */", time.Now().UnixNano())
	query := marker + ` FUNCTION pause_at_row(n NUMBER) RETURN NUMBER IS
BEGIN
  IF n=1000 THEN DBMS_SESSION.SLEEP(30); END IF;
  RETURN n;
END;
SELECT pause_at_row(level) value FROM dual CONNECT BY level<=1500`
	c.config.InstanceConfig.CustomQueries = []config.CustomQuery{{
		Query:   query,
		Columns: []config.CustomQueryColumns{{Name: "value", Type: "gauge"}},
	}}
	done := make(chan error, 1)
	go func() { done <- c.Run() }()
	// Observe the server wait so cancellation happens during a later fetch,
	// after QueryContext has already returned the first batch of rows.
	require.Eventually(t, func() bool {
		var count int
		err := observer.Get(&count, `SELECT COUNT(*) FROM v$session s WHERE s.event='PL/SQL lock timer'
AND s.sql_id IN (SELECT sql_id FROM v$sql WHERE sql_text LIKE :1)`, marker+"%")
		return err == nil && count == 1
	}, 15*time.Second, 50*time.Millisecond)
	c.Cancel()
	select {
	case err := <-done:
		require.ErrorIs(t, err, context.Canceled)
	case <-time.After(15 * time.Second):
		t.Fatal("cancellation did not interrupt the 30-second fetch")
	}
	<-c.lifecycle.done
	require.Nil(t, c.db)
	require.Nil(t, c.dbCustomQueries)
}
