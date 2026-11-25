// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package session

import (
	"bytes"
	"compress/gzip"
	"context"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	configmock "github.com/DataDog/datadog-agent/pkg/config/mock"
	pkgconfigmodel "github.com/DataDog/datadog-agent/pkg/config/model"
)

const testAPIKey = "test-api-key"

type recorded struct {
	Method string
	Path   string
	Header http.Header
	// Body is the decoded body: a gzip page is recorded uncompressed.
	Body []byte
	Sent int
}

type reply struct {
	status int
	code   string
}

// stubIntake is a recording its-agent-intake stand-in. replies maps "METHOD suffix"
// (for example "PUT /pages/1") to a queue of replies; an empty queue answers 204.
// maxPageRows, when set, rejects pages with more rows as result_file_too_large.
type stubIntake struct {
	srv         *httptest.Server
	mu          sync.Mutex
	requests    []recorded
	replies     map[string][]reply
	refuseGzip  bool
	maxPageRows int
}

func newStubIntake(t *testing.T) *stubIntake {
	s := &stubIntake{replies: map[string][]reply{}}
	s.srv = httptest.NewServer(http.HandlerFunc(s.serve))
	t.Cleanup(s.srv.Close)
	return s
}

func (s *stubIntake) serve(w http.ResponseWriter, r *http.Request) {
	raw, _ := io.ReadAll(r.Body)
	body := raw
	suffix := strings.TrimPrefix(r.URL.Path, "/api/unstable/its-agent-intake/uploads/"+testUploadID)
	s.mu.Lock()
	defer s.mu.Unlock()
	if r.Header.Get("Content-Encoding") == "gzip" {
		if s.refuseGzip {
			s.requests = append(s.requests, recorded{Method: r.Method, Path: suffix, Header: r.Header.Clone(), Sent: len(raw)})
			w.Header().Set("Accept-Encoding", "gzip")
			w.WriteHeader(http.StatusUnsupportedMediaType)
			_, _ = io.WriteString(w, `{"error":{"code":"unsupported_encoding","message":"no"}}`)
			return
		}
		zr, err := gzip.NewReader(bytes.NewReader(raw))
		if err != nil {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		body, _ = io.ReadAll(zr)
	}
	s.requests = append(s.requests, recorded{Method: r.Method, Path: suffix, Header: r.Header.Clone(), Body: body, Sent: len(raw)})
	key := r.Method + " " + suffix
	if queue := s.replies[key]; len(queue) > 0 {
		rep := queue[0]
		s.replies[key] = queue[1:]
		w.WriteHeader(rep.status)
		_, _ = fmt.Fprintf(w, `{"error":{"code":%q,"message":"stub"}}`, rep.code)
		return
	}
	if rows, _ := strconv.Atoi(r.Header.Get("X-DD-Source-Page-Rows")); s.maxPageRows > 0 && r.Method == http.MethodPut && rows > s.maxPageRows {
		w.WriteHeader(http.StatusRequestEntityTooLarge)
		_, _ = io.WriteString(w, `{"error":{"code":"result_file_too_large","message":"big"}}`)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *stubIntake) reply(key string, replies ...reply) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.replies[key] = replies
}

func (s *stubIntake) recorded() []recorded {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]recorded(nil), s.requests...)
}

func (s *stubIntake) sequence() []string {
	var seq []string
	for _, r := range s.recorded() {
		seq = append(seq, r.Method+" "+r.Path)
	}
	return seq
}

func (s *stubIntake) delivery() *Delivery {
	return &Delivery{CrawlID: testCrawlID, TaskID: testTaskID, UploadID: testUploadID, BaseURL: s.srv.URL + "/api/unstable/its-agent-intake",
		Limits: Limits{MaxFileBytes: 16777216, MaxResultBytes: 107374182400, MaxRowBytes: 16777216, MaxColumns: 1024, MaxSchemaBytes: 1048576, MaxPages: 1280, TimeoutMs: 60000}}
}

