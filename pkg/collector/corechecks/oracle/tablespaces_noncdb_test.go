// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build oracle_test

package oracle

import (
	"database/sql"
	"fmt"
	"testing"

	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

// TestProbeContainerViews reports what V$DATABASE.CDB and V$CONTAINERS return on the
// test database. The CI image is a CDB, so this does not answer what a non-CDB returns;
// it establishes the CDB baseline that the non-CDB behavior is being compared against.
// Output goes to stdout so it is visible without -v.
func TestProbeContainerViews(t *testing.T) {
	c, _ := newDefaultCheck(t, "", "")
	defer c.Teardown()
	require.NoError(t, c.Run())

	var cdb string
	require.NoError(t, getWrapper(&c, &cdb, "SELECT cdb FROM v$database"))
	fmt.Printf("PROBE v$database.cdb = %q multitenant=%v connectedToPdb=%v version=%s\n",
		cdb, c.multitenant, c.connectedToPdb, c.dbVersion)

	type containerRow struct {
		ConID int            `db:"CON_ID"`
		Name  sql.NullString `db:"NAME"`
	}
	var containers []containerRow
	require.NoError(t, selectWrapper(&c, &containers, "SELECT con_id, name FROM v$containers ORDER BY con_id"))
	fmt.Printf("PROBE v$containers rowcount=%d\n", len(containers))
	for _, r := range containers {
		fmt.Printf("PROBE   con_id=%d name=%q\n", r.ConID, r.Name.String)
	}

	// The CON_ID that the tablespace queries actually join on.
	var tsRows []struct {
		ConID          int    `db:"CON_ID"`
		TablespaceName string `db:"TABLESPACE_NAME"`
	}
	require.NoError(t, selectWrapper(&c, &tsRows,
		"SELECT con_id, tablespace_name FROM cdb_tablespaces ORDER BY con_id, tablespace_name"))
	fmt.Printf("PROBE cdb_tablespaces rowcount=%d\n", len(tsRows))
	for _, r := range tsRows {
		fmt.Printf("PROBE   con_id=%d tablespace=%s\n", r.ConID, r.TablespaceName)
	}
}

// TestTablespacesNonCdbQueryPath exercises the branch taken by a non-CDB database.
// A true non-CDB cannot be provisioned in CI: the image is 21c XE, which is CDB-only,
// and 21c desupported the non-CDB architecture outright. The DBA_* views are valid
// inside a CDB though, scoped to the current container, so forcing c.multitenant to
// false runs exactly the SQL a non-CDB would run, against a real database.
func TestTablespacesNonCdbQueryPath(t *testing.T) {
	c, s := newDefaultCheck(t, "", "")
	defer c.Teardown()
	require.NoError(t, c.Run())

	usage, maxSize := c.tablespaceQueries()
	require.Equal(t, tablespaceQuery12, usage, "test db should be a CDB, so the baseline is the 12c path")
	require.Equal(t, maxSizeQuery12, maxSize)

	// Force the non-CDB branch and confirm it selects the DBA_* variants.
	c.multitenant = false
	usage, maxSize = c.tablespaceQueries()
	require.Equal(t, tablespaceQuery11, usage)
	require.Equal(t, maxSizeQuery11, maxSize)

	// The queries must parse and execute against a live Oracle, not just be selected.
	var rows []RowDB
	require.NoError(t, selectWrapper(&c, &rows, usage), "DBA_* tablespace query failed to execute")
	require.NotEmpty(t, rows, "DBA_* tablespace query returned no rows")

	var maxRows []rowMaxSizeDB
	require.NoError(t, selectWrapper(&c, &maxRows, maxSize), "DBA_* max size query failed to execute")
	require.NotEmpty(t, maxRows, "DBA_* max size query returned no rows")

	// This is the tag question Codex raised: with the DBA_* variants, PdbName is never
	// populated, so appendPDBTag emits nothing. Record it explicitly.
	for _, r := range rows {
		require.False(t, r.PdbName.Valid, "DBA_* query unexpectedly projected a pdb name")
	}
	fmt.Printf("PROBE non-cdb path: usage_rows=%d maxsize_rows=%d pdb_tag_emitted=false\n",
		len(rows), len(maxRows))

	// Full collection through Tablespaces() must succeed on this path and still emit metrics.
	s.On("Gauge", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return()
	require.NoError(t, c.Tablespaces())
	s.AssertMetric(t, "Gauge", "oracle.tablespace.size", float64(104857600), c.dbHostname,
		[]string{"tablespace:TBS_TEST"})
}
