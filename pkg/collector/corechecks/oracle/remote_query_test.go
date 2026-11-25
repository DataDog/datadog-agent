// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build oracle

package oracle

import (
	"bytes"
	"compress/gzip"
	"context"
	"database/sql"
	"encoding/csv"
	"encoding/json"
	"errors"
	"io"
	"math"
	"net"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	go_ora "github.com/sijms/go-ora/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/collector/check"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/oracle/config"
	"github.com/DataDog/datadog-agent/pkg/collector/remotequeries/session"
)

const (
	testCrawlID  = "7c4e2b1a-9f3d-4c8e-b6a5-1d2f3e4a5b6c"
	testTaskID   = "d25e7f9f-6883-4462-b31e-b449f70c0894"
	testUploadID = "6f1c2a9e-3b7d-4f0a-9c84-2d5e1b7a0c3f"
	testPassword = "s3cret-pw"
	testAPIKey   = "test-api-key"
	testHostname = "agent-host-1"
	testQuery    = "SELECT id, label FROM rq_probe ORDER BY id"
)

type recordedRequest struct {
	Method string
	Path   string
	Header http.Header
	Body   []byte
}

// stubIntake is a recording its-agent-intake stand-in that answers every request 204,
// or the statuses queued for "METHOD suffix". It records gzip pages uncompressed.
type stubIntake struct {
	srv      *httptest.Server
	mu       sync.Mutex
	requests []recordedRequest
	statuses map[string][]int
}

func newStubIntake(t *testing.T) *stubIntake {
	s := &stubIntake{statuses: map[string][]int{}}
	s.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		if r.Header.Get("Content-Encoding") == "gzip" {
			if zr, err := gzip.NewReader(bytes.NewReader(body)); err == nil {
				body, _ = io.ReadAll(zr)
			}
		}
		suffix := strings.TrimPrefix(r.URL.Path, "/api/unstable/its-agent-intake/uploads/"+testUploadID)
		s.mu.Lock()
		s.requests = append(s.requests, recordedRequest{Method: r.Method, Path: suffix, Header: r.Header.Clone(), Body: body})
		status := http.StatusNoContent
		if queue := s.statuses[r.Method+" "+suffix]; len(queue) > 0 {
			status, s.statuses[r.Method+" "+suffix] = queue[0], queue[1:]
		}
		s.mu.Unlock()
		w.WriteHeader(status)
		if status != http.StatusNoContent {
			_, _ = io.WriteString(w, `{"error":{"code":"invalid_argument","message":"bad"}}`)
		}
	}))
	t.Cleanup(s.srv.Close)
	return s
}

func (s *stubIntake) base() string { return s.srv.URL + "/api/unstable/its-agent-intake" }

func (s *stubIntake) recorded() []recordedRequest {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]recordedRequest(nil), s.requests...)
}

func (s *stubIntake) sequence() []string {
	var seq []string
	for _, r := range s.recorded() {
		seq = append(seq, r.Method+" "+r.Path)
	}
	return seq
}

// abortReason is the failure the last abort carried.
func (s *stubIntake) abortReason(t *testing.T) session.Failure {
	t.Helper()
	reqs := s.recorded()
	require.NotEmpty(t, reqs)
	last := reqs[len(reqs)-1]
	require.Equal(t, "POST /abort", last.Method+" "+last.Path)
	var body struct {
		Error session.Failure `json:"error"`
	}
	require.NoError(t, json.Unmarshal(last.Body, &body))
	return body.Error
}

type remoteFixture struct {
	check    *Check
	intake   *stubIntake
	mock     sqlmock.Sqlmock
	db       *sql.DB
	opens    atomic.Int32
	killed   atomic.Int32
	openHook func(cfg config.ConnectionConfig, timeout time.Duration) (*remoteDB, error)
	lastCfg  config.ConnectionConfig
	lastTime time.Duration
}

