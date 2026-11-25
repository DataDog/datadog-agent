// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build oracle

package oracle

import (
	"context"
	"encoding/csv"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/oracle/config"
)

// Opt-in smoke test of the remote query path against a real Oracle database, using
// the production openRemoteOracle/go-ora code and a local HTTPS stub intake. It is
// skipped unless RQ_ORACLE_LIVE_HOST is set. Credentials come from the environment:
//
//	RQ_ORACLE_LIVE_HOST, RQ_ORACLE_LIVE_PORT (default 1521), RQ_ORACLE_LIVE_SERVICE (default FREEPDB1)
//	RQ_ORACLE_LIVE_READER_USER / _READER_PASSWORD  SELECT-only login that sees RQ_ORACLE_PROBE
//	RQ_ORACLE_LIVE_OWNER_USER / _OWNER_PASSWORD    optional: fixture owner, to prove the read-only transaction
//	RQ_ORACLE_LIVE_EMPTY_USER / _EMPTY_PASSWORD    optional: login whose RQ_ORACLE_PROBE is empty
//
// The fixture is RQ_ORACLE_PROBE (ID NUMBER, LABEL VARCHAR2(100)) holding the rows
// asserted below.

type liveTarget struct {
	host, service string
	port          int
}

func liveEnv(t *testing.T) liveTarget {
	t.Helper()
	host := os.Getenv("RQ_ORACLE_LIVE_HOST")
	if host == "" {
		t.Skip("RQ_ORACLE_LIVE_HOST is not set")
	}
	port := 1521
	if p := os.Getenv("RQ_ORACLE_LIVE_PORT"); p != "" {
		var err error
		port, err = strconv.Atoi(p)
		require.NoError(t, err)
	}
	service := os.Getenv("RQ_ORACLE_LIVE_SERVICE")
	if service == "" {
		service = "FREEPDB1"
	}
	return liveTarget{host: host, port: port, service: service}
}

func liveLogin(t *testing.T, prefix string) (string, string) {
	t.Helper()
	user, password := os.Getenv("RQ_ORACLE_LIVE_"+prefix+"_USER"), os.Getenv("RQ_ORACLE_LIVE_"+prefix+"_PASSWORD")
	if user == "" || password == "" {
		t.Skipf("RQ_ORACLE_LIVE_%s_USER/_PASSWORD are not set", prefix)
	}
	return user, password
}

// tlsStubIntake records the upload session over HTTPS and accepts every request.
type tlsStubIntake struct {
	srv      *httptest.Server
	mu       sync.Mutex
	requests []recordedRequest
}

func newTLSStubIntake(t *testing.T) *tlsStubIntake {
	s := &tlsStubIntake{}
	s.srv = httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		suffix := strings.TrimPrefix(r.URL.Path, "/api/unstable/its-agent-intake/uploads/"+testUploadID)
		s.mu.Lock()
		s.requests = append(s.requests, recordedRequest{Method: r.Method, Path: suffix, Header: r.Header.Clone(), Body: body})
		s.mu.Unlock()
		w.WriteHeader(http.StatusNoContent)
	}))
	t.Cleanup(s.srv.Close)
	return s
}

func (s *tlsStubIntake) base() string { return s.srv.URL + "/api/unstable/its-agent-intake" }

func (s *tlsStubIntake) recorded() []recordedRequest {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]recordedRequest(nil), s.requests...)
}

func (s *tlsStubIntake) sequence() []string {
	var seq []string
	for _, r := range s.recorded() {
		seq = append(seq, r.Method+" "+r.Path)
	}
	return seq
}

// liveCheck builds a check pointing at the real database. Only the intake side is
// replaced: the production openRemoteOracle opens the database connection.
func liveCheck(t *testing.T, target liveTarget, user, password string, intake *tlsStubIntake) *Check {
	t.Helper()
	return &Check{
		config: &config.CheckConfig{InstanceConfig: config.InstanceConfig{ConnectionConfig: config.ConnectionConfig{
			Server: target.host, Port: target.port, ServiceName: target.service, Username: user, Password: password,
		}}},
		agentHostname: testHostname,
		remoteTestHooks: &remoteQueryHooks{
			openDB:          openRemoteOracle,
			apiKey:          func() string { return testAPIKey },
			httpClient:      func() (*http.Client, func()) { return intake.srv.Client(), func() {} },
			validateBaseURL: func(u string) bool { return u == intake.base() },
		},
	}
}

func liveExecute(t *testing.T, c *Check, target liveTarget, intake *tlsStubIntake) remoteAnswer {
	t.Helper()
	out, err := c.RemoteQuery(context.Background(), executeRequest(intake.base(), remoteProbeSQL, true,
		map[string]any{"host": target.host, "port": target.port, "dbname": target.service}))
	require.NoError(t, err)
	return decodeAnswer(t, out)
}

