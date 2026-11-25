// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build oracle

package oracle

import (
	"context"
	"database/sql"
	"encoding/csv"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/collector/check"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/oracle/config"
)

const (
	testCrawlID  = "7c4e2b1a-9f3d-4c8e-b6a5-1d2f3e4a5b6c"
	testTaskID   = "d25e7f9f-6883-4462-b31e-b449f70c0894"
	testUploadID = "6f1c2a9e-3b7d-4f0a-9c84-2d5e1b7a0c3f"
	testPassword = "s3cret-pw"
	testAPIKey   = "test-api-key"
	testHostname = "agent-host-1"
)

type recordedRequest struct {
	Method  string
	Path    string
	Header  http.Header
	Body    []byte
	Headers http.Header
}

// stubIntake is a recording its-agent-intake stand-in. responses maps "METHOD suffix"
// (for example "PUT /pages/1") to a queue of statuses; an empty queue answers 204.
type stubIntake struct {
	srv       *httptest.Server
	mu        sync.Mutex
	requests  []recordedRequest
	responses map[string][]int
}

func newStubIntake(t *testing.T) *stubIntake {
	s := &stubIntake{responses: map[string][]int{}}
	s.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		suffix := strings.TrimPrefix(r.URL.Path, "/api/unstable/its-agent-intake/uploads/"+testUploadID)
		s.mu.Lock()
		s.requests = append(s.requests, recordedRequest{Method: r.Method, Path: suffix, Header: r.Header.Clone(), Body: body})
		key := r.Method + " " + suffix
		status := http.StatusNoContent
		if queue := s.responses[key]; len(queue) > 0 {
			status, s.responses[key] = queue[0], queue[1:]
		}
		s.mu.Unlock()
		switch status {
		case http.StatusNoContent:
			w.WriteHeader(status)
		case http.StatusServiceUnavailable:
			w.WriteHeader(status)
			_, _ = io.WriteString(w, `{"error":{"code":"unavailable","message":"busy"}}`)
		case http.StatusInternalServerError:
			w.WriteHeader(status)
			_, _ = io.WriteString(w, `{"error":{"code":"internal","message":"boom"}}`)
		case http.StatusRequestEntityTooLarge:
			w.WriteHeader(status)
			_, _ = io.WriteString(w, `{"error":{"code":"result_file_too_large","message":"big"}}`)
		default:
			w.WriteHeader(status)
			_, _ = io.WriteString(w, `{"error":{"code":"invalid_argument","message":"bad"}}`)
		}
	}))
	t.Cleanup(s.srv.Close)
	return s
}

func (s *stubIntake) base() string { return s.srv.URL + "/api/unstable/its-agent-intake" }

func (s *stubIntake) respond(key string, statuses ...int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.responses[key] = statuses
}

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

type remoteFixture struct {
	check    *Check
	intake   *stubIntake
	mock     sqlmock.Sqlmock
	db       *sql.DB
	opens    atomic.Int32
	killed   atomic.Int32
	openHook func(cfg remoteDBConfig) (*remoteDB, error)
	lastCfg  remoteDBConfig
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
	f.check.remoteTestHooks = &remoteQueryHooks{
		openDB: func(cfg remoteDBConfig) (*remoteDB, error) {
			f.opens.Add(1)
			f.lastCfg = cfg
			if f.openHook != nil {
				return f.openHook(cfg)
			}
			return &remoteDB{DB: f.db, kill: func() { f.killed.Add(1) }}, nil
		},
		apiKey:          func() string { return testAPIKey },
		httpClient:      func() (*http.Client, func()) { return f.intake.srv.Client(), func() {} },
		validateBaseURL: func(u string) bool { return u == f.intake.base() },
	}
	return f
}

