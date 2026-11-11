// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build oracle_test

package oracle

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	_ "github.com/godror/godror"
)

func TestGetFullSqlText(t *testing.T) {
	c, _ := newDefaultCheck(t, "", "")
	defer c.Teardown()

	c.dbmEnabled = false

	err := c.Run()
	assert.NoError(t, err, "check run")
	assertConnectionCount(t, &c, expectedSessionsDefault)

	var SQLStatement string
	err = getFullSQLText(&c, &SQLStatement, "sql_id", "A")
	assert.NoError(t, err, "no rows returned an error")
}

func TestFullSQLTextReusesPool(t *testing.T) {
	c, _ := newDefaultCheck(t, "", "")
	t.Cleanup(c.Teardown)
	var err error
	c.db, err = c.Connect()
	require.NoError(t, err)
	c.db.SetMaxOpenConns(1)
	c.db.SetMaxIdleConns(1)

	// Keep the source cursor open so it cannot age out during repeated reads.
	sourceSQL := "SELECT /* DD fulltext pool " + strings.Repeat("long_sql_", 2000) + " */ 1 FROM dual"
	stmt, err := c.db.Preparex(sourceSQL)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, stmt.Close()) })
	var one int
	require.NoError(t, stmt.Get(&one))
	var sqlID, originalSID string
	require.NoError(t, c.db.Get(&sqlID, "SELECT sql_id FROM v$sql WHERE sql_text LIKE :1 AND rownum=1", "SELECT /* DD fulltext pool %"))
	require.NoError(t, c.db.Get(&originalSID, "SELECT SYS_CONTEXT('USERENV', 'SID') FROM dual"))

	for i := 0; i < 25; i++ {
		var fullText string
		require.NoError(t, getFullSQLText(&c, &fullText, "sql_id", sqlID))
		require.Equal(t, sourceSQL, fullText)
	}
	var currentSID string
	require.NoError(t, c.db.Get(&currentSID, "SELECT SYS_CONTEXT('USERENV', 'SID') FROM dual"))
	require.Equal(t, originalSID, currentSID)
	require.Equal(t, 1, c.db.Stats().OpenConnections)
}