func (s *stubIntake) client(trace *Trace) *Client {
	c := NewClient(s.srv.Client(), s.delivery(), testAPIKey, trace)
	c.sleep = func(context.Context, time.Duration) error { return nil }
	return c
}

var testColumns = []Column{{Name: "ID", DatabaseType: "NUMBER", LogicalType: LogicalDecimal}, {Name: "LABEL", DatabaseType: "VARCHAR2", LogicalType: LogicalString}}

func row(t *testing.T, id, label string) []Cell {
	t.Helper()
	idCell, ok := DecimalCell(id)
	require.True(t, ok)
	labelCell, ok := StringCell(label)
	require.True(t, ok)
	return []Cell{idCell, labelCell}
}

func upload(t *testing.T, s *stubIntake, d *Delivery, includeSchema bool, rows int, label string) error {
	t.Helper()
	u, f := NewUploader(s.client(nil), d, testColumns, includeSchema, "agent-host-1", "test")
	require.Nil(t, f)
	require.NoError(t, u.Register(context.Background()))
	for i := 1; i <= rows; i++ {
		if err := u.Add(context.Background(), row(t, strconv.Itoa(i), label)); err != nil {
			return err
		}
	}
	return u.Finish(context.Background())
}

func pages(s *stubIntake) []recorded {
	var out []recorded
	for _, r := range s.recorded() {
		if r.Method == http.MethodPut {
			out = append(out, r)
		}
	}
	return out
}

// resultFile builds the file intake would store for a page, to check the uploader's
// size bound.
func resultFile(t *testing.T, includeSchema bool, offset int64, body []byte) []byte {
	t.Helper()
	records, err := csv.NewReader(bytes.NewReader(body)).ReadAll()
	require.NoError(t, err)
	var rows []string
	for _, rec := range records {
		// A decimal column is stored as a string of its exact text.
		rows = append(rows, fmt.Sprintf(`{"ID":"%s","LABEL":%s}`, rec[0], rec[1]))
	}
	schema := ""
	if includeSchema {
		schema = `"schema":[{"column_name":"ID","vendor_data_type":"NUMBER"},{"column_name":"LABEL","vendor_data_type":"VARCHAR2"}],`
	}
	return []byte(fmt.Sprintf(`{"contract_version":"1.0.0","crawl_id":%q,"task_id":%q,"record_offset":%d,%s"data":[%s],"org_id":%d,"agent_hostname":"agent-host-1","sensitive_data_scanned":false}`,
		testCrawlID, testTaskID, offset, schema, strings.Join(rows, ","), int64(1<<62)))
}

// DESC-04, UPLOAD-01, UPLOAD-02, FINISH-01
func TestUPLOAD01_UploadPagesInOrderAndFinalize(t *testing.T) {
	s := newStubIntake(t)
	require.NoError(t, upload(t, s, s.delivery(), true, 3, `he said "hi", then`+"\nleft ✓"))
	assert.Equal(t, []string{"POST /descriptor", "PUT /pages/0", "POST /finalize"}, s.sequence())
	reqs := s.recorded()
	assert.JSONEq(t, `{"format_version":"csv-json-cell-v1","include_schema":true,"agent_hostname":"agent-host-1","columns":[`+
		`{"column_name":"ID","vendor_data_type":"NUMBER","logical_type":"decimal"},`+
		`{"column_name":"LABEL","vendor_data_type":"VARCHAR2","logical_type":"string"}]}`, string(reqs[0].Body))
	page := reqs[1]
	assert.Equal(t, "3", page.Header.Get("X-DD-Source-Page-Rows"))
	assert.Equal(t, "0", page.Header.Get("X-DD-Record-Offset"))
	assert.Equal(t, strconv.Itoa(len(page.Body)), page.Header.Get("X-DD-Source-Page-Bytes"))
	records, err := csv.NewReader(bytes.NewReader(page.Body)).ReadAll()
	require.NoError(t, err)
	require.Len(t, records, 3)
	assert.Equal(t, []string{"1", `"he said \"hi\", then\nleft ✓"`}, records[0], "each field holds one JSON value")
	assert.JSONEq(t, `{"expected_page_count":1}`, string(reqs[2].Body))
}