func (f *remoteFixture) expectProbe(rows *sqlmock.Rows) {
	f.mock.ExpectBegin()
	f.mock.ExpectExec(regexp.QuoteMeta("SET TRANSACTION READ ONLY")).WillReturnResult(sqlmock.NewResult(0, 0))
	f.mock.ExpectQuery(regexp.QuoteMeta(remoteProbeSQL)).WillReturnRows(rows)
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

type limitsOverride func(*map[string]any)

func executeRequest(base string, query string, includeSchema bool, target map[string]any, overrides ...limitsOverride) string {
	limits := map[string]any{
		"maxFileBytes": 16777216, "maxResultBytes": 107374182400, "maxRowBytes": 16777216, "maxColumns": 1024,
		"maxSchemaBytes": 1048576, "maxPages": 1280, "timeoutMs": 60000,
	}
	for _, o := range overrides {
		o(&limits)
	}
	if target == nil {
		target = map[string]any{"host": "db.example.com", "port": 1521, "dbname": "FREEPDB1"}
	}
	out, _ := json.Marshal(map[string]any{
		"operation": "execute", "target": target, "query": query, "includeSchema": includeSchema,
		"resultDelivery": map[string]any{"crawlId": testCrawlID, "taskId": testTaskID, "uploadId": testUploadID, "baseUrl": base, "limits": limits},
		"traceContext":   map[string]any{"traceId": "5843016738114925931", "parentId": "1122334455667788", "samplingPriority": 1},
	})
	return string(out)
}

func withLimit(key string, value int) limitsOverride {
	return func(m *map[string]any) { (*m)[key] = value }
}

func decodeAnswer(t *testing.T, raw string) remoteAnswer {
	t.Helper()
	var a remoteAnswer
	require.NoError(t, json.Unmarshal([]byte(raw), &a), raw)
	return a
}

func requireFailure(t *testing.T, raw, code string) {
	t.Helper()
	a := decodeAnswer(t, raw)
	require.Equal(t, "FAILED", a.Status, raw)
	require.NotNil(t, a.Error, raw)
	require.Equal(t, code, a.Error.Code, raw)
	for _, secret := range []string{testPassword, testAPIKey, "RQ_ORACLE_PROBE", "db.example.com", "rq_reader", "SELECT"} {
		require.NotContains(t, raw, secret, "an answer must not leak %q", secret)
	}
}

// --- resolve ---------------------------------------------------------------------

func TestRemoteResolveMatchesConfigurationOnly(t *testing.T) {
	f := newRemoteFixture(t)
	resolve := func(target map[string]any) string {
		req, _ := json.Marshal(map[string]any{"operation": "resolve", "target": target})
		out, err := f.check.RemoteQuery(context.Background(), string(req))
		require.NoError(t, err)
		return out
	}
	assert.JSONEq(t, `{"status":"MATCHED"}`, resolve(map[string]any{"host": "db.example.com", "port": 1521, "dbname": "FREEPDB1"}))
	// Host: trim, case and one trailing dot; service: trim and case-insensitive.
	assert.JSONEq(t, `{"status":"MATCHED"}`, resolve(map[string]any{"host": "  DB.Example.COM. ", "port": 1521, "dbname": " freepdb1 "}))
	for name, target := range map[string]map[string]any{
		"host":              {"host": "other.example.com", "port": 1521, "dbname": "FREEPDB1"},
		"two trailing dots": {"host": "db.example.com..", "port": 1521, "dbname": "FREEPDB1"},
		"port":              {"host": "db.example.com", "port": 1522, "dbname": "FREEPDB1"},
		"service":           {"host": "db.example.com", "port": 1521, "dbname": "XEPDB1"},
		"database instance": {"database_instance": "db.example.com"},
	} {
		requireFailure(t, resolve(target), remoteCodeTargetNotFound)
		_ = name
	}
	assert.Zero(t, f.opens.Load(), "resolve must not connect")
	assert.Empty(t, f.intake.recorded(), "resolve must not contact intake")
}

func TestRemoteResolveIgnoresConfigurationsOutsideThePOC(t *testing.T) {
	target := remoteTarget{Host: "db.example.com", Port: 1521, DBName: "FREEPDB1"}
	base := func() *config.CheckConfig {
		return &config.CheckConfig{InstanceConfig: config.InstanceConfig{ConnectionConfig: config.ConnectionConfig{
			Server: "db.example.com", Port: 1521, ServiceName: "FREEPDB1", Username: "u",
		}}}
	}
	require.True(t, remoteServes(base(), target))
	for name, mutate := range map[string]func(*config.CheckConfig){
		"tns alias":      func(c *config.CheckConfig) { c.TnsAlias = "alias" },
		"tns admin":      func(c *config.CheckConfig) { c.TnsAdmin = "/etc/tns" },
		"oracle client":  func(c *config.CheckConfig) { c.OracleClient = true },
		"tcps":           func(c *config.CheckConfig) { c.Protocol = "TCPS" },
		"wallet":         func(c *config.CheckConfig) { c.Wallet = "/wallet" },
		"embedded port":  func(c *config.CheckConfig) { c.ServerEmbedsPort = true },
		"implicit svc":   func(c *config.CheckConfig) { c.ServiceName = "  " },
		"no server":      func(c *config.CheckConfig) { c.Server = "" },
		"invalid port":   func(c *config.CheckConfig) { c.Port = 0 },
		"case-only host": nil,
	} {
		cfg := base()
		if mutate == nil {
			cfg.Server = "DB.EXAMPLE.COM"
			assert.True(t, remoteServes(cfg, target), name)
			continue
		}
		mutate(cfg)
		assert.False(t, remoteServes(cfg, target), name)
	}
	cfg := base()
	cfg.Protocol = "tcp"
	assert.True(t, remoteServes(cfg, target))
}

func TestRemoteResolveRejectsMalformedRequests(t *testing.T) {
	f := newRemoteFixture(t)
	for name, req := range map[string]string{
		"not json":         `nope`,
		"unknown op":       `{"operation":"run","target":{"host":"db.example.com","port":1521,"dbname":"x"}}`,
		"resolve + query":  `{"operation":"resolve","target":{"host":"db.example.com","port":1521,"dbname":"x"},"query":"SELECT 1"}`,
		"unknown field":    `{"operation":"resolve","target":{"host":"h","port":1521,"dbname":"x"},"extra":1}`,
		"bad port":         `{"operation":"resolve","target":{"host":"h","port":70000,"dbname":"x"}}`,
		"instance + host":  `{"operation":"resolve","target":{"database_instance":"i","host":"h"}}`,
		"execute no query": `{"operation":"execute","target":{"host":"h","port":1521,"dbname":"x"}}`,
	} {
		out, err := f.check.RemoteQuery(context.Background(), req)
		require.NoError(t, err, name)
		requireFailure(t, out, remoteCodeInvalidRequest)
	}
	assert.Zero(t, f.opens.Load())
}

// --- execute: fail closed before any connection ---------------------------------------

func TestRemoteExecuteRejectsUnsupportedSQLWithoutConnecting(t *testing.T) {
	for _, query := range []string{
		"SELECT 1 FROM dual",
		"DELETE FROM RQ_ORACLE_PROBE",
		remoteProbeSQL + "; DROP TABLE RQ_ORACLE_PROBE",
		remoteProbeSQL + ";",
		"select id, label from rq_oracle_probe order by id, label",
		"SELECT ID, LABEL FROM RQ_ORACLE_PROBE ORDER BY ID, LABEL FOR UPDATE",
		"SELECT * FROM RQ_ORACLE_PROBE",
	} {
		f := newRemoteFixture(t)
		out, err := f.check.RemoteQuery(context.Background(), executeRequest(f.intake.base(), query, false, nil))
		require.NoError(t, err)
		requireFailure(t, out, remoteCodeUnsupportedQuery)
		assert.Zero(t, f.opens.Load(), "no connection for %q", query)
		assert.Equal(t, []string{"POST /abort"}, f.intake.sequence(), "only the session is aborted for %q", query)
	}
}

func TestRemoteExecuteNormalizesWhitespaceOfTheApprovedQueryOnly(t *testing.T) {
	f := newRemoteFixture(t)
	f.expectProbe(probeRows([2]any{"1", "one"}))
	out, err := f.check.RemoteQuery(context.Background(), executeRequest(f.intake.base(), "SELECT ID,   LABEL\n FROM RQ_ORACLE_PROBE\tORDER BY ID, LABEL", false, nil))
	require.NoError(t, err)
	assert.JSONEq(t, `{"status":"SUCCEEDED"}`, out)
	require.NoError(t, f.mock.ExpectationsWereMet())
}

func TestRemoteExecuteRevalidatesTheTargetWithoutConnecting(t *testing.T) {
	for name, target := range map[string]map[string]any{
		"host":     {"host": "evil.example.com", "port": 1521, "dbname": "FREEPDB1"},
		"port":     {"host": "db.example.com", "port": 1, "dbname": "FREEPDB1"},
		"service":  {"host": "db.example.com", "port": 1521, "dbname": "OTHER"},
		"instance": {"database_instance": "db.example.com"},
	} {
		f := newRemoteFixture(t)
		out, err := f.check.RemoteQuery(context.Background(), executeRequest(f.intake.base(), remoteProbeSQL, false, target))
		require.NoError(t, err, name)
		requireFailure(t, out, remoteCodeTargetNotFound)
		assert.Zero(t, f.opens.Load(), name)
		assert.Equal(t, []string{"POST /abort"}, f.intake.sequence(), name)
	}
}

func TestRemoteExecuteRefusesAnyDestinationButTheAgentsIntake(t *testing.T) {
	f := newRemoteFixture(t)
	other := newStubIntake(t)
	out, err := f.check.RemoteQuery(context.Background(), executeRequest(other.base(), remoteProbeSQL, false, nil))
	require.NoError(t, err)
	requireFailure(t, out, remoteCodeInvalidRequest)
	assert.Zero(t, f.opens.Load())
	assert.Empty(t, other.recorded(), "the API key must not be sent to an unexpected destination")
	assert.Empty(t, f.intake.recorded())
}

func TestRemoteExecuteNeedsTheAgentAPIKey(t *testing.T) {
	f := newRemoteFixture(t)
	f.check.remoteTestHooks.apiKey = func() string { return "" }
	out, err := f.check.RemoteQuery(context.Background(), executeRequest(f.intake.base(), remoteProbeSQL, false, nil))
	require.NoError(t, err)
	requireFailure(t, out, remoteCodeCredentials)
	assert.Zero(t, f.opens.Load())
}

// --- execute: success and upload shape -----------------------------------------------

func TestRemoteExecuteStreamsAndFinalizes(t *testing.T) {
	f := newRemoteFixture(t)
	f.expectProbe(probeRows(
		[2]any{"1", "alpha"},
		[2]any{"12345678901234567890.123456789012345678", "he said \"hi\", then\nleft"},
		[2]any{"-0.5", nil},
		[2]any{nil, "naïve ✓"},
	))
	out, err := f.check.RemoteQuery(context.Background(), executeRequest(f.intake.base(), remoteProbeSQL, true, nil))
	require.NoError(t, err)
	assert.JSONEq(t, `{"status":"SUCCEEDED"}`, out)
	require.NoError(t, f.mock.ExpectationsWereMet())
	assert.Equal(t, []string{"POST /descriptor", "PUT /pages/0", "POST /finalize"}, f.intake.sequence())

	// The run used the check's own configured direct server and login.
	assert.Equal(t, remoteDBConfig{Server: "db.example.com", Port: 1521, ServiceName: "FREEPDB1", Username: "rq_reader", Password: testPassword, Timeout: f.lastCfg.Timeout}, f.lastCfg)
	assert.Greater(t, f.lastCfg.Timeout, 50*time.Second, "the driver read timeout follows the run time limit")

	reqs := f.intake.recorded()
	for _, r := range reqs {
		assert.Equal(t, testAPIKey, r.Header.Get("DD-API-KEY"))
		assert.Equal(t, "5843016738114925931", r.Header.Get("x-datadog-trace-id"))
		assert.Equal(t, "1122334455667788", r.Header.Get("x-datadog-parent-id"))
		assert.Equal(t, "1", r.Header.Get("x-datadog-sampling-priority"))
	}
	assert.JSONEq(t, `{"format_version":"csv-json-cell-v1","include_schema":true,"agent_hostname":"agent-host-1","columns":[`+
		`{"column_name":"ID","vendor_data_type":"NUMBER","logical_type":"decimal"},`+
		`{"column_name":"LABEL","vendor_data_type":"VARCHAR2","logical_type":"string"}]}`, string(reqs[0].Body))
	assert.Equal(t, "application/json", reqs[0].Header.Get("Content-Type"))

	page := reqs[1]
	assert.Equal(t, "text/csv", page.Header.Get("Content-Type"))
	assert.Equal(t, "4", page.Header.Get("X-DD-Source-Page-Rows"))
	assert.Equal(t, "0", page.Header.Get("X-DD-Record-Offset"))
	assert.Equal(t, strconv.Itoa(len(page.Body)), page.Header.Get("X-DD-Source-Page-Bytes"))
	assert.Empty(t, page.Header.Get("Content-Encoding"))
	records, err := csv.NewReader(strings.NewReader(string(page.Body))).ReadAll()
	require.NoError(t, err)
	assert.Equal(t, [][]string{
		{`1`, `"alpha"`},
		{`12345678901234567890.123456789012345678`, `"he said \"hi\", then\nleft"`},
		{`-0.5`, `null`},
		{`null`, `"naïve ✓"`},
	}, records, "NUMBER text is preserved exactly, never through a float")
	assert.JSONEq(t, `{"expected_page_count":1}`, string(reqs[2].Body))
}

func TestRemoteExecuteEmptyResultIsOneEmptyPage(t *testing.T) {
	f := newRemoteFixture(t)
	f.expectProbe(probeRows())
	out, err := f.check.RemoteQuery(context.Background(), executeRequest(f.intake.base(), remoteProbeSQL, false, nil))
	require.NoError(t, err)
	assert.JSONEq(t, `{"status":"SUCCEEDED"}`, out)
	reqs := f.intake.recorded()
	require.Len(t, reqs, 3)
	assert.Equal(t, "0", reqs[1].Header.Get("X-DD-Source-Page-Rows"))
	assert.Equal(t, "0", reqs[1].Header.Get("X-DD-Source-Page-Bytes"))
	assert.Empty(t, reqs[1].Body)
	assert.JSONEq(t, `{"expected_page_count":1}`, string(reqs[2].Body))
}

// synthesizeResultFile builds the file intake would store for a page, to check the
// producer's size bound without a copy of intake.
func synthesizeResultFile(t *testing.T, includeSchema bool, offset int64, body []byte) []byte {
	t.Helper()
	records, err := csv.NewReader(strings.NewReader(string(body))).ReadAll()
	require.NoError(t, err)
	var rows []string
	for _, rec := range records {
		var id any
		require.NoError(t, json.Unmarshal([]byte(rec[0]), &id))
		// A decimal column is stored as a string of its exact text.
		idToken := `null`
		if rec[0] != "null" {
			idToken = `"` + rec[0] + `"`
		}
		rows = append(rows, fmt.Sprintf(`{"ID":%s,"LABEL":%s}`, idToken, rec[1]))
	}
	schema := ""
	if includeSchema {
		schema = `"schema":[{"column_name":"ID","vendor_data_type":"NUMBER"},{"column_name":"LABEL","vendor_data_type":"VARCHAR2"}],`
	}
	return []byte(fmt.Sprintf(`{"contract_version":"1.0.0","crawl_id":%q,"task_id":%q,"record_offset":%d,%s"data":[%s],"org_id":%d,"agent_hostname":%q,"sensitive_data_scanned":false}`,
		testCrawlID, testTaskID, offset, schema, strings.Join(rows, ","), int64(1<<62), testHostname))
}

func TestRemoteExecuteSplitsPagesWithinTheFileLimit(t *testing.T) {
	for _, includeSchema := range []bool{false, true} {
		f := newRemoteFixture(t)
		label := strings.Repeat("x", 80)
		var rows [][2]any
		for i := 1; i <= 20; i++ {
			rows = append(rows, [2]any{strconv.Itoa(i), label})
		}
		f.expectProbe(probeRows(rows...))
		const maxFile = 700
		out, err := f.check.RemoteQuery(context.Background(), executeRequest(f.intake.base(), remoteProbeSQL, includeSchema, nil, withLimit("maxFileBytes", maxFile), withLimit("maxRowBytes", maxFile)))
		require.NoError(t, err)
		assert.JSONEq(t, `{"status":"SUCCEEDED"}`, out)

		var pageReqs []recordedRequest
		for _, r := range f.intake.recorded() {
			if r.Method == http.MethodPut {
				pageReqs = append(pageReqs, r)
			}
		}
		require.Greater(t, len(pageReqs), 2, "a small file limit forces several pages")
		var offset int64
		total := 0
		for i, p := range pageReqs {
			assert.Equal(t, "/pages/"+strconv.Itoa(i), p.Path)
			assert.Equal(t, strconv.FormatInt(offset, 10), p.Header.Get("X-DD-Record-Offset"))
			n, _ := strconv.ParseInt(p.Header.Get("X-DD-Source-Page-Rows"), 10, 64)
			assert.Positive(t, n)
			file := synthesizeResultFile(t, includeSchema, offset, p.Body)
			assert.LessOrEqual(t, len(file), maxFile, "page %d result file", i)
			offset += n
			total += int(n)
		}
		assert.Equal(t, 20, total)
		last := f.intake.recorded()[len(f.intake.recorded())-1]
		assert.Equal(t, "/finalize", last.Path)
		assert.JSONEq(t, fmt.Sprintf(`{"expected_page_count":%d}`, len(pageReqs)), string(last.Body))
	}
}

func TestRemoteExecuteEnforcesCallerLimits(t *testing.T) {
	label := strings.Repeat("x", 80)
	rows := func() *sqlmock.Rows { return probeRows([2]any{"1", label}, [2]any{"2", label}, [2]any{"3", label}) }
	for name, tc := range map[string]struct {
		overrides []limitsOverride
		code      string
		schema    bool
	}{
		"row too large":           {[]limitsOverride{withLimit("maxRowBytes", 50)}, remoteCodeRowTooLarge, false},
		"file smaller than a row": {[]limitsOverride{withLimit("maxFileBytes", 300), withLimit("maxRowBytes", 300)}, remoteCodeRowTooLarge, false},
		"page count":              {[]limitsOverride{withLimit("maxFileBytes", 560), withLimit("maxRowBytes", 560), withLimit("maxPages", 1)}, remoteCodeResultTooLarge, false},
		"total size":              {[]limitsOverride{withLimit("maxFileBytes", 560), withLimit("maxRowBytes", 560), withLimit("maxResultBytes", 600)}, remoteCodeResultTooLarge, false},
		"columns":                 {[]limitsOverride{withLimit("maxColumns", 1)}, remoteCodeInvalidColumns, false},
		"schema size":             {[]limitsOverride{withLimit("maxSchemaBytes", 10)}, remoteCodeDescriptorTooLarge, true},
	} {
		f := newRemoteFixture(t)
		f.expectProbe(rows())
		out, err := f.check.RemoteQuery(context.Background(), executeRequest(f.intake.base(), remoteProbeSQL, tc.schema, nil, tc.overrides...))
		require.NoError(t, err, name)
		requireFailure(t, out, tc.code)
		seq := f.intake.sequence()
		assert.Equal(t, "POST /abort", seq[len(seq)-1], name)
		assert.NotContains(t, seq, "POST /finalize", name)
	}
}

func TestRemoteExecuteRejectsUnsupportedColumnTypesBeforeUploading(t *testing.T) {
	for _, dbType := range []string{"OCIClobLocator", "OCIBlobLocator", "DATE", "TIMESTAMP", "RAW", "BFloat", "LONG", "XMLType"} {
		f := newRemoteFixture(t)
		f.mock.ExpectBegin()
		f.mock.ExpectExec("SET TRANSACTION READ ONLY").WillReturnResult(sqlmock.NewResult(0, 0))
		f.mock.ExpectQuery(regexp.QuoteMeta(remoteProbeSQL)).WillReturnRows(
			sqlmock.NewRowsWithColumnDefinition(sqlmock.NewColumn("ID").OfType("NUMBER", ""), sqlmock.NewColumn("LABEL").OfType(dbType, "")).AddRow("1", "x"))
		f.mock.ExpectRollback()
		out, err := f.check.RemoteQuery(context.Background(), executeRequest(f.intake.base(), remoteProbeSQL, false, nil))
		require.NoError(t, err)
		requireFailure(t, out, remoteCodeUnsupportedColumn)
		assert.Equal(t, []string{"POST /abort"}, f.intake.sequence(), dbType)
	}
}

func TestRemoteExecuteRejectsInexactOrInvalidValues(t *testing.T) {
	for name, tc := range map[string]struct {
		id, label any
	}{
		"float64 NUMBER":      {float64(0.1), "x"},
		"float32 NUMBER":      {float32(0.1), "x"},
		"NaN text":            {"NaN", "x"},
		"leading zero":        {"007", "x"},
		"invalid UTF-8":       {"1", "bad\xff"},
		"non string VARCHAR2": {"1", int64(5)},
	} {
		f := newRemoteFixture(t)
		f.mock.ExpectBegin()
		f.mock.ExpectExec("SET TRANSACTION READ ONLY").WillReturnResult(sqlmock.NewResult(0, 0))
		f.mock.ExpectQuery(regexp.QuoteMeta(remoteProbeSQL)).WillReturnRows(probeRows([2]any{tc.id, tc.label}))
		f.mock.ExpectRollback()
		out, err := f.check.RemoteQuery(context.Background(), executeRequest(f.intake.base(), remoteProbeSQL, false, nil))
		require.NoError(t, err, name)
		requireFailure(t, out, remoteCodeUnsupportedValue)
		seq := f.intake.sequence()
		assert.NotContains(t, seq, "PUT /pages/0", name)
		assert.Equal(t, "POST /abort", seq[len(seq)-1], name)
	}
}

// --- execute: abort and failure paths ---------------------------------------------------

func TestRemoteExecuteAbortsTheSessionWhenAPageFails(t *testing.T) {
	f := newRemoteFixture(t)
	f.intake.respond("PUT /pages/0", http.StatusBadRequest)
	f.expectProbe(probeRows([2]any{"1", "x"}))
	out, err := f.check.RemoteQuery(context.Background(), executeRequest(f.intake.base(), remoteProbeSQL, false, nil))
	require.NoError(t, err)
	requireFailure(t, out, remoteCodeUploadFailed)
	assert.Equal(t, []string{"POST /descriptor", "PUT /pages/0", "POST /abort"}, f.intake.sequence())
}

func TestRemoteExecuteAbortsTheSessionWhenFinalizeFails(t *testing.T) {
	f := newRemoteFixture(t)
	f.intake.respond("POST /finalize", http.StatusBadRequest)
	f.expectProbe(probeRows([2]any{"1", "x"}))
	out, err := f.check.RemoteQuery(context.Background(), executeRequest(f.intake.base(), remoteProbeSQL, false, nil))
	require.NoError(t, err)
	requireFailure(t, out, remoteCodeUploadFailed)
	assert.Equal(t, []string{"POST /descriptor", "PUT /pages/0", "POST /finalize", "POST /abort"}, f.intake.sequence())
}

func TestRemoteExecuteReportsAnOversizedResultFileDistinctly(t *testing.T) {
	f := newRemoteFixture(t)
	f.intake.respond("PUT /pages/0", http.StatusRequestEntityTooLarge)
	f.expectProbe(probeRows([2]any{"1", "x"}))
	out, err := f.check.RemoteQuery(context.Background(), executeRequest(f.intake.base(), remoteProbeSQL, false, nil))
	require.NoError(t, err)
	requireFailure(t, out, remoteCodeRowTooLarge)
	assert.Equal(t, "POST /abort", f.intake.sequence()[len(f.intake.sequence())-1])
}

func TestRemoteExecuteRetriesTransientIntakeFailuresWithTheSameBytes(t *testing.T) {
	f := newRemoteFixture(t)
	f.intake.respond("PUT /pages/0", http.StatusServiceUnavailable, http.StatusInternalServerError)
	f.expectProbe(probeRows([2]any{"1", "x"}))
	out, err := f.check.RemoteQuery(context.Background(), executeRequest(f.intake.base(), remoteProbeSQL, false, nil))
	require.NoError(t, err)
	assert.JSONEq(t, `{"status":"SUCCEEDED"}`, out)
	var pages []recordedRequest
	for _, r := range f.intake.recorded() {
		if r.Method == http.MethodPut {
			pages = append(pages, r)
		}
	}
	require.Len(t, pages, 3)
	assert.Equal(t, pages[0].Body, pages[2].Body)
	assert.Equal(t, "POST /finalize", f.intake.sequence()[len(f.intake.sequence())-1])
}

func TestRemoteExecuteDatabaseErrorsCarryOnlyTheOracleCode(t *testing.T) {
	f := newRemoteFixture(t)
	f.mock.ExpectBegin()
	f.mock.ExpectExec("SET TRANSACTION READ ONLY").WillReturnResult(sqlmock.NewResult(0, 0))
	f.mock.ExpectQuery(regexp.QuoteMeta(remoteProbeSQL)).WillReturnError(errors.New("ORA-00942: table or view does not exist RQ_ORACLE_PROBE for rq_reader/" + testPassword + "@db.example.com:1521"))
	f.mock.ExpectRollback()
	out, err := f.check.RemoteQuery(context.Background(), executeRequest(f.intake.base(), remoteProbeSQL, false, nil))
	require.NoError(t, err)
	requireFailure(t, out, remoteCodeQueryFailed)
	assert.Contains(t, out, "ORA-00942")
	assert.Equal(t, []string{"POST /abort"}, f.intake.sequence())
}

func TestRemoteExecuteConnectionErrorsLeakNothing(t *testing.T) {
	f := newRemoteFixture(t)
	f.openHook = func(remoteDBConfig) (*remoteDB, error) {
		return nil, errors.New("dial tcp db.example.com:1521 user rq_reader password " + testPassword)
	}
	out, err := f.check.RemoteQuery(context.Background(), executeRequest(f.intake.base(), remoteProbeSQL, false, nil))
	require.NoError(t, err)
	requireFailure(t, out, remoteCodeConnectionFailed)
}

func TestRemoteExecuteTimesOutAndAborts(t *testing.T) {
	f := newRemoteFixture(t)
	f.mock.ExpectBegin()
	f.mock.ExpectExec("SET TRANSACTION READ ONLY").WillReturnResult(sqlmock.NewResult(0, 0))
	f.mock.ExpectQuery(regexp.QuoteMeta(remoteProbeSQL)).WillDelayFor(time.Hour).WillReturnRows(probeRows())
	f.mock.ExpectRollback()
	out, err := f.check.RemoteQuery(context.Background(), executeRequest(f.intake.base(), remoteProbeSQL, false, nil, withLimit("timeoutMs", 100)))
	require.NoError(t, err)
	requireFailure(t, out, remoteCodeTimeout)
	assert.Equal(t, []string{"POST /abort"}, f.intake.sequence())
	assert.Positive(t, f.killed.Load(), "the run's sockets are closed when its time ends")
}

// --- lifecycle ---------------------------------------------------------------------------

func TestRemoteCancelStopsRunsAndRefusesNewCalls(t *testing.T) {
	f := newRemoteFixture(t)
	f.mock.ExpectBegin()
	f.mock.ExpectExec("SET TRANSACTION READ ONLY").WillReturnResult(sqlmock.NewResult(0, 0))
	f.mock.ExpectQuery(regexp.QuoteMeta(remoteProbeSQL)).WillDelayFor(time.Hour).WillReturnRows(probeRows())
	f.mock.ExpectRollback()

	answers := make(chan string, 1)
	go func() {
		out, err := f.check.RemoteQuery(context.Background(), executeRequest(f.intake.base(), remoteProbeSQL, false, nil))
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
	requireFailure(t, <-answers, remoteCodeCancelled)
	assert.Equal(t, []string{"POST /abort"}, f.intake.sequence(), "the cancelled run aborts its session and never finalizes")
	assert.Positive(t, f.killed.Load())
}

func TestRemoteAfterCancelNoCallStarts(t *testing.T) {
	f := newRemoteFixture(t)
	f.check.Cancel()
	for _, req := range []string{
		`{"operation":"resolve","target":{"host":"db.example.com","port":1521,"dbname":"FREEPDB1"}}`,
		executeRequest(f.intake.base(), remoteProbeSQL, false, nil),
	} {
		_, err := f.check.RemoteQuery(context.Background(), req)
		assert.ErrorIs(t, err, check.ErrRemoteQueryCancelled)
	}
	assert.Zero(t, f.opens.Load())
	assert.Empty(t, f.intake.recorded())
}

func TestRemoteCallsNeverStartOnAnEndedContext(t *testing.T) {
	f := newRemoteFixture(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := f.check.RemoteQuery(ctx, executeRequest(f.intake.base(), remoteProbeSQL, false, nil))
	assert.ErrorIs(t, err, context.Canceled)
	assert.Zero(t, f.opens.Load())
}

func TestSupportsRemoteQueriesNeedsAConfiguredCheck(t *testing.T) {
	assert.False(t, (&Check{}).SupportsRemoteQueries())
	assert.True(t, newRemoteFixture(t).check.SupportsRemoteQueries())
}

// --- transport and encoding ----------------------------------------------------------------

func TestIntakeClientNeverFollowsRedirects(t *testing.T) {
	var elsewhere atomic.Int32
	other := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { elsewhere.Add(1) }))
	defer other.Close()
	first := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, other.URL+"/steal", http.StatusTemporaryRedirect)
	}))
	defer first.Close()
	client := first.Client()
	client.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	c := newIntakeClient(client, first.URL, testUploadID, testAPIKey, nil)
	err := c.registerDescriptor(context.Background(), []byte(`{}`))
	require.Error(t, err)
	assert.Zero(t, elsewhere.Load())
	assert.True(t, isIntakeCode(err, "invalid_response"))
}

func TestIntakeBaseURLIsDerivedFromTheSiteOnly(t *testing.T) {
	got, ok := intakeSiteBaseURL(" DatadogHQ.com. ")
	assert.True(t, ok)
	assert.Equal(t, "https://api.datadoghq.com/api/unstable/its-agent-intake", got)
	for _, bad := range []string{"", "evil.com/x", "a@b.com", "x.com:8080", "x.com?y", "x y.com", "x.com#f", `x.com\y`} {
		_, ok := intakeSiteBaseURL(bad)
		assert.False(t, ok, bad)
	}
}

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

func TestRemoteExecutePanicsBecomeInternalErrorsAndAbort(t *testing.T) {
	f := newRemoteFixture(t)
	f.openHook = func(remoteDBConfig) (*remoteDB, error) { panic("boom RQ_ORACLE_PROBE " + testPassword) }
	out, err := f.check.RemoteQuery(context.Background(), executeRequest(f.intake.base(), remoteProbeSQL, false, nil))
	require.NoError(t, err)
	requireFailure(t, out, remoteCodeInternalError)
	assert.Equal(t, []string{"POST /abort"}, f.intake.sequence())
}
