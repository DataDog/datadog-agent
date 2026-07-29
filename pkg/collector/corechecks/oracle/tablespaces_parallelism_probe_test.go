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

	"github.com/stretchr/testify/require"
)

// TEMPORARY, PROBE ONLY -- do not merge.
//
// SDBM-2824 assumes the CDB_* tablespace queries are what drive the reported
// parallelism. That has never been measured. This probe measures it directly on the
// CI database: for each query variant it records the real executed plan, whether that
// plan contains parallel (PX) operations, how many parallel server processes the
// statement actually consumed, and the instance-wide "queries parallelized" delta.
//
// Caveat that limits every conclusion drawn here: the CI database is a 21c XE CDB with
// 3 containers and a handful of tablespaces, not a 19c non-CDB Exadata RAC. Absolute
// DOP numbers will not match the customer's. What this can establish is the
// qualitative question -- does the CDB_* form go parallel where the DBA_* form does
// not -- which is the causal link the fix rests on.

type sysstatRow struct {
	Name  string  `db:"NAME"`
	Value float64 `db:"VALUE"`
}

type sqlStatRow struct {
	SQLID               string  `db:"SQL_ID"`
	Executions          float64 `db:"EXECUTIONS"`
	PxServersExecutions float64 `db:"PX_SERVERS_EXECUTIONS"`
	ElapsedTime         float64 `db:"ELAPSED_TIME"`
	BufferGets          float64 `db:"BUFFER_GETS"`
	Rows                float64 `db:"ROWS_PROCESSED"`
}

type planRow struct {
	ID           int    `db:"ID"`
	Operation    string `db:"OPERATION"`
	Options      string `db:"OPTIONS"`
	ObjectName   string `db:"OBJECT_NAME"`
	Distribution string `db:"DISTRIBUTION"`
}

// parallelCounters reads the instance-wide parallel-execution statistics.
func parallelCounters(t *testing.T, c *Check) map[string]float64 {
	var rows []sysstatRow
	require.NoError(t, selectWrapper(c, &rows,
		"SELECT name, value FROM v$sysstat WHERE lower(name) LIKE '%parallel%'"))
	counters := make(map[string]float64, len(rows))
	for _, r := range rows {
		counters[r.Name] = r.Value
	}
	return counters
}

func TestProbeTablespaceParallelism(t *testing.T) {
	c, _ := newDefaultCheck(t, "", "")
	defer c.Teardown()
	require.NoError(t, c.Run())

	// Part 1: do the CDB_* views actually fan out via CONTAINERS()? This is the
	// mechanism the whole theory depends on, and it is readable straight from the
	// dictionary. TEXT_VC avoids the LONG column that TEXT would return.
	for _, view := range []string{
		"CDB_TABLESPACES", "CDB_TABLESPACE_USAGE_METRICS", "CDB_DATA_FILES",
		"DBA_TABLESPACES", "DBA_TABLESPACE_USAGE_METRICS", "DBA_DATA_FILES",
	} {
		var text string
		err := getWrapper(&c, &text,
			fmt.Sprintf("SELECT text_vc FROM dba_views WHERE view_name = '%s'", view))
		if err != nil {
			fmt.Printf("PROBE viewdef %s ERROR %s\n", view, err)
			continue
		}
		flat := strings.Join(strings.Fields(text), " ")
		fmt.Printf("PROBE viewdef %s uses_containers=%v len=%d\n",
			view, strings.Contains(strings.ToUpper(flat), "CONTAINERS("), len(flat))
		if len(flat) > 300 {
			flat = flat[:300] + "..."
		}
		fmt.Printf("PROBE viewdef %s text=%s\n", view, flat)
	}

	// Part 2: execute each variant and measure what it actually did.
	variants := []struct {
		name  string
		query string
	}{
		{"cdb_usage", tablespaceQuery12},
		{"dba_usage", tablespaceQuery11},
		{"cdb_maxsize", maxSizeQuery12},
		{"dba_maxsize", maxSizeQuery11},
	}

	for _, v := range variants {
		marker := "ddprobe_" + v.name
		// Wrapping keeps the inner query verbatim while giving it a findable marker.
		marked := fmt.Sprintf("SELECT /* %s */ * FROM (%s)", marker, v.query)

		before := parallelCounters(t, &c)

		rows, err := c.db.Query(marked)
		if err != nil {
			fmt.Printf("PROBE run %s ERROR %s\n", v.name, err)
			continue
		}
		// Oracle does the work on fetch, so drain the cursor before measuring.
		fetched := 0
		for rows.Next() {
			fetched++
		}
		rowsErr := rows.Err()
		rows.Close()

		after := parallelCounters(t, &c)

		fmt.Printf("PROBE run %s rows=%d err=%v\n", v.name, fetched, rowsErr)
		for name, post := range after {
			if delta := post - before[name]; delta != 0 {
				fmt.Printf("PROBE run %s sysstat_delta %s=%.0f\n", v.name, name, delta)
			}
		}

		var stats []sqlStatRow
		require.NoError(t, selectWrapper(&c, &stats, fmt.Sprintf(
			`SELECT sql_id, executions, px_servers_executions, elapsed_time, buffer_gets, rows_processed
			 FROM v$sql
			 WHERE sql_text LIKE '%%%s%%' AND sql_text NOT LIKE '%%v$sql%%'`, marker)))
		for _, s := range stats {
			fmt.Printf("PROBE sqlstat %s sql_id=%s execs=%.0f px_servers_executions=%.0f elapsed_us=%.0f buffer_gets=%.0f rows=%.0f\n",
				v.name, s.SQLID, s.Executions, s.PxServersExecutions, s.ElapsedTime, s.BufferGets, s.Rows)

			var plan []planRow
			require.NoError(t, selectWrapper(&c, &plan, fmt.Sprintf(
				`SELECT id, operation, NVL(options,' ') options, NVL(object_name,' ') object_name,
				        NVL(distribution,' ') distribution
				 FROM v$sql_plan WHERE sql_id = '%s' ORDER BY id`, s.SQLID)))
			pxOps := 0
			for _, p := range plan {
				if strings.HasPrefix(p.Operation, "PX") {
					pxOps++
				}
				fmt.Printf("PROBE plan %s %2d %s %s %s %s\n",
					v.name, p.ID, p.Operation, p.Options, p.ObjectName, p.Distribution)
			}
			fmt.Printf("PROBE planpx %s px_operations=%d plan_rows=%d\n", v.name, pxOps, len(plan))
		}
	}

	// Part 3: how many rows does each form return? A CDB_* query on a single-container
	// database returning the same rows as DBA_* is the premise of the release note.
	for _, q := range []struct {
		name  string
		query string
	}{{"cdb_usage", tablespaceQuery12}, {"dba_usage", tablespaceQuery11}} {
		var rows []RowDB
		if err := selectWrapper(&c, &rows, q.query); err != nil {
			fmt.Printf("PROBE rowcount %s ERROR %s\n", q.name, err)
			continue
		}
		pdbTagged := 0
		for _, r := range rows {
			if r.PdbName.Valid {
				pdbTagged++
			}
		}
		fmt.Printf("PROBE rowcount %s rows=%d with_pdb_name=%d\n", q.name, len(rows), pdbTagged)
	}
}
