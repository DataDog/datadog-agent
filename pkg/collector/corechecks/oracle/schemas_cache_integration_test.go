// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build oracle_test

package oracle

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/jmoiron/sqlx"
	"github.com/stretchr/testify/require"
)

type schemaCacheQuery struct {
	index      int
	executions int
	values     map[string]struct{}
}

type schemaCacheRecorder struct {
	conn    *sqlx.Conn
	prefix  string
	queries map[string]*schemaCacheQuery
}

func (r *schemaCacheRecorder) QueryxContext(ctx context.Context, query string, args ...any) (*sqlx.Rows, error) {
	record, ok := r.queries[query]
	if !ok {
		record = &schemaCacheQuery{index: len(r.queries), values: make(map[string]struct{})}
		r.queries[query] = record
	}
	marker := fmt.Sprintf("%s%d */ ", r.prefix, record.index)
	rows, err := r.conn.QueryxContext(ctx, marker+query, args...)
	if err == nil {
		record.executions++
		values, marshalErr := json.Marshal(args)
		if marshalErr != nil {
			_ = rows.Close()
			return nil, marshalErr
		}
		record.values[string(values)] = struct{}{}
	}
	return rows, err
}

type schemaCacheCounters struct {
	SQLID         string `db:"SQL_ID"`
	Executions    int64  `db:"EXECUTIONS"`
	Loads         int64  `db:"LOADS"`
	Invalidations int64  `db:"INVALIDATIONS"`
	Children      int64  `db:"CHILDREN"`
}

func schemaCacheStats(ctx context.Context, t *testing.T, observer *sqlx.DB, prefix string) map[string]schemaCacheCounters {
	t.Helper()
	var rows []schemaCacheCounters
	err := observer.SelectContext(ctx, &rows, `SELECT sql_id, SUM(executions) AS executions,
		SUM(loads) AS loads, SUM(invalidations) AS invalidations, COUNT(*) AS children
		FROM v$sql WHERE INSTR(sql_text, :1) = 1 AND executions > 0 GROUP BY sql_id`, prefix)
	require.NoError(t, err)
	stats := make(map[string]schemaCacheCounters, len(rows))
	for _, row := range rows {
		stats[row.SQLID] = row
	}
	return stats
}