func newRemoteFixture(t *testing.T) *remoteFixture {
	t.Helper()
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	f := &remoteFixture{intake: newStubIntake(t), mock: mock, db: db}
	f.check = &Check{
		config: &config.CheckConfig{InstanceConfig: config.InstanceConfig{ConnectionConfig: config.ConnectionConfig{
			Server: "db.example.com", Port: 1521, ServiceName: "FREEPDB1", Username: "rq_reader", Password: testPassword,
		}}},
		agentHostname: testHostname,
	}
	f.check.remoteDeps = &remoteDependencies{
		openDB: func(cfg config.ConnectionConfig, timeout time.Duration) (*remoteDB, error) {
			f.opens.Add(1)
			f.lastCfg, f.lastTime = cfg, timeout
			if f.openHook != nil {
				return f.openHook(cfg, timeout)
			}
			return &remoteDB{DB: f.db, kill: func() { f.killed.Add(1) }}, nil
		},
		apiKey:     func() string { return testAPIKey },
		httpClient: func() (*http.Client, func()) { return f.intake.srv.Client(), func() {} },
	}
	return f
}

// expectStart expects the session settings, the read-only transaction, and the query.
func (f *remoteFixture) expectStart(query string) *sqlmock.ExpectedQuery {
	f.mock.ExpectExec(regexp.QuoteMeta(remoteSessionSettings)).WillReturnResult(sqlmock.NewResult(0, 0))
	f.mock.ExpectBegin()
	f.mock.ExpectExec(regexp.QuoteMeta("SET TRANSACTION READ ONLY")).WillReturnResult(sqlmock.NewResult(0, 0))
	return f.mock.ExpectQuery(regexp.QuoteMeta(query))
}

func (f *remoteFixture) expectQuery(rows *sqlmock.Rows) {
	f.expectStart(testQuery).WillReturnRows(rows)
	f.mock.ExpectRollback()
}

func probeRows(values ...[2]any) *sqlmock.Rows {
	rows := sqlmock.NewRowsWithColumnDefinition(
		sqlmock.NewColumn("ID").OfType("NUMBER", ""),
		sqlmock.NewColumn("LABEL").OfType("NCHAR", ""),
	)
	for _, v := range values {
		rows.AddRow(v[0], v[1])
	}
	return rows
}

func (f *remoteFixture) execute(t *testing.T, edit func(map[string]any)) string {
	t.Helper()
	out, err := f.check.RemoteQuery(context.Background(), executeRequest(f.intake.base(), edit))
	require.NoError(t, err)
	return out
}

func executeRequest(base string, edit func(map[string]any)) string {
	req := map[string]any{
		"operation": "execute", "target": map[string]any{"host": "db.example.com", "port": 1521, "dbname": "FREEPDB1"},
		"query": testQuery, "includeSchema": false,
		"resultDelivery": map[string]any{"crawlId": testCrawlID, "taskId": testTaskID, "uploadId": testUploadID, "baseUrl": base, "limits": map[string]any{
			"maxFileBytes": 16777216, "maxResultBytes": 107374182400, "maxRowBytes": 16777216, "maxColumns": 1024,
			"maxSchemaBytes": 1048576, "maxPages": 1280, "timeoutMs": 60000,
		}},
	}
	if edit != nil {
		edit(req)
	}
	out, _ := json.Marshal(req)
	return string(out)
}

func withQuery(q string) func(map[string]any) { return func(r map[string]any) { r["query"] = q } }

func withTimeout(ms int) func(map[string]any) {
	return func(r map[string]any) {
		r["resultDelivery"].(map[string]any)["limits"].(map[string]any)["timeoutMs"] = ms
	}
}

