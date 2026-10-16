// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build oracle

package oracle

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/oracle/common"
)

// Crediting an arbitrary PL/SQL block as one call invents counts for loops,
// branches and nested expressions. Only a single scalar-argument call qualifies.
func TestDirectProcedureCall(t *testing.T) {
	for _, tc := range []struct{ sql, owner, name string }{
		{"CALL orders.refresh(:1)", "ORDERS", "REFRESH"},
		{"begin /* wrapper */ refresh(p_id => :id, p_text => 'a;--b'); end;", "APP", "REFRESH"},
		{`BEGIN "Mixed.Owner"."Mixed.Case"(2, NULL); END;`, "Mixed.Owner", "Mixed.Case"},
		{`begin "a""b"; end;`, "APP", `a"b`},
		{"CALL refresh(-1.2e3); -- trailing", "APP", "REFRESH"},
	} {
		t.Run(tc.sql, func(t *testing.T) {
			owner, name, _, ok := parseDirectProcedureCall(tc.sql, "APP")
			require.True(t, ok)
			assert.Equal(t, tc.owner, owner)
			assert.Equal(t, tc.name, name)
		})
	}
	for _, statement := range []string{
		"begin a(:1); b(:1); end;", "begin for i in 1..5 loop a(i); end loop; end;",
		"declare x number; begin a(x); end;", "begin a(f()); end;", "call a(1+1)",
		"begin if 1=1 then a; end if; end;", "begin app.pkg.a(:1); end;",
		"call a@remote(:1)", "begin a(:1); exception when others then null; end;",
		"begin a(:1); end; /", "begin a(:1); end; /* incomplete", "select a(:1) from dual",
	} {
		_, _, _, ok := parseDirectProcedureCall(statement, "APP")
		assert.False(t, ok, statement)
	}
}

// A cursor can reuse its SQL ID and address after purge, and DDL can retain an
// object ID. Neither case may turn a new lifetime into a positive counter spike.
func TestProcedureCursorLifetime(t *testing.T) {
	previous := procedureCursorBaseline{counts: procedureCounters{Executions: 2, ElapsedTime: 10}, loads: 1, objectID: 42, lastDDL: "first", signature: "p"}
	for _, tc := range []struct {
		name   string
		change func(*procedureCursorBaseline)
		valid  bool
	}{
		{"increment", func(b *procedureCursorBaseline) { b.counts.Executions = 5 }, true},
		{"counter reset", func(b *procedureCursorBaseline) { b.counts.Executions = 1 }, false},
		{"timer reset", func(b *procedureCursorBaseline) { b.counts.ElapsedTime = 1 }, false},
		{"reload", func(b *procedureCursorBaseline) { b.loads++ }, false},
		{"invalidation", func(b *procedureCursorBaseline) { b.invalidations++ }, false},
		{"DDL", func(b *procedureCursorBaseline) { b.lastDDL = "second" }, false},
		{"replacement", func(b *procedureCursorBaseline) { b.objectID++ }, false},
		{"identity reuse", func(b *procedureCursorBaseline) { b.signature = "different" }, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			current := previous
			tc.change(&current)
			_, valid := current.delta(previous)
			assert.Equal(t, tc.valid, valid)
		})
	}
}

func procedureObjectRows(id int64, name string) *sqlmock.Rows {
	return sqlmock.NewRows([]string{"OBJECT_ID", "OWNER", "OBJECT_NAME", "LAST_DDL"}).AddRow(id, "APP", name, "2026-01-01 00:00:00")
}

