// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build oracle_test

package oracle

import (
	"context"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.yaml.in/yaml/v3"

	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/oracle/config"
	"github.com/DataDog/datadog-agent/pkg/collector/remotequeries/session"
)

// These tests run the remote query path against the test database, with the real
// go-ora driver, and a stub intake. They create their own fixture users with the
// SYS connection of the other integration tests.

const rqFixturePassword = "RqFixture_2026"

type rqFixture struct {
	owner, reader string
	connection    config.ConnectionConfig
}

var (
	rqFixtureOnce sync.Once
	rqFixtureMade rqFixture
)

// setupRemoteQueryFixture creates the fixture once per test binary.
func setupRemoteQueryFixture(t *testing.T) rqFixture {
	t.Helper()
	rqFixtureOnce.Do(func() { rqFixtureMade = createRemoteQueryFixture(t) })
	require.NotEmpty(t, rqFixtureMade.owner, "fixture setup failed")
	return rqFixtureMade
}

func createRemoteQueryFixture(t *testing.T) rqFixture {
	t.Helper()
	db, err := getSysConnection(t)
	require.NoError(t, err)
	defer db.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	var container string
	require.NoError(t, db.QueryRowContext(ctx, "SELECT SYS_CONTEXT('USERENV', 'CON_NAME') FROM dual").Scan(&container))
	prefix := ""
	if container == "CDB$ROOT" {
		prefix = "C##" // common users in the root container
	}
	f := rqFixture{owner: prefix + "RQ_OWNER", reader: prefix + "RQ_READER", connection: getConnectData(t, useSysUser)}
	exec := func(statement string, args ...any) {
		t.Helper()
		_, err := db.ExecContext(ctx, statement, args...)
		require.NoError(t, err, statement)
	}
	for _, user := range []string{f.reader, f.owner} {
		exec(fmt.Sprintf(`BEGIN EXECUTE IMMEDIATE 'DROP USER %s CASCADE'; EXCEPTION WHEN OTHERS THEN IF SQLCODE != -1918 THEN RAISE; END IF; END;`, user))
	}
	exec(fmt.Sprintf(`CREATE USER %s IDENTIFIED BY "%s" QUOTA UNLIMITED ON USERS`, f.owner, rqFixturePassword))
	exec("GRANT CREATE SESSION, CREATE TABLE TO " + f.owner)
	exec(fmt.Sprintf(`CREATE USER %s IDENTIFIED BY "%s"`, f.reader, rqFixturePassword))
	exec("GRANT CREATE SESSION TO " + f.reader)
	exec(fmt.Sprintf(`CREATE TABLE %s.RQ_TYPES (ID NUMBER, N NUMBER, N52 NUMBER(5,2), BF BINARY_FLOAT, BD BINARY_DOUBLE, C CHAR(3), V VARCHAR2(40), NV NVARCHAR2(40),
		D DATE, TS TIMESTAMP(6), TSTZ TIMESTAMP(6) WITH TIME ZONE, TSLTZ TIMESTAMP WITH LOCAL TIME ZONE, IYM INTERVAL YEAR TO MONTH,
		IDS INTERVAL DAY TO SECOND, R RAW(16), DOC CLOB) TABLESPACE USERS`, f.owner))
	exec(fmt.Sprintf(`INSERT INTO %s.RQ_TYPES VALUES (1, 12345678901234567890.123456789012345678, 123.45, 1.25, 2.5, 'ab', :1, :2,
		DATE '2024-02-29', TIMESTAMP '2024-02-29 12:34:56.123456', TIMESTAMP '2024-02-29 12:34:56.5 +05:30', TIMESTAMP '2024-02-29 12:34:56 +00:00',
		INTERVAL '1-2' YEAR TO MONTH, INTERVAL '3 04:05:06.7' DAY TO SECOND, HEXTORAW('DEADBEEF'), 'doc')`, f.owner), `naïve "quoted", ✓`+"\n", "wörld")
	exec(fmt.Sprintf(`INSERT INTO %s.RQ_TYPES (ID, BF, BD) VALUES (2, BINARY_FLOAT_NAN, -BINARY_DOUBLE_INFINITY)`, f.owner))
	exec("COMMIT")
	exec(fmt.Sprintf(`GRANT SELECT ON %s.RQ_TYPES TO %s`, f.owner, f.reader))
	// A read-only transaction cannot read a table whose definition is newer than its
	// snapshot, at the database's few-seconds time resolution (ORA-01466).
	time.Sleep(6 * time.Second)
	return f
}

func (f rqFixture) check(t *testing.T, user string, intake *stubIntake, instance string) *Check {
	t.Helper()
	connection := f.connection
	connection.Username, connection.Password = user, rqFixturePassword
	raw, err := yaml.Marshal(connection)
	require.NoError(t, err)
	cfg, err := config.NewCheckConfig(append(raw, []byte("\n"+instance)...), nil)
	require.NoError(t, err)
	c := &Check{config: cfg, agentHostname: testHostname, remoteDeps: defaultRemoteDependencies()}
	c.remoteDeps.apiKey = func() string { return testAPIKey }
	c.remoteDeps.httpClient = func() (*http.Client, func()) { return intake.srv.Client(), func() {} }
	return c
}