func requireFailure(t *testing.T, raw, code string) {
	t.Helper()
	var a struct {
		Status string
		Error  *session.Failure
	}
	require.NoError(t, json.Unmarshal([]byte(raw), &a), raw)
	require.Equal(t, "FAILED", a.Status, raw)
	require.NotNil(t, a.Error, raw)
	require.Equal(t, code, a.Error.Code, raw)
	for _, secret := range []string{testPassword, testAPIKey, testQuery, "rq_probe", "db.example.com", "rq_reader"} {
		require.NotContains(t, raw, secret, "an answer must not leak %q", secret)
	}
}

func resolve(t *testing.T, c *Check, target map[string]any) string {
	t.Helper()
	req, _ := json.Marshal(map[string]any{"operation": "resolve", "target": target})
	out, err := c.RemoteQuery(context.Background(), string(req))
	require.NoError(t, err)
	return out
}

// REQ-01, REQ-02
func TestREQ02_InvalidRequestsFailWithoutIO(t *testing.T) {
	f := newRemoteFixture(t)
	for _, raw := range []string{`{`, `{"operation":"execute"}`, executeRequest(f.intake.base(), func(r map[string]any) { r["extra"] = 1 })} {
		out, err := f.check.RemoteQuery(context.Background(), raw)
		require.NoError(t, err)
		requireFailure(t, out, session.CodeInvalidRequest)
	}
	assert.Zero(t, f.opens.Load())
	assert.Empty(t, f.intake.recorded())
}

// REQ-03
func TestREQ03_RequireTheAgentsAPIKey(t *testing.T) {
	f := newRemoteFixture(t)
	f.check.remoteDeps.apiKey = func() string { return "" }
	requireFailure(t, f.execute(t, nil), session.CodeCredentialsUnavailable)
	assert.Zero(t, f.opens.Load())
}

// MATCH-01, MATCH-03
func TestMATCH01_MatchAHostPortAndServiceTarget(t *testing.T) {
	target := map[string]any{"host": "db.example.com", "port": 1521, "dbname": "FREEPDB1"}
	for name, tc := range map[string]struct {
		cfg    func(*config.ConnectionConfig)
		target map[string]any
		match  bool
	}{
		"exact":                         {nil, target, true},
		"host case, spaces, and dot":    {nil, map[string]any{"host": " DB.Example.COM. ", "port": 1521, "dbname": "FREEPDB1"}, true},
		"service case and spaces":       {nil, map[string]any{"host": "db.example.com", "port": 1521, "dbname": " freepdb1 "}, true},
		"configured host case":          {func(c *config.ConnectionConfig) { c.Server = "DB.example.com." }, target, true},
		"TCPS with a wallet":            {func(c *config.ConnectionConfig) { c.Protocol, c.Wallet = "TCPS", "/wallet" }, target, true},
		"TNS admin without an alias":    {func(c *config.ConnectionConfig) { c.TnsAdmin = "/tns" }, target, true},
		"other host":                    {nil, map[string]any{"host": "db2.example.com", "port": 1521, "dbname": "FREEPDB1"}, false},
		"other port":                    {nil, map[string]any{"host": "db.example.com", "port": 1522, "dbname": "FREEPDB1"}, false},
		"other service":                 {nil, map[string]any{"host": "db.example.com", "port": 1521, "dbname": "XEPDB1"}, false},
		"no service":                    {func(c *config.ConnectionConfig) { c.ServiceName = " " }, target, false},
		"TNS alias":                     {func(c *config.ConnectionConfig) { c.TnsAlias = "prod" }, target, false},
		"Oracle Client":                 {func(c *config.ConnectionConfig) { c.OracleClient = true }, target, false},
		"database instance not yet set": {nil, map[string]any{"database_instance": "orders"}, false},
	} {
		f := newRemoteFixture(t)
		if tc.cfg != nil {
			tc.cfg(&f.check.config.ConnectionConfig)
		}
		out := resolve(t, f.check, tc.target)
		if tc.match {
			assert.JSONEq(t, `{"status":"MATCHED"}`, out, name)
		} else {
			assert.Contains(t, out, `"FAILED"`, name)
		}
		assert.Zero(t, f.opens.Load(), "%s: matching never connects", name)
		assert.Empty(t, f.intake.recorded(), name)
	}

	// The host:port shorthand is the same server and port to the check.
	cfg, err := config.NewCheckConfig([]byte("username: u\nserver: db.example.com:1522\nservice_name: FREEPDB1\n"), nil)
	require.NoError(t, err)
	f := newRemoteFixture(t)
	f.check.config = cfg
	assert.JSONEq(t, `{"status":"MATCHED"}`, resolve(t, f.check, map[string]any{"host": "db.example.com", "port": 1522, "dbname": "FREEPDB1"}))
}