// SIZE-01, SIZE-06
func TestSIZE01_SizeEachPageByItsResultFile(t *testing.T) {
	for _, includeSchema := range []bool{false, true} {
		s := newStubIntake(t)
		d := s.delivery()
		d.Limits.MaxFileBytes, d.Limits.MaxRowBytes = 700, 700
		require.NoError(t, upload(t, s, d, includeSchema, 20, strings.Repeat("x", 80)))
		ps := pages(s)
		require.Greater(t, len(ps), 2, "a small result file size forces several pages")
		var offset int64
		for i, p := range ps {
			assert.Equal(t, "/pages/"+strconv.Itoa(i), p.Path)
			assert.Equal(t, strconv.FormatInt(offset, 10), p.Header.Get("X-DD-Record-Offset"))
			n, _ := strconv.ParseInt(p.Header.Get("X-DD-Source-Page-Rows"), 10, 64)
			assert.LessOrEqual(t, len(resultFile(t, includeSchema, offset, p.Body)), 700, "page %d", i)
			offset += n
		}
		assert.EqualValues(t, 20, offset)
	}

	for name, tc := range map[string]struct {
		edit func(*Limits)
		code string
	}{
		"row size":          {func(l *Limits) { l.MaxRowBytes = 50 }, CodeRowTooLarge},
		"page count":        {func(l *Limits) { l.MaxFileBytes, l.MaxPages = 560, 1 }, CodeResultTooLarge},
		"total result size": {func(l *Limits) { l.MaxFileBytes, l.MaxResultBytes = 560, 600 }, CodeResultTooLarge},
	} {
		s := newStubIntake(t)
		d := s.delivery()
		tc.edit(&d.Limits)
		err := upload(t, s, d, false, 3, strings.Repeat("x", 80))
		var f *Failure
		require.ErrorAs(t, err, &f, name)
		assert.Equal(t, tc.code, f.Code, name)
		assert.NotContains(t, s.sequence(), "POST /finalize", name)
	}
}

// SIZE-02
func TestSIZE02_SplitAPageThatIntakeReportsAsOversized(t *testing.T) {
	s := newStubIntake(t)
	s.maxPageRows = 3
	require.NoError(t, upload(t, s, s.delivery(), false, 10, "x"))
	var accepted []string
	var offset int64
	for _, p := range pages(s) {
		n, _ := strconv.ParseInt(p.Header.Get("X-DD-Source-Page-Rows"), 10, 64)
		if n > 3 {
			continue
		}
		assert.Equal(t, strconv.FormatInt(offset, 10), p.Header.Get("X-DD-Record-Offset"))
		accepted = append(accepted, p.Path+"="+strconv.FormatInt(n, 10))
		offset += n
	}
	// 10 rows rejected, then their first 5, then 2 accepted; the 8 left out start the
	// next page, which is halved the same way.
	assert.Equal(t, []string{"/pages/0=2", "/pages/1=2", "/pages/2=3", "/pages/3=3"}, accepted)
	assert.EqualValues(t, 10, offset, "no row is lost or repeated")
	assert.JSONEq(t, `{"expected_page_count":4}`, string(s.recorded()[len(s.recorded())-1].Body))

	s = newStubIntake(t)
	s.reply("PUT /pages/0", reply{http.StatusRequestEntityTooLarge, "result_file_too_large"})
	err := upload(t, s, s.delivery(), false, 1, "x")
	var f *Failure
	require.ErrorAs(t, err, &f)
	assert.Equal(t, CodeRowTooLarge, f.Code, "a page of one record cannot be split")
}

// SIZE-05
func TestSIZE05_RepresentAnEmptyResult(t *testing.T) {
	s := newStubIntake(t)
	require.NoError(t, upload(t, s, s.delivery(), true, 0, ""))
	reqs := s.recorded()
	require.Equal(t, []string{"POST /descriptor", "PUT /pages/0", "POST /finalize"}, s.sequence())
	assert.Equal(t, "0", reqs[1].Header.Get("X-DD-Source-Page-Rows"))
	assert.Equal(t, "0", reqs[1].Header.Get("X-DD-Source-Page-Bytes"))
	assert.Empty(t, reqs[1].Body)
	assert.JSONEq(t, `{"expected_page_count":1}`, string(reqs[2].Body))
}

