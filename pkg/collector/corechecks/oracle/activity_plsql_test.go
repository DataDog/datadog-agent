// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build oracle_test

package oracle

import (
	"encoding/json"
	"errors"
	"regexp"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/benbjohnson/clock"
	"github.com/jmoiron/sqlx"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestIsInPLSQL(t *testing.T) {
	objectID := uint64(12345)
	zeroObjectID := uint64(0)
	tests := []struct {
		name               string
		plsqlEntryObjectID *uint64
		topLevelSQLID      string
		sqlID              string
		expected           bool
	}{
		{name: "top-level statement", sqlID: "abc", expected: false},
		{name: "plsql entry object set", plsqlEntryObjectID: &objectID, sqlID: "abc", expected: true},
		{name: "plsql entry object set to zero", plsqlEntryObjectID: &zeroObjectID, sqlID: "abc", expected: true},
		{name: "ash top-level statement", topLevelSQLID: "abc", sqlID: "abc", expected: false},
		{name: "ash statement run by a plsql call", topLevelSQLID: "plsqlcall", sqlID: "abc", expected: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, isInPLSQL(tt.plsqlEntryObjectID, tt.topLevelSQLID, tt.sqlID))
		})
	}
}

func newActivityMockCheck(t *testing.T) (*Check, sqlmock.Sqlmock) {
	db, dbMock, err := sqlmock.New()
	require.NoError(t, err)
	t.Cleanup(func() { db.Close() })

	c, _ := newDbDoesNotExistCheck(t, "", "")
	c.db = sqlx.NewDb(db, "sqlmock")
	c.clock = clock.New()
	c.sqlSubstringLength = 4000
	c.config.QuerySamples.IncludeAllSessions = true
	return &c, dbMock
}

// activityRowJSON returns the JSON object sent for an activity row.
func activityRowJSON(t *testing.T, row OracleActivityRow) map[string]interface{} {
	b, err := json.Marshal(row)
	require.NoError(t, err)
	var m map[string]interface{}
	require.NoError(t, json.Unmarshal(b, &m))
	return m
}

func TestSampleSessionInPLSQLDirect(t *testing.T) {
	c, dbMock := newActivityMockCheck(t)
	c.config.QuerySamples.ForceDirectQuery = true

	activityRows := sqlmock.NewRows([]string{
		"NOW", "SID", "SERIAL#", "STATUS", "SQL_ID", "SQL_FULLTEXT", "PREV_SQL_ID", "PREV_SQL_FULLTEXT", "PLSQL_ENTRY_OBJECT_ID",
	}).
		AddRow("2024-01-01 00:00:00", 1, 1, "ACTIVE", "toplevel", "SELECT 1 FROM dual", nil, nil, nil).
		AddRow("2024-01-01 00:00:00", 2, 1, "ACTIVE", "inproc", "SELECT 2 FROM dual", nil, nil, 12345).
		AddRow("2024-01-01 00:00:00", 3, 1, "INACTIVE", nil, nil, "previous", "SELECT 3 FROM dual", nil).
		AddRow("2024-01-01 00:00:00", 4, 1, "ACTIVE", "zeroentry", "SELECT 4 FROM dual", nil, nil, 0)
	dbMock.ExpectQuery("s.plsql_entry_object_id").WillReturnRows(activityRows)

	require.NoError(t, c.SampleSession())
	require.NoError(t, dbMock.ExpectationsWereMet())
	require.Len(t, c.lastOracleActivityRows, 4)

	topLevel := activityRowJSON(t, c.lastOracleActivityRows[0])
	assert.Equal(t, false, topLevel["in_plsql"], "in_plsql must be sent when false")
	assert.NotContains(t, topLevel, "plsql_entry_object_id")
	assert.NotContains(t, topLevel, "top_level_sql_id")

	inProcedure := activityRowJSON(t, c.lastOracleActivityRows[1])
	assert.Equal(t, true, inProcedure["in_plsql"])
	assert.Equal(t, float64(12345), inProcedure["plsql_entry_object_id"])

	previous := activityRowJSON(t, c.lastOracleActivityRows[2])
	assert.Equal(t, true, previous["previous_sql"])
	assert.NotContains(t, previous, "in_plsql", "in_plsql is unknown for the previous SQL of an idle session")

	// Only NULL means no entry object: a non-NULL zero is sent and counts as set.
	zeroEntry := activityRowJSON(t, c.lastOracleActivityRows[3])
	assert.Equal(t, float64(0), zeroEntry["plsql_entry_object_id"])
	assert.Equal(t, true, zeroEntry["in_plsql"])
}