// MATCH-02, MATCH-04
func TestMATCH02_MatchADatabaseInstanceTarget(t *testing.T) {
	f := newRemoteFixture(t)
	requireFailure(t, resolve(t, f.check, map[string]any{"database_instance": "orders"}), session.CodeTargetUnavailable)
	f.check.publishRemoteIdentifier("orders")
	assert.JSONEq(t, `{"status":"MATCHED"}`, resolve(t, f.check, map[string]any{"database_instance": "orders"}))
	requireFailure(t, resolve(t, f.check, map[string]any{"database_instance": "Orders"}), session.CodeTargetNotFound)
	f.check.config.TnsAlias = "prod"
	requireFailure(t, resolve(t, f.check, map[string]any{"database_instance": "orders"}), session.CodeTargetNotFound)
	assert.Zero(t, f.opens.Load())
}

// MATCH-05
func TestMATCH05_MatchAgainBeforeExecuting(t *testing.T) {
	f := newRemoteFixture(t)
	f.check.config.TnsAlias = "prod"
	requireFailure(t, f.execute(t, nil), session.CodeTargetNotFound)
	assert.Zero(t, f.opens.Load())
	assert.Equal(t, session.CodeTargetNotFound, f.intake.abortReason(t).Code)
}

// QUERY-01, QUERY-02, DESC-04, FINISH-02; the statement order of QUERY-03 and QUERY-04
func TestQUERY01_ConnectAsTheCheckOnARunConnection(t *testing.T) {
	f := newRemoteFixture(t)
	f.check.config.Protocol, f.check.config.Wallet = "TCPS", "/wallet"
	f.expectQuery(probeRows([2]any{"1", "alpha"}, [2]any{"12345678901234567890.123456789012345678", "he said \"hi\"\nthen ✓"}, [2]any{nil, nil}))
	assert.JSONEq(t, `{"status":"SUCCEEDED"}`, f.execute(t, nil))
	require.NoError(t, f.mock.ExpectationsWereMet(), "settings, then SET TRANSACTION READ ONLY first, then the query, then a rollback")
	assert.Equal(t, f.check.config.ConnectionConfig, f.lastCfg, "the run connects with the check's own settings, TLS included")
	assert.Greater(t, f.lastTime, 50*time.Second, "the driver's read timeout follows the run's database time")
	assert.Equal(t, []string{"POST /descriptor", "PUT /pages/0", "POST /finalize"}, f.intake.sequence())
	page := f.intake.recorded()[1]
	records, err := csv.NewReader(strings.NewReader(string(page.Body))).ReadAll()
	require.NoError(t, err)
	assert.Equal(t, [][]string{{`1`, `"alpha"`}, {`12345678901234567890.123456789012345678`, `"he said \"hi\"\nthen ✓"`}, {`null`, `null`}}, records)

	// A query that is not SELECT or WITH never reaches the database.
	for _, q := range []string{"CREATE TABLE t (n NUMBER)", "BEGIN NULL; END;", "WITH FUNCTION f RETURN NUMBER IS BEGIN RETURN 1; END; SELECT f FROM dual"} {
		f := newRemoteFixture(t)
		requireFailure(t, f.execute(t, withQuery(q)), codeUnsupportedQuery)
		assert.Zero(t, f.opens.Load(), q)
	}
}