func (f rqFixture) execute(t *testing.T, c *Check, intake *stubIntake, query string, edits ...func(map[string]any)) string {
	t.Helper()
	out, err := c.RemoteQuery(context.Background(), executeRequest(intake.base(), func(r map[string]any) {
		r["target"] = map[string]any{"host": f.connection.Server, "port": f.connection.Port, "dbname": f.connection.ServiceName}
		r["query"] = query
		for _, e := range edits {
			e(r)
		}
	}))
	require.NoError(t, err)
	return out
}

func pageRecords(t *testing.T, intake *stubIntake) ([][]string, []recordedRequest) {
	t.Helper()
	var all [][]string
	var pages []recordedRequest
	for _, r := range intake.recorded() {
		if r.Method != http.MethodPut {
			continue
		}
		pages = append(pages, r)
		records, err := csv.NewReader(strings.NewReader(string(r.Body))).ReadAll()
		require.NoError(t, err)
		all = append(all, records...)
	}
	return all, pages
}

// QUERY-04, DESC-01; and the driver types and values of DESC-02 and ORA-02.
func TestQUERY04_LiveFixesTheSessionSettings(t *testing.T) {
	f := setupRemoteQueryFixture(t)
	intake := newStubIntake(t)
	c := f.check(t, f.reader, intake, "")
	out := f.execute(t, c, intake, `SELECT N, N52, BF, BD, C, V, NV, D, TS, TSTZ, TSLTZ, IYM, IDS, R, TO_CHAR(D) AS D_TEXT
		FROM `+f.owner+`.RQ_TYPES ORDER BY ID`)
	require.JSONEq(t, `{"status":"SUCCEEDED"}`, out)
	reqs := intake.recorded()
	require.Equal(t, []string{"POST /descriptor", "PUT /pages/0", "POST /finalize"}, intake.sequence())
	var descriptor struct {
		Columns []struct {
			Name    string `json:"column_name"`
			Type    string `json:"vendor_data_type"`
			Logical string `json:"logical_type"`
		} `json:"columns"`
	}
	require.NoError(t, json.Unmarshal(reqs[0].Body, &descriptor))
	var types []string
	for _, c := range descriptor.Columns {
		types = append(types, c.Name+" "+c.Type+" "+c.Logical)
	}
	assert.Equal(t, []string{
		"N NUMBER decimal", "N52 NUMBER(5,2) decimal", "BF BINARY_FLOAT float", "BD BINARY_DOUBLE float", "C CHAR string", "V VARCHAR2 string",
		"NV VARCHAR2 string", "D DATE temporal", "TS TIMESTAMP temporal", "TSTZ TIMESTAMP WITH TIME ZONE temporal",
		"TSLTZ TIMESTAMP WITH LOCAL TIME ZONE temporal", "IYM INTERVAL YEAR TO MONTH temporal", "IDS INTERVAL DAY TO SECOND temporal",
		"R RAW binary", "D_TEXT VARCHAR2 string",
	}, types)
	assert.Equal(t, "gzip", reqs[1].Header.Get("Content-Encoding"))
	records, _ := pageRecords(t, intake)
	assert.Equal(t, [][]string{
		{`12345678901234567890.123456789012345678`, `123.45`, `1.25`, `2.5`, `"ab "`, `"naïve \"quoted\", ✓\n"`, `"wörld"`,
			`"2024-02-29T00:00:00"`, `"2024-02-29T12:34:56.123456"`, `"2024-02-29T12:34:56.5+05:30"`, `"2024-02-29T12:34:56Z"`,
			`"+01-02"`, `"+03 04:05:06.700000"`, `"\\xdeadbeef"`, `"2024-02-29T00:00:00"`},
		{`null`, `null`, `"NaN"`, `"-Infinity"`, `null`, `null`, `null`, `null`, `null`, `null`, `null`, `null`, `null`, `null`, `null`},
	}, records, "exact numbers, the session's fixed text conversions (QUERY-03), and ISO temporal values")
}