func TestSampleSessionInPLSQLActiveSessionHistory(t *testing.T) {
	c, dbMock := newActivityMockCheck(t)
	c.config.QuerySamples.ActiveSessionHistory = true
	c.lastSampleID = 1

	activityRows := sqlmock.NewRows([]string{
		"NOW", "SAMPLE_ID", "SID", "SERIAL#", "STATUS", "SQL_ID", "SQL_FULLTEXT", "PLSQL_ENTRY_OBJECT_ID", "TOP_LEVEL_SQL_ID",
	}).
		AddRow("2024-01-01 00:00:00", 2, 1, 1, "ACTIVE", "toplevel", "SELECT 1 FROM dual", nil, "toplevel").
		AddRow("2024-01-01 00:00:00", 2, 2, 1, "ACTIVE", "intrigger", "INSERT INTO audit VALUES (1)", nil, "callerid")
	dbMock.ExpectQuery("s.top_level_sql_id").WillReturnRows(activityRows)

	require.NoError(t, c.SampleSession())
	require.NoError(t, dbMock.ExpectationsWereMet())
	require.Len(t, c.lastOracleActivityRows, 2)

	topLevel := activityRowJSON(t, c.lastOracleActivityRows[0])
	assert.Equal(t, false, topLevel["in_plsql"])
	assert.Equal(t, "toplevel", topLevel["top_level_sql_id"])

	inTrigger := activityRowJSON(t, c.lastOracleActivityRows[1])
	assert.Equal(t, true, inTrigger["in_plsql"])
	assert.Equal(t, "callerid", inTrigger["top_level_sql_id"])
}

func TestSampleSessionActivityView(t *testing.T) {
	probe := regexp.QuoteMeta(activityViewPLSQLEntryObjectIDProbe)
	activityRows := func() *sqlmock.Rows {
		return sqlmock.NewRows([]string{"NOW", "SID", "SERIAL#", "STATUS", "SQL_ID", "SQL_FULLTEXT", "PLSQL_ENTRY_OBJECT_ID"}).
			AddRow("2024-01-01 00:00:00", 1, 1, "ACTIVE", "toplevel", "SELECT 1 FROM dual", nil)
	}

	t.Run("view with plsql_entry_object_id", func(t *testing.T) {
		c, dbMock := newActivityMockCheck(t)
		c.hostingType = selfManaged
		c.dbVersion = "19.0.0.0.0"

		dbMock.ExpectQuery(probe).WillReturnRows(sqlmock.NewRows([]string{"PLSQL_ENTRY_OBJECT_ID"}))
		dbMock.ExpectQuery(`\splsql_entry_object_id,(.|\n)*FROM sys.dd_session`).WillReturnRows(activityRows())

		require.NoError(t, c.SampleSession())
		require.NoError(t, dbMock.ExpectationsWereMet())
		require.Len(t, c.lastOracleActivityRows, 1)
		assert.Equal(t, false, activityRowJSON(t, c.lastOracleActivityRows[0])["in_plsql"])
	})

	t.Run("view created before plsql_entry_object_id was added", func(t *testing.T) {
		c, dbMock := newActivityMockCheck(t)
		c.hostingType = selfManaged
		c.dbVersion = "19.0.0.0.0"

		dbMock.ExpectQuery(probe).WillReturnError(errors.New(`ORA-00904: "PLSQL_ENTRY_OBJECT_ID": invalid identifier`))
		dbMock.ExpectQuery(`NULL plsql_entry_object_id,(.|\n)*FROM sys.dd_session`).WillReturnRows(activityRows())
		// The missing column is remembered: the second run doesn't probe the view again.
		dbMock.ExpectQuery(`NULL plsql_entry_object_id,(.|\n)*FROM sys.dd_session`).WillReturnRows(activityRows())

		require.NoError(t, c.SampleSession())
		require.NoError(t, c.SampleSession())
		require.NoError(t, dbMock.ExpectationsWereMet())
		require.Len(t, c.lastOracleActivityRows, 1)
		assert.NotContains(t, activityRowJSON(t, c.lastOracleActivityRows[0]), "in_plsql", "in_plsql is unknown without the column")
	})

	t.Run("probe failure other than a missing column", func(t *testing.T) {
		c, dbMock := newActivityMockCheck(t)
		c.hostingType = selfManaged
		c.dbVersion = "19.0.0.0.0"

		dbMock.ExpectQuery(probe).WillReturnError(errors.New("ORA-00942: table or view does not exist"))

		require.Error(t, c.SampleSession())
		require.NoError(t, dbMock.ExpectationsWereMet())
		assert.False(t, c.activityViewPLSQLEntryObjectIDChecked, "a failed probe must be retried")
	})

	for name, query := range map[string]string{"12": activityQueryOnView12, "11": activityQueryOnView11} {
		assert.Contains(t, activityQueryOnViewWithoutPLSQLEntryObjectID(query), "\tNULL plsql_entry_object_id,", "activityQueryOnView%s", name)
	}
}