// QUERY-02
func TestQUERY02_RunOnlyQueries(t *testing.T) {
	for _, ok := range []string{
		"SELECT 1 FROM dual", "  select * from t", "WITH a AS (SELECT 1 x FROM dual) SELECT x FROM a", "(SELECT 1 FROM dual) UNION (SELECT 2 FROM dual)",
		"-- note\nSELECT 1 FROM dual", "/* c */ /*+ hint */ SELECT 1 FROM dual", "select*from dual", `WITH "FUNCTION" AS (SELECT 1 FROM dual) SELECT * FROM "FUNCTION"`,
	} {
		assert.True(t, remoteQueryIsSelect(ok), ok)
	}
	for _, bad := range []string{
		"", "   ", "-- SELECT", "/* SELECT", "INSERT INTO t VALUES (1)", "DELETE FROM t", "CREATE TABLE t (n NUMBER)", "DROP TABLE t",
		"ALTER SESSION SET x = 1", "BEGIN NULL; END;", "DECLARE x NUMBER; BEGIN NULL; END;", "CALL p()", "GRANT SELECT ON t TO u",
		"MERGE INTO t USING s ON (1=1) WHEN MATCHED THEN UPDATE SET n = 1", "WITH FUNCTION f RETURN NUMBER IS BEGIN RETURN 1; END; SELECT f FROM dual",
		"with /* x */ procedure p IS BEGIN NULL; END; SELECT 1 FROM dual", "SELECTX 1", "WITH", "EXPLAIN PLAN FOR SELECT 1 FROM dual", "LOCK TABLE t IN EXCLUSIVE MODE",
	} {
		assert.False(t, remoteQueryIsSelect(bad), bad)
	}
}

// QUERY-05, CONFIG-01
func TestQUERY05_BoundTheDatabaseTime(t *testing.T) {
	f := newRemoteFixture(t)
	timeout := 100
	f.check.config.RemoteQueries.TimeoutMs = &timeout
	f.expectStart(testQuery).WillDelayFor(time.Hour).WillReturnRows(probeRows())
	start := time.Now()
	requireFailure(t, f.execute(t, nil), session.CodeTimeout)
	assert.Less(t, time.Since(start), 10*time.Second, "the instance setting shortens the run's 60 s")
	assert.LessOrEqual(t, f.lastTime, 100*time.Millisecond, "and the driver's read timeout with it")
	assert.Equal(t, session.CodeTimeout, f.intake.abortReason(t).Code)
}

// QUERY-06, OBS-01
func TestQUERY06_ReportDatabaseErrorsByTheirCode(t *testing.T) {
	f := newRemoteFixture(t)
	f.expectStart(testQuery).WillReturnError(errors.New("ORA-00942: table or view does not exist rq_probe for rq_reader/" + testPassword + "@db.example.com:1521"))
	out := f.execute(t, nil)
	requireFailure(t, out, session.CodeQueryFailed)
	assert.Contains(t, out, "ORA-00942")
	assert.Equal(t, []string{"POST /abort"}, f.intake.sequence())

	f = newRemoteFixture(t)
	f.openHook = func(config.ConnectionConfig, time.Duration) (*remoteDB, error) {
		return nil, errors.New("dial tcp db.example.com:1521 user rq_reader password " + testPassword)
	}
	requireFailure(t, f.execute(t, nil), session.CodeConnectionFailed)
}

