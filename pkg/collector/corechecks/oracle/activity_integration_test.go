// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build oracle_test

package oracle

import (
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