// Payloads must preserve unknown nested counts, independent PDB identity and
// nonexclusive sampled SQL membership without duplicating child SQL counters.
func TestCollectStoredProcedures(t *testing.T) {
	c, mock, sender := newSamplingCheck(t)
	c.multitenant = true
	call := "begin app.parent(:1); end;"
	for _, executions := range []int{4, 7, 1, 3} {
		rows := sqlmock.NewRows([]string{"CON_ID", "PDB_NAME", "SQL_ID", "CHILD_NUMBER", "CHILD_ADDRESS", "LOADS", "PROGRAM_ID", "PROGRAM_LINE", "PARSING_SCHEMA_NAME", "COMMAND_TYPE", "SQL_TEXT", "SQL_TEXT_LENGTH", "EXECUTIONS", "ELAPSED_TIME"})
		for _, pdb := range []int{3, 4} {
			rows.AddRow(pdb, fmt.Sprintf("PDB%d", pdb), "parent", 0, "ABC", 1, 0, 0, "APP", 47, call, len(call), executions, executions*10)
			identity := (procedureObject{Owner: "APP", Name: "CHILD"}).identity(c.getFullPDBName(fmt.Sprintf("PDB%d", pdb)))
			c.observeActivityProcedure(identity)
			c.observeProcedureQuery(OracleActivityRow{EntryProcedureName: identity.Name, EntryProcedureSignature: identity.Signature, QuerySignature: "query"}, fmt.Sprintf("PDB%d", pdb))
		}
		mock.ExpectQuery("DD_PROCEDURES").WillReturnRows(rows)
		for _, pdb := range []int{3, 4} {
			mock.ExpectQuery("SELECT 1 FROM cdb_objects").WithArgs(pdb, "APP", "APP").WillReturnRows(sqlmock.NewRows([]string{"1"}))
			mock.ExpectQuery("FROM cdb_objects").WithArgs(pdb, "APP", "PARENT").WillReturnRows(procedureObjectRows(40, "PARENT"))
		}
		require.NoError(t, c.collectStoredProcedures())
	}
	var payloads []procedureMetricsPayload
	for _, call := range sender.Calls {
		if call.Method == "EventPlatformEvent" && call.Arguments[1] == "dbm-metrics" {
			var payload procedureMetricsPayload
			require.NoError(t, json.Unmarshal(call.Arguments[0].([]byte), &payload))
			payloads = append(payloads, payload)
		}
	}
	require.Len(t, payloads, 4)
	for i, payload := range payloads {
		assert.Equal(t, "procedure_metrics", payload.Kind)
		require.Len(t, payload.OracleRows, 4)
		require.Len(t, payload.Associations, 2)
		identities := map[string]bool{}
		for _, row := range payload.OracleRows {
			assert.False(t, identities[row.Signature], "PDB identities must not collapse")
			identities[row.Signature] = true
			assert.Equal(t, 1, row.Observed)
			if row.Name == "APP.CHILD" {
				assert.Equal(t, "observed", row.MetricsSource)
				assert.Nil(t, row.Executions)
				assert.Nil(t, row.ElapsedTime)
			} else if i == 0 || i == 2 {
				assert.Nil(t, row.Executions, "initial observation/reset establishes baseline")
			} else {
				require.NotNil(t, row.Executions)
				assert.Equal(t, []float64{0, 3, 0, 2}[i], *row.Executions)
			}
		}
		assert.Equal(t, payload.Associations[0].QuerySignature, payload.Associations[1].QuerySignature)
		assert.NotEqual(t, payload.Associations[0].Signature, payload.Associations[1].Signature)
	}
	require.NoError(t, mock.ExpectationsWereMet())
}