// DESC-02, ORA-01
func TestDESC02_NameEachColumnsTypes(t *testing.T) {
	for _, tc := range []struct {
		driver           string
		precision, scale int64
		hasScale         bool
		dbType, logical  string
	}{
		{"NUMBER", 38, 255, true, "NUMBER", "decimal"},
		{"NUMBER", 10, 0, true, "NUMBER(10)", "decimal"},
		{"NUMBER", 5, 2, true, "NUMBER(5,2)", "decimal"},
		{"NUMBER", 0, 0, false, "NUMBER", "decimal"},
		{"IBFloat", 0, 0, false, "BINARY_FLOAT", "float"},
		{"IBDouble", 0, 0, false, "BINARY_DOUBLE", "float"},
		{"NCHAR", 0, 0, false, "VARCHAR2", "string"},
		{"CHAR", 0, 0, false, "CHAR", "string"},
		{"ROWID", 0, 0, false, "ROWID", "string"},
		{"DATE", 0, 0, false, "DATE", "temporal"},
		{"TimeStampDTY", 0, 0, false, "TIMESTAMP", "temporal"},
		{"TimeStampTZ_DTY", 0, 0, false, "TIMESTAMP WITH TIME ZONE", "temporal"},
		{"TimeStampLTZ_DTY", 0, 0, false, "TIMESTAMP WITH LOCAL TIME ZONE", "temporal"},
		{"IntervalYM_DTY", 0, 0, false, "INTERVAL YEAR TO MONTH", "temporal"},
		{"IntervalDS_DTY", 0, 0, false, "INTERVAL DAY TO SECOND", "temporal"},
		{"RAW", 0, 0, false, "RAW", "binary"},
	} {
		col, failure := remoteColumnType("C", tc.driver, tc.precision, tc.scale, tc.hasScale)
		require.Nil(t, failure, tc.driver)
		assert.Equal(t, tc.dbType, col.DatabaseType, tc.driver)
		assert.Equal(t, tc.logical, col.LogicalType, tc.driver)
	}
	for _, driver := range []string{"LongVarChar", "LONG", "OCIClobLocator", "LongRaw", "OCIBlobLocator", "OCIFileLocator", "XMLType", "VECTOR", ""} {
		_, failure := remoteColumnType("PAYLOAD", driver, 0, 0, false)
		require.NotNil(t, failure, driver)
		assert.Equal(t, session.CodeUnsupportedColumn, failure.Code, driver)
		assert.Contains(t, failure.Message, `"PAYLOAD"`, "the error names the column")
	}

	f := newRemoteFixture(t)
	f.expectStart(testQuery).WillReturnRows(sqlmock.NewRowsWithColumnDefinition(
		sqlmock.NewColumn("ID").OfType("NUMBER", ""), sqlmock.NewColumn("DOC").OfType("OCIClobLocator", "")).AddRow("1", "x"))
	f.mock.ExpectRollback()
	requireFailure(t, f.execute(t, nil), session.CodeUnsupportedColumn)
	assert.Equal(t, []string{"POST /abort"}, f.intake.sequence(), "nothing is uploaded before the columns are known to fit")
}