func TestSchemaCollectionReusesCachedSQLAgainstDatabase(t *testing.T) {
	setupSchemaFixtures(t)
	sysCheck, _ := newSysCheck(t, "", "")
	observer, err := sysCheck.Connect()
	require.NoError(t, err)
	t.Cleanup(func() { _ = observer.Close() })
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	const otherOwner = "C##DD_SCHEMA_BIND_OTHER"
	_, err = observer.ExecContext(ctx, "CREATE USER "+otherOwner+" IDENTIFIED BY dd_schema_test CONTAINER=ALL")
	require.NoError(t, err)
	t.Cleanup(func() { _, _ = observer.Exec("DROP USER " + otherOwner + " CASCADE") })
	_, err = observer.ExecContext(ctx, "GRANT UNLIMITED TABLESPACE TO "+otherOwner)
	require.NoError(t, err)
	_, err = observer.ExecContext(ctx, "CREATE TABLE "+otherOwner+".DD_BIND_OTHER (id NUMBER)")
	require.NoError(t, err)
	for i := range 201 {
		_, err = observer.ExecContext(ctx, fmt.Sprintf("CREATE TABLE %s.DD_BIND_T%03d (id NUMBER, value VARCHAR2(30) DEFAULT 'value')", schemaTestUser, i))
		require.NoError(t, err)
		if i < 101 {
			_, err = observer.ExecContext(ctx, fmt.Sprintf("CREATE VIEW %s.DD_BIND_V%03d AS SELECT id FROM %s.DD_BIND_T%03d", schemaTestUser, i, schemaTestUser, i))
			require.NoError(t, err)
		}
	}

	c, sender := newDefaultCheck(t, `collect_schemas:
  enabled: true
  include_schemas:
    - '^C##DD_SCHEMA_(TEST|BIND_OTHER)$'
  include_tables:
    - '^DD_BIND_'
`, "")
	defer c.Teardown()
	require.NoError(t, c.init())
	conn, err := c.db.Connx(ctx)
	require.NoError(t, err)
	defer conn.Close()
	_, err = conn.ExecContext(ctx, "ALTER SESSION SET cursor_sharing=EXACT")
	require.NoError(t, err)
	var sid int
	require.NoError(t, conn.QueryRowContext(ctx, "SELECT SYS_CONTEXT('USERENV', 'SID') FROM dual").Scan(&sid))
	recorder := &schemaCacheRecorder{
		conn: conn, prefix: fmt.Sprintf("/* dd_schema_cache_%d_", time.Now().UnixNano()),
		queries: make(map[string]*schemaCacheQuery),
	}
	c.schemaQueryer = recorder
	hardParses := func() int64 {
		var count int64
		require.NoError(t, observer.GetContext(ctx, &count, `SELECT s.value FROM v$sesstat s
			JOIN v$statname n ON n.statistic# = s.statistic#
			WHERE s.sid = :1 AND n.name = 'parse count (hard)'`, sid))
		return count
	}
	var previous []schemaEvent
	collect := func() {
		start := len(sender.Calls)
		require.NoError(t, c.schemaCollection(ctx))
		var events []schemaEvent
		for _, call := range sender.Calls[start:] {
			if call.Method != "EventPlatformEvent" {
				continue
			}
			var event schemaEvent
			require.NoError(t, json.Unmarshal(call.Arguments.Get(0).([]byte), &event))
			event.Timestamp = 0
			event.CollectionStartedAt = 0
			events = append(events, event)
		}
		require.NotEmpty(t, events)
		if previous != nil {
			require.Equal(t, previous, events, "binding and cursor reuse must preserve metadata and snapshot completion")
		}
		previous = events
	}
	collect()
	collect()
	warmQueryCount := len(recorder.queries)
	before := schemaCacheStats(ctx, t, observer, recorder.prefix)
	parsesBefore := hardParses()
	collect()
	parsesAfter := hardParses()
	after := schemaCacheStats(ctx, t, observer, recorder.prefix)
	require.Len(t, recorder.queries, warmQueryCount, "collector generated a new SQL text after warmup")
	require.NotEmpty(t, before)
	reused := 0
	for id, prior := range before {
		current, ok := after[id]
		if ok && current.Executions > prior.Executions && current.Loads == prior.Loads &&
			current.Invalidations == prior.Invalidations && current.Children == prior.Children {
			reused++
		} else {
			t.Logf("Oracle cursor changed or aged out for %s: before=%+v after=%+v", id, prior, current)
		}
	}
	require.Positive(t, reused, "no cursor reuse observed; check shared-pool pressure and invalidations")
	for _, fragment := range []string{"WITH ranked_columns", "FROM cdb_indexes", "FROM cdb_col_comments", "FROM cdb_views WHERE"} {
		shared := false
		for query, record := range recorder.queries {
			if strings.Contains(query, fragment) && len(record.values) > 1 {
				shared = true
			}
		}
		require.True(t, shared, "different pages must share SQL for %s", fragment)
	}
	var tables, views, completions int
	for _, event := range previous {
		if event.CollectionPayloadsCount > 0 {
			completions++
		}
		for _, container := range event.Metadata {
			for _, schema := range container.Schemas {
				tables += len(schema.Tables)
				views += len(schema.Views)
			}
		}
	}
	require.Equal(t, 202, tables)
	require.Equal(t, 101, views)
	require.Positive(t, completions)
	t.Logf("reused %d/%d observed SQL IDs; warm collection session hard-parse delta: %d", reused, len(before), parsesAfter-parsesBefore)
}