// Purging the first PDB's cursor leaves a surviving PROGRAM_ID that can name
// an unrelated procedure in another PDB. No multitenant origin row is trustworthy.
func TestMultitenantProcedureOriginIsUnresolved(t *testing.T) {
	for _, pdbLocal := range []bool{false, true} {
		t.Run(fmt.Sprintf("PDB local=%t", pdbLocal), func(t *testing.T) {
			c, mock, sender := newSamplingCheck(t)
			c.multitenant, c.connectedToPdb = true, pdbLocal
			rows := sqlmock.NewRows([]string{"CON_ID", "PDB_NAME", "SQL_ID", "PROGRAM_ID", "SQL_TEXT", "SQL_TEXT_LENGTH"}).
				AddRow(4, "PDB4", "surviving_shared_cursor", 42, "select 1 from dual", 18)
			// Origin-only cursors must not consume the direct-call collection budget.
			mock.ExpectQuery(`s.command_type IN [(]47,170[)] AND s.program_id = 0`).WillReturnRows(rows)
			require.NoError(t, c.collectStoredProcedures())
			var payload procedureMetricsPayload
			for _, call := range sender.Calls {
				if call.Method == "EventPlatformEvent" {
					require.NoError(t, json.Unmarshal(call.Arguments[0].([]byte), &payload))
				}
			}
			assert.Empty(t, payload.OracleRows, "an unrelated object must not enter the inventory")
			assert.Empty(t, payload.Associations)
			require.NoError(t, mock.ExpectationsWereMet())
		})
	}
}

// Upgrading the Agent before the installed activity view must retain samples.
func TestProcedureActivityOldViewFallback(t *testing.T) {
	c, mock, _ := newSamplingCheck(t)
	c.config.StoredProcedures.Enabled = true
	c.config.QuerySamples.ForceDirectQuery = false
	c.hostingType = selfManaged
	mock.ExpectQuery("plsql_entry_object_id").WillReturnError(errors.New("ORA-00904: invalid identifier"))
	mock.ExpectQuery("FROM sys.dd_session").WillReturnRows(sqlmock.NewRows([]string{"NOW", "SID", "SERIAL#", "STATUS"}).AddRow("now", 1, 1, "ACTIVE"))
	require.NoError(t, c.SampleSession())
	require.Len(t, c.lastOracleActivityRows, 1)
	assert.Empty(t, c.lastOracleActivityRows[0].EntryProcedureSignature)
	require.NoError(t, mock.ExpectationsWereMet())
}

// Entry and current routines are different roles, and resolving their names
// must not lose an otherwise valid activity sample when grants are missing.
func TestProcedureActivityContext(t *testing.T) {
	for _, mode := range []string{"direct", "ash", "missing grants"} {
		t.Run(mode, func(t *testing.T) {
			c, mock, _ := newSamplingCheck(t)
			c.config.StoredProcedures.Enabled = true
			c.cdbName = "free"
			c.multitenant = true
			c.config.QuerySamples.ActiveSessionHistory = mode == "ash"
			c.lastSampleID = 1
			mock.ExpectQuery("plsql_entry_object_id").WillReturnRows(sqlmock.NewRows([]string{
				"NOW", "SID", "SERIAL#", "STATUS", "CON_ID", "PDB_NAME", "PLSQL_ENTRY_OBJECT_ID", "PLSQL_ENTRY_SUBPROGRAM_ID", "PLSQL_OBJECT_ID", "PLSQL_SUBPROGRAM_ID", "SQL_ID", "SQL_FULLTEXT", "COMMAND_NAME",
			}).AddRow("now", 1, 1, "ACTIVE", 3, "PDB3", 40, 1, 42, 1, "sql", "select 1 from dual", "SELECT"))
			if mode == "missing grants" {
				mock.ExpectQuery("FROM cdb_objects").WillReturnError(errors.New("ORA-00942: missing grant"))
			} else {
				mock.ExpectQuery("FROM cdb_objects").WithArgs(3, int64(40)).WillReturnRows(procedureObjectRows(40, "PARENT"))
				mock.ExpectQuery("FROM cdb_objects").WithArgs(3, int64(42)).WillReturnRows(procedureObjectRows(42, "CHILD"))
			}
			require.NoError(t, c.SampleSession())
			require.Len(t, c.lastOracleActivityRows, 1)
			row := c.lastOracleActivityRows[0]
			assert.NotEmpty(t, row.QuerySignature)
			if mode == "missing grants" {
				assert.Empty(t, row.EntryProcedureName)
				assert.Empty(t, row.CurrentProcedureName)
			} else {
				assert.Equal(t, "APP.PARENT", row.EntryProcedureName)
				assert.Equal(t, "APP.CHILD", row.CurrentProcedureName)
				assert.Len(t, c.procedureActivityAssociations, 2)
				assert.Equal(t, (procedureObject{Owner: "APP", Name: "PARENT"}).identity("free.PDB3").Signature, row.EntryProcedureSignature)
				assert.NotEqual(t, row.EntryProcedureSignature, row.CurrentProcedureSignature)
			}
			require.NoError(t, mock.ExpectationsWereMet())
		})
	}
}