// ORA-02
func TestORA02_EncodeValuesByColumnType(t *testing.T) {
	tz := time.FixedZone("", 5*3600+30*60)
	ts := time.Date(2024, 2, 29, 12, 34, 56, 123456000, time.UTC)
	for _, tc := range []struct {
		kind  remoteKind
		value any
		token string
	}{
		{kindDecimal, nil, `null`},
		{kindDecimal, "9007199254740993", `9007199254740993`},
		{kindDecimal, []byte("-0.001"), `-0.001`},
		{kindDecimal, int64(math.MinInt64), `-9223372036854775808`},
		{kindFloat, float32(1.25), `1.25`},
		{kindFloat, float64(0.1), `0.1`},
		{kindFloat, float32(math.NaN()), `"NaN"`},
		{kindFloat, math.Inf(1), `"Infinity"`},
		{kindFloat, math.Inf(-1), `"-Infinity"`},
		{kindString, "ab ", `"ab "`},
		{kindString, "<&>\u2028é", `"<&>\u2028é"`},
		{kindDate, time.Date(2024, 2, 29, 0, 0, 0, 0, time.UTC), `"2024-02-29T00:00:00"`},
		{kindTimestamp, ts, `"2024-02-29T12:34:56.123456"`},
		{kindTimestamp, ts.Truncate(time.Second), `"2024-02-29T12:34:56"`},
		{kindTimestampTZ, time.Date(2024, 2, 29, 12, 34, 56, 500000000, tz), `"2024-02-29T12:34:56.5+05:30"`},
		{kindTimestampUTC, time.Date(2024, 2, 29, 18, 4, 56, 0, tz), `"2024-02-29T12:34:56Z"`},
		{kindInterval, "+03 04:05:06.700000", `"+03 04:05:06.700000"`},
		{kindBinary, []byte{0xde, 0xad, 0xbe, 0xef}, `"\\xdeadbeef"`},
	} {
		cell, failure := remoteCell(tc.kind, tc.value)
		require.Nil(t, failure, "%v", tc.value)
		assert.Equal(t, tc.token, cell.Token, "%v", tc.value)
	}
	for _, tc := range []struct {
		kind  remoteKind
		value any
	}{
		{kindDecimal, float64(0.1)}, {kindDecimal, "NaN"}, {kindDecimal, "007"}, {kindString, "bad\xff"}, {kindString, int64(5)},
		{kindDate, time.Date(0, 1, 1, 0, 0, 0, 0, time.UTC)}, {kindBinary, "dead"}, {kindFloat, "1.5"},
	} {
		_, failure := remoteCell(tc.kind, tc.value)
		require.NotNil(t, failure, "%v", tc.value)
		assert.Equal(t, codeUnsupportedValue, failure.Code)
	}
	// go-ora decodes NUMBER to its exact decimal text.
	for _, text := range []string{"0", "-1", "0.5", "9007199254740993", "99999999999999999999999999999999999999", "123456789012345678901234567890.123456789"} {
		number, err := go_ora.NewNumberFromString(text)
		require.NoError(t, err)
		got, err := number.String()
		require.NoError(t, err)
		cell, failure := remoteCell(kindDecimal, got)
		require.Nil(t, failure)
		assert.Equal(t, text, cell.Token)
	}
}

// TIME-01, ABORT-01
func TestTIME01_BoundTheRunByItsTimeLimit(t *testing.T) {
	f := newRemoteFixture(t)
	f.expectStart(testQuery).WillDelayFor(time.Hour).WillReturnRows(probeRows())
	requireFailure(t, f.execute(t, withTimeout(100)), session.CodeTimeout)
	assert.Equal(t, []string{"POST /abort"}, f.intake.sequence())
	assert.Equal(t, session.Failure{Code: session.CodeTimeout, Message: "the remote query exceeded its time limit"}, f.intake.abortReason(t))
	assert.Positive(t, f.killed.Load(), "the run's sockets are closed when it ends")
}

// ABORT-01
func TestABORT01_AbortTheUploadOfAFailedRun(t *testing.T) {
	f := newRemoteFixture(t)
	f.intake.statuses["POST /finalize"] = []int{http.StatusBadRequest}
	f.expectQuery(probeRows([2]any{"1", "x"}))
	requireFailure(t, f.execute(t, nil), session.CodeUploadFailed)
	assert.Equal(t, []string{"POST /descriptor", "PUT /pages/0", "POST /finalize", "POST /abort"}, f.intake.sequence())
	assert.Equal(t, session.Failure{Code: session.CodeUploadFailed, Message: "intake request failed: invalid_argument"}, f.intake.abortReason(t))

	f = newRemoteFixture(t)
	f.expectQuery(probeRows([2]any{"1", "bad\xff"}))
	requireFailure(t, f.execute(t, nil), codeUnsupportedValue)
	reason := f.intake.abortReason(t)
	assert.Equal(t, codeUnsupportedValue, reason.Code)
	assert.Contains(t, reason.Message, `"LABEL"`)
	assert.NotContains(t, f.intake.sequence(), "PUT /pages/0")
}