func TestRemoteLiveOracleStreamsExactValues(t *testing.T) {
	target := liveEnv(t)
	user, password := liveLogin(t, "READER")
	intake := newTLSStubIntake(t)
	answer := liveExecute(t, liveCheck(t, target, user, password, intake), target, intake)
	require.Equal(t, "SUCCEEDED", answer.Status, "%+v", answer.Error)
	require.Equal(t, []string{"POST /descriptor", "PUT /pages/0", "POST /finalize"}, intake.sequence())

	reqs := intake.recorded()
	for _, r := range reqs {
		assert.Equal(t, testAPIKey, r.Header.Get("DD-API-KEY"))
	}
	assert.JSONEq(t, `{"format_version":"csv-json-cell-v1","include_schema":true,"agent_hostname":"agent-host-1","columns":[`+
		`{"column_name":"ID","vendor_data_type":"NUMBER","logical_type":"decimal"},`+
		`{"column_name":"LABEL","vendor_data_type":"VARCHAR2","logical_type":"string"}]}`, string(reqs[0].Body))

	page := reqs[1]
	assert.Equal(t, "7", page.Header.Get("X-DD-Source-Page-Rows"))
	records, err := csv.NewReader(strings.NewReader(string(page.Body))).ReadAll()
	require.NoError(t, err)
	// ORDER BY ID, LABEL: NUMBER order, the null ID sorts last in Oracle.
	assert.Equal(t, [][]string{
		{`-0.5`, `"negative decimal"`},
		{`0.1234567890123456789012345678901234567`, `"37 digit fraction"`},
		{`1`, `"one"`},
		{`3`, `null`},
		{`9007199254740993`, `"2^53 + 1"`},
		{`12345678901234567890123`, `"beyond int64"`},
		{`null`, `"naïve ✓ \"quoted\", with comma"`},
	}, records, "NUMBER text is exact; VARCHAR2 and NULL are preserved")
	assert.JSONEq(t, `{"expected_page_count":1}`, string(reqs[2].Body))
}

// The fixed statement runs in a transaction that go-ora started with SET TRANSACTION
// READ ONLY. The fixture owner can write, so a write inside that transaction is only
// refused because the transaction really is read-only (ORA-01456).
func TestRemoteLiveOracleTransactionIsReadOnly(t *testing.T) {
	target := liveEnv(t)
	user, password := liveLogin(t, "OWNER")
	run := &remoteRun{check: liveCheck(t, target, user, password, newTLSStubIntake(t))}
	run.req = remoteRequest{}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	db, err := openRemoteOracle(remoteDBConfig{Server: target.host, Port: target.port, ServiceName: target.service, Username: user, Password: password, Timeout: 30 * time.Second})
	require.NoError(t, err)
	defer db.kill()
	defer db.Close()

	tx, rows, err := run.startQuery(ctx, db, remoteProbeSQL)
	require.NoError(t, err)
	require.NoError(t, rows.Close())
	_, err = tx.ExecContext(ctx, "INSERT INTO RQ_ORACLE_PROBE (ID, LABEL) VALUES (999, 'must not be written')")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "ORA-01456", "writes inside the run's transaction must be refused as read-only")
	require.NoError(t, tx.Rollback())

	// Control: the same owner can write outside such a transaction, so the refusal above is
	// not just missing privileges. Rolled back, nothing is kept.
	ctl, err := db.BeginTx(ctx, nil)
	require.NoError(t, err)
	_, err = ctl.ExecContext(ctx, "INSERT INTO RQ_ORACLE_PROBE (ID, LABEL) VALUES (999, 'rolled back')")
	require.NoError(t, err)
	require.NoError(t, ctl.Rollback())

	// The connection pool of one connection is reusable and the fixture is unchanged.
	var count int
	require.NoError(t, db.QueryRowContext(ctx, "SELECT COUNT(*) FROM RQ_ORACLE_PROBE WHERE ID = 999").Scan(&count))
	assert.Zero(t, count)
}

func TestRemoteLiveOracleEmptyResultIsOneEmptyPage(t *testing.T) {
	target := liveEnv(t)
	user, password := liveLogin(t, "EMPTY")
	intake := newTLSStubIntake(t)
	answer := liveExecute(t, liveCheck(t, target, user, password, intake), target, intake)
	require.Equal(t, "SUCCEEDED", answer.Status, "%+v", answer.Error)
	require.Equal(t, []string{"POST /descriptor", "PUT /pages/0", "POST /finalize"}, intake.sequence())
	page := intake.recorded()[1]
	assert.Equal(t, "0", page.Header.Get("X-DD-Source-Page-Rows"))
	assert.Empty(t, strings.TrimSpace(string(page.Body)))
	var finalize map[string]int
	require.NoError(t, json.Unmarshal(intake.recorded()[2].Body, &finalize))
	assert.Equal(t, 1, finalize["expected_page_count"])
}