// QUERY-03; and QUERY-02 keeping DDL off the database.
func TestQUERY03_LiveRunInOneReadOnlyTransaction(t *testing.T) {
	f := setupRemoteQueryFixture(t)
	intake := newStubIntake(t)
	c := f.check(t, f.owner, intake, "")
	out := f.execute(t, c, intake, "SELECT ID FROM "+f.owner+".RQ_TYPES FOR UPDATE")
	assert.Contains(t, out, `"query_failed"`)
	assert.Contains(t, out, "ORA-01456", "Oracle refused the lock in the read-only transaction")
	assert.Equal(t, session.CodeQueryFailed, intake.abortReason(t).Code)

	intake = newStubIntake(t)
	c = f.check(t, f.owner, intake, "")
	out = f.execute(t, c, intake, "DROP TABLE "+f.owner+".RQ_TYPES")
	assert.Contains(t, out, `"`+codeUnsupportedQuery+`"`)
	db, err := getSysConnection(t)
	require.NoError(t, err)
	defer db.Close()
	var n int
	require.NoError(t, db.QueryRow("SELECT COUNT(*) FROM "+f.owner+".RQ_TYPES").Scan(&n))
	assert.Equal(t, 2, n, "the table is untouched")
}

// Wiring of SIZE-01, SIZE-05, UPLOAD-08, and ORA-01 with the real driver.
func TestLiveWiring_PagesEmptyResultAndLOBs(t *testing.T) {
	f := setupRemoteQueryFixture(t)
	intake := newStubIntake(t)
	c := f.check(t, f.reader, intake, "")
	out := f.execute(t, c, intake, "SELECT LEVEL AS N, RPAD('x', 200, 'x') AS S FROM dual CONNECT BY LEVEL <= 5000", func(r map[string]any) {
		l := r["resultDelivery"].(map[string]any)["limits"].(map[string]any)
		l["maxFileBytes"], l["maxRowBytes"] = 256<<10, 256<<10
	})
	require.JSONEq(t, `{"status":"SUCCEEDED"}`, out)
	records, pages := pageRecords(t, intake)
	require.Greater(t, len(pages), 3)
	require.Len(t, records, 5000)
	var offset int
	for i, p := range pages {
		assert.Equal(t, "/pages/"+strconv.Itoa(i), p.Path)
		assert.Equal(t, strconv.Itoa(offset), p.Header.Get("X-DD-Record-Offset"))
		assert.Equal(t, "gzip", p.Header.Get("Content-Encoding"))
		n, _ := strconv.Atoi(p.Header.Get("X-DD-Source-Page-Rows"))
		offset += n
	}
	for i, rec := range records {
		require.Equal(t, strconv.Itoa(i+1), rec[0])
	}

	intake = newStubIntake(t)
	c = f.check(t, f.reader, intake, "")
	require.JSONEq(t, `{"status":"SUCCEEDED"}`, f.execute(t, c, intake, "SELECT ID FROM "+f.owner+".RQ_TYPES WHERE 1 = 0"))
	_, pages = pageRecords(t, intake)
	require.Len(t, pages, 1)
	assert.Equal(t, "0", pages[0].Header.Get("X-DD-Source-Page-Rows"))

	intake = newStubIntake(t)
	c = f.check(t, f.reader, intake, "")
	out = f.execute(t, c, intake, "SELECT ID, DOC FROM "+f.owner+".RQ_TYPES")
	assert.Contains(t, out, `"unsupported_column"`)
	intake = newStubIntake(t)
	c = f.check(t, f.reader, intake, "")
	require.JSONEq(t, `{"status":"SUCCEEDED"}`, f.execute(t, c, intake, "SELECT ID, DBMS_LOB.SUBSTR(DOC, 100) AS DOC FROM "+f.owner+".RQ_TYPES ORDER BY ID"))
}

// heavyQuery runs for minutes on the test database.
const heavyQuery = "SELECT COUNT(*) FROM all_objects a, all_objects b, all_objects c"

// CANCEL-01, with QUERY-05: the driver's cancel request stops the statement well
// before the run would close its sockets.
func TestCANCEL01_LiveStopACancelledRun(t *testing.T) {
	f := setupRemoteQueryFixture(t)

	intake := newStubIntake(t)
	c := f.check(t, f.reader, intake, "remote_queries:\n  timeout_ms: 1500\n")
	start := time.Now()
	out := f.execute(t, c, intake, heavyQuery)
	assert.Contains(t, out, `"timeout"`)
	assert.Less(t, time.Since(start), 1500*time.Millisecond+remoteCancelGrace, "the database stopped at the instance's time limit")

	intake = newStubIntake(t)
	c = f.check(t, f.reader, intake, "")
	answers := make(chan string, 1)
	go func() { answers <- f.execute(t, c, intake, heavyQuery) }()
	time.Sleep(1500 * time.Millisecond)
	cancelled := time.Now()
	c.Cancel()
	assert.Less(t, time.Since(cancelled), remoteCancelGrace, "Cancel returned once the database stopped the statement")
	assert.Contains(t, <-answers, `"cancelled"`)
	assert.Equal(t, session.CodeCancelled, intake.abortReason(t).Code)

	db, err := getSysConnection(t)
	require.NoError(t, err)
	defer db.Close()
	var sessions int
	require.NoError(t, db.QueryRow("SELECT COUNT(*) FROM v$session WHERE username = :1", f.reader).Scan(&sessions))
	assert.Zero(t, sessions, "the run's connection is closed")
}