// CANCEL-03
func TestCANCEL03_TearDownOnlyAfterRunsEnd(t *testing.T) {
	f := newRemoteFixture(t)
	f.expectStart(testQuery).WillDelayFor(time.Hour).WillReturnRows(probeRows())
	answers := make(chan string, 1)
	go func() {
		out, err := f.check.RemoteQuery(context.Background(), executeRequest(f.intake.base(), nil))
		assert.NoError(t, err)
		answers <- out
	}()
	require.Eventually(t, func() bool { return f.opens.Load() == 1 }, 5*time.Second, 5*time.Millisecond)
	done := make(chan struct{})
	go func() { f.check.Cancel(); close(done) }()
	select {
	case <-done:
	case <-time.After(10 * time.Second):
		t.Fatal("Cancel did not return")
	}
	select {
	case out := <-answers:
		requireFailure(t, out, session.CodeCancelled)
	default:
		t.Fatal("Cancel returned before the run ended")
	}
	assert.Equal(t, []string{"POST /abort"}, f.intake.sequence())
	assert.Equal(t, session.CodeCancelled, f.intake.abortReason(t).Code)
}

// CANCEL-02
func TestCANCEL02_RefuseRequestsOnAnUnscheduledCheck(t *testing.T) {
	f := newRemoteFixture(t)
	f.check.Cancel()
	for _, req := range []string{`{"operation":"resolve","target":{"host":"db.example.com","port":1521,"dbname":"FREEPDB1"}}`, executeRequest(f.intake.base(), nil), `{`} {
		_, err := f.check.RemoteQuery(context.Background(), req)
		assert.ErrorIs(t, err, check.ErrRemoteQueryCancelled)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := newRemoteFixture(t).check.RemoteQuery(ctx, executeRequest(f.intake.base(), nil))
	assert.ErrorIs(t, err, context.Canceled, "no call starts on an ended context")
	assert.Zero(t, f.opens.Load())
	assert.Empty(t, f.intake.recorded())
}

func TestSupportsRemoteQueriesNeedsAConfiguredCheck(t *testing.T) {
	assert.False(t, (&Check{}).SupportsRemoteQueries())
	assert.True(t, newRemoteFixture(t).check.SupportsRemoteQueries())
}

// The socket fallback of CANCEL-01: closing the sockets stops a driver that does not return.
func TestSocketTrackerClosesEverySocket(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	defer ln.Close()
	accepted := make(chan net.Conn, 1)
	go func() {
		c, _ := ln.Accept()
		accepted <- c
	}()
	tracker := &socketTracker{}
	conn, err := tracker.DialContext(context.Background(), "tcp", ln.Addr().String())
	require.NoError(t, err)
	srv := <-accepted
	defer srv.Close()
	read := make(chan error, 1)
	go func() {
		_, err := conn.Read(make([]byte, 1))
		read <- err
	}()
	time.Sleep(20 * time.Millisecond)
	tracker.closeAll()
	select {
	case err := <-read:
		require.Error(t, err, "a blocked read ends when the socket is closed")
	case <-time.After(5 * time.Second):
		t.Fatal("blocked read did not end")
	}
	_, err = tracker.DialContext(context.Background(), "tcp", ln.Addr().String())
	assert.Error(t, err, "no socket opens after the run was torn down")
}

// OBS-01
func TestOBS01_UnexpectedFailuresAreInternalErrors(t *testing.T) {
	f := newRemoteFixture(t)
	f.openHook = func(config.ConnectionConfig, time.Duration) (*remoteDB, error) {
		panic("boom rq_probe " + testPassword)
	}
	requireFailure(t, f.execute(t, nil), session.CodeInternalError)
	assert.Equal(t, []string{"POST /abort"}, f.intake.sequence())
}