// UPLOAD-03, UPLOAD-04
func TestUPLOAD04_RetryRequestsThatCanStillSucceed(t *testing.T) {
	s := newStubIntake(t)
	s.reply("PUT /pages/0", reply{http.StatusServiceUnavailable, "unavailable"}, reply{http.StatusServiceUnavailable, "unavailable"},
		reply{http.StatusServiceUnavailable, "unavailable"}, reply{http.StatusServiceUnavailable, "unavailable"},
		reply{http.StatusServiceUnavailable, "unavailable"}, reply{http.StatusInternalServerError, "internal"})
	require.NoError(t, upload(t, s, s.delivery(), false, 50, strings.Repeat("compressible ", 10)))
	ps := pages(s)
	require.Len(t, ps, 7, "capacity replies do not count as attempts")
	for _, p := range ps[1:] {
		assert.Equal(t, ps[0].Body, p.Body, "every attempt sends the same page")
		assert.Equal(t, ps[0].Sent, p.Sent, "and the same compressed body")
	}

	s = newStubIntake(t)
	s.reply("PUT /pages/0", reply{500, "internal"}, reply{500, "internal"}, reply{500, "internal"}, reply{500, "internal"}, reply{500, "internal"})
	err := upload(t, s, s.delivery(), false, 1, "x")
	assert.True(t, IsIntakeCode(err, "internal"), "retries end after the attempts limit")
	assert.Len(t, pages(s), MaxAttempts)

	s = newStubIntake(t)
	s.reply("POST /finalize", reply{http.StatusBadRequest, "invalid_argument"})
	err = upload(t, s, s.delivery(), false, 1, "x")
	assert.True(t, IsIntakeCode(err, "invalid_argument"), "an error that is not retryable is not retried")
	assert.Equal(t, []string{"POST /descriptor", "PUT /pages/0", "POST /finalize"}, s.sequence())

	s = newStubIntake(t)
	s.reply("POST /descriptor", reply{http.StatusBadRequest, "something new"})
	err = s.client(nil).RegisterDescriptor(context.Background(), []byte(`{}`))
	assert.True(t, IsIntakeCode(err, "invalid_response"), "unknown codes are never propagated")
}

// UPLOAD-05
func TestUPLOAD05_BoundEachAttempt(t *testing.T) {
	// A server that reads nothing of a large body stalls the send.
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	defer ln.Close()
	var accepted atomic.Int32
	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			accepted.Add(1)
			defer conn.Close()
		}
	}()
	d := &Delivery{UploadID: testUploadID, BaseURL: "http://" + ln.Addr().String()}
	c := NewClient(&http.Client{}, d, testAPIKey, nil)
	c.stall = 200 * time.Millisecond
	c.sleep = func(context.Context, time.Duration) error { return nil }
	c.identityOnly = true
	start := time.Now()
	_, err = c.PutPage(context.Background(), 0, bytes.Repeat([]byte("x"), 64<<20), 1, 0)
	assert.True(t, IsIntakeCode(err, "transport_error"), "%v", err)
	assert.Less(t, time.Since(start), 30*time.Second, "each stalled attempt ends after the stall time")
	assert.EqualValues(t, MaxAttempts, accepted.Load())
}