// Repeated collection failures cannot accumulate an unbounded or indefinitely
// stale activity-derived procedure inventory.
func TestProcedureActivityObservationBounds(t *testing.T) {
	c, mock, sender := newSamplingCheck(t)
	c.config.StoredProcedures.DBRowsLimit = 1
	c.config.StoredProcedures.CollectionInterval = 60
	c.procedureActivityObserved = map[string]observedProcedure{
		"old": {identity: procedureIdentity{Signature: "old"}, seen: c.clock.Now().Add(-3 * time.Minute)},
	}
	c.observeActivityProcedure(procedureIdentity{Signature: "recent"})
	c.observeActivityProcedure(procedureIdentity{Signature: "overflow"})
	require.Len(t, c.procedureActivityObserved, 1)
	assert.Contains(t, c.procedureActivityObserved, "recent")
	// A requested collection interval shorter than the actual check interval
	// must still publish the previous check's CPU-only procedure observation.
	c.config.StoredProcedures.CollectionInterval = 1
	recent := c.procedureActivityObserved["recent"]
	recent.seen = c.clock.Now().Add(-4 * time.Second)
	c.procedureActivityObserved["recent"] = recent
	mock.ExpectQuery("DD_PROCEDURES").WillReturnRows(sqlmock.NewRows([]string{"SQL_ID"}))
	require.NoError(t, c.collectStoredProcedures())
	var payload procedureMetricsPayload
	for _, call := range sender.Calls {
		if call.Method == "EventPlatformEvent" {
			require.NoError(t, json.Unmarshal(call.Arguments[0].([]byte), &payload))
		}
	}
	require.Len(t, payload.OracleRows, 1)
	assert.Equal(t, "recent", payload.OracleRows[0].Signature)
	assert.Nil(t, payload.OracleRows[0].Executions)
	require.NoError(t, mock.ExpectationsWereMet())
}

// Only an active current SQL statement can create a sampled procedure link;
// previous SQL, call wrappers and incomplete/unsafe text invent associations.
func TestSampledProcedureAssociationBoundaries(t *testing.T) {
	c, _, _ := newSamplingCheck(t)
	c.config.StoredProcedures.Enabled = true
	for _, mode := range []string{"current", "previous", "fallback", "truncated", "obfuscation failure", "wrapper", "inactive", "no SQL ID"} {
		t.Run(mode, func(t *testing.T) {
			c.procedureActivityAssociations = nil
			row := OracleActivityRow{OracleSQLRow: OracleSQLRow{SQLID: "sql"}, QuerySignature: "query", EntryProcedureName: "APP.P", EntryProcedureSignature: "procedure"}
			sample := OracleActivityRowDB{Status: "ACTIVE", CommandName: sql.NullString{String: "SELECT", Valid: true}, PdbName: sql.NullString{String: "PDB", Valid: true}}
			completeSQLText := true
			switch mode {
			case "previous":
				row.PreviousSQL = true
			case "fallback":
				completeSQLText = false
			case "truncated":
				completeSQLText = false
			case "obfuscation failure":
				row.QuerySignature = ""
			case "wrapper":
				sample.CommandName.String = "PL/SQL EXECUTE"
			case "inactive":
				sample.Status = "INACTIVE"
			case "no SQL ID":
				row.SQLID = ""
			}
			c.observeCurrentProcedureQuery(sample, row, completeSQLText)
			if mode == "current" {
				require.Len(t, c.procedureActivityAssociations, 1)
			} else {
				assert.Empty(t, c.procedureActivityAssociations)
			}
		})
	}
}