// UPLOAD-06, UPLOAD-07, OBS-02
func TestUPLOAD06_SendRequestsOnlyToTheUploadAddress(t *testing.T) {
	var elsewhere atomic.Int32
	other := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { elsewhere.Add(1) }))
	defer other.Close()
	first := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, other.URL+"/steal", http.StatusTemporaryRedirect)
	}))
	defer first.Close()
	cfg := configmock.New(t)
	cfg.Set("skip_ssl_validation", true, pkgconfigmodel.SourceAgentRuntime)
	client, done := NewHTTPClient(cfg)
	defer done()
	assert.False(t, client.Transport.(*http.Transport).TLSClientConfig.InsecureSkipVerify, "intake's certificate is always verified")
	err := NewClient(client, &Delivery{UploadID: testUploadID, BaseURL: first.URL}, testAPIKey, nil).RegisterDescriptor(context.Background(), []byte(`{}`))
	assert.True(t, IsIntakeCode(err, "redirect"), "%v", err)
	assert.Zero(t, elsewhere.Load(), "a redirect is never followed")

	s := newStubIntake(t)
	require.NoError(t, s.client(&Trace{TraceID: "5843016738114925931", ParentID: "1122334455667788", SamplingPriority: 2}).RegisterDescriptor(context.Background(), []byte(`{}`)))
	require.NoError(t, s.client(nil).Finalize(context.Background(), 1))
	reqs := s.recorded()
	for _, r := range reqs {
		assert.Equal(t, testAPIKey, r.Header.Get("DD-API-KEY"))
		assert.Empty(t, r.Header.Get("Authorization"))
		assert.Empty(t, r.Header.Get("Cookie"))
	}
	assert.Equal(t, "/descriptor", reqs[0].Path)
	assert.Equal(t, "5843016738114925931", reqs[0].Header.Get("x-datadog-trace-id"))
	assert.Equal(t, "1122334455667788", reqs[0].Header.Get("x-datadog-parent-id"))
	assert.Equal(t, "2", reqs[0].Header.Get("x-datadog-sampling-priority"))
	assert.Empty(t, reqs[1].Header.Get("x-datadog-trace-id"), "no trace context, no trace headers")
}

// UPLOAD-08
func TestUPLOAD08_CompressPageBodies(t *testing.T) {
	s := newStubIntake(t)
	require.NoError(t, upload(t, s, s.delivery(), false, 100, strings.Repeat("compressible ", 20)))
	p := pages(s)[0]
	assert.Equal(t, "gzip", p.Header.Get("Content-Encoding"))
	assert.Equal(t, strconv.Itoa(len(p.Body)), p.Header.Get("X-DD-Source-Page-Bytes"), "the declared length is the uncompressed one")
	assert.Less(t, p.Sent, len(p.Body)/4)

	s = newStubIntake(t)
	require.NoError(t, upload(t, s, s.delivery(), false, 1, "x"))
	assert.Empty(t, pages(s)[0].Header.Get("Content-Encoding"), "a page that does not get smaller is sent uncompressed")

	s = newStubIntake(t)
	s.refuseGzip = true
	d := s.delivery()
	d.Limits.MaxFileBytes, d.Limits.MaxRowBytes = 2000, 2000
	require.NoError(t, upload(t, s, d, false, 100, strings.Repeat("compressible ", 5)))
	ps := pages(s)
	assert.Equal(t, "gzip", ps[0].Header.Get("Content-Encoding"))
	for _, p := range ps[1:] {
		assert.Empty(t, p.Header.Get("Content-Encoding"), "after a refusal, this and later pages are uncompressed")
	}
	assert.Greater(t, len(ps), 3)
}

// ABORT-01
func TestABORT01_AbortTheUploadOfAFailedRun(t *testing.T) {
	s := newStubIntake(t)
	c := s.client(nil)
	require.NoError(t, c.Abort(Fail(CodeQueryFailed, "the database rejected the query (ORA-00942) "+strings.Repeat("é", 600))))
	r := s.recorded()[0]
	assert.Equal(t, "POST /abort", r.Method+" "+r.Path)
	var body struct {
		Error struct{ Code, Message string } `json:"error"`
	}
	require.NoError(t, json.Unmarshal(r.Body, &body))
	assert.Equal(t, CodeQueryFailed, body.Error.Code)
	assert.LessOrEqual(t, len(body.Error.Message), 1024)
	assert.True(t, strings.HasPrefix(body.Error.Message, "the database rejected the query (ORA-00942)"))

	// The abort has its own budget: it is sent even after the run's context ended.
}