// Successful CLOB recovery establishes a complete current SQL identity even
// when the statement exceeds the activity substring limit. Failed recovery
// must not link the truncated prefix to a procedure.
func TestSampledProcedureAssociationFullTextRecovery(t *testing.T) {
	for _, tc := range []struct {
		name      string
		recovered bool
		multibyte bool
	}{
		{"ASCII", true, false},
		{"missing CLOB", false, false},
		{"multibyte", true, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c, mock, _ := newSamplingCheck(t)
			c.driver = common.Godror
			c.multitenant = true
			c.config.StoredProcedures.Enabled = true
			statement := "select 1 from dual " + strings.Repeat(" ", c.sqlSubstringLength) + "where 1=1"
			if tc.multibyte {
				statement = "select /*" + strings.Repeat("界", c.sqlSubstringLength) + "*/ 1 from dual where 1=1"
			}
			mock.ExpectQuery("DD_ACTIVITY_SAMPLING").WillReturnRows(sqlmock.NewRows([]string{
				"NOW", "SID", "SERIAL#", "STATUS", "CON_ID", "PDB_NAME", "PLSQL_ENTRY_OBJECT_ID", "PLSQL_ENTRY_SUBPROGRAM_ID", "SQL_ID", "SQL_FULLTEXT", "COMMAND_NAME",
			}).AddRow("now", 1, 1, "ACTIVE", 3, "PDB3", 40, 1, "long_sql", string([]rune(statement)[:c.sqlSubstringLength]), "SELECT"))
			mock.ExpectQuery("FROM cdb_objects").WithArgs(3, int64(40)).WillReturnRows(procedureObjectRows(40, "PARENT"))
			expected := mock.ExpectQuery("SELECT /\\* DD \\*/ sql_fulltext").WithArgs("long_sql")
			if tc.recovered {
				expected.WillReturnRows(sqlmock.NewRows([]string{"SQL_FULLTEXT"}).AddRow(statement))
			} else {
				expected.WillReturnRows(sqlmock.NewRows([]string{"SQL_FULLTEXT"}))
			}
			require.NoError(t, c.SampleSession())
			require.Len(t, c.lastOracleActivityRows, 1)
			if tc.recovered {
				require.Len(t, c.procedureActivityAssociations, 1)
				for association := range c.procedureActivityAssociations {
					assert.Equal(t, c.lastOracleActivityRows[0].QuerySignature, association.QuerySignature)
				}
			} else {
				assert.Empty(t, c.procedureActivityAssociations)
			}
			require.NoError(t, mock.ExpectationsWereMet())
		})
	}
}

// Non-CDB CON_ID=0 is classified as connectedToPdb by initialization, but its
// cursor origin is still within one database and resolves through DBA_OBJECTS.
func TestNonCDBProcedureOrigin(t *testing.T) {
	c, mock, _ := newSamplingCheck(t)
	c.connectedToPdb = true
	mock.ExpectQuery("FROM dba_objects").WithArgs(int64(42)).WillReturnRows(procedureObjectRows(42, "CHILD"))
	resolver := procedureResolver{c: c, ctx: context.Background(), objects: make(map[procedureObjectKey]*procedureObject)}
	object, err := resolver.cursorOrigin(procedureCursor{ProgramID: 42})
	require.NoError(t, err)
	require.NotNil(t, object)
	assert.Equal(t, "CHILD", object.Name)
	require.NoError(t, mock.ExpectationsWereMet())
}
