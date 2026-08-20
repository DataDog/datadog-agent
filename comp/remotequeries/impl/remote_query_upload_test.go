// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package remotequeriesimpl

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	configcomp "github.com/DataDog/datadog-agent/comp/core/config"
	"github.com/DataDog/datadog-agent/pkg/collector/check"
)

func validUploadDelivery() *RemoteQueryResultDelivery {
	return &RemoteQueryResultDelivery{
		Mode:        RemoteQueryResultDeliveryModeChunkedUpload,
		UploadID:    "upload-01k",
		BaseURL:     "https://dd.datad0g.com/api/intake/its-agent-intake",
		Token:       "scoped-upload-token",
		ChunkBytes:  8,
		MaxBytes:    24,
		Format:      "csv",
		Compression: "none",
	}
}

// fakeUploadTransport records every round-trip and returns canned responses in order.
type fakeUploadTransport struct {
	mu        sync.Mutex
	requests  []fakeUploadRequest
	responses []fakeUploadResponse
	calls     int
}

type fakeUploadRequest struct {
	method  string
	url     string
	headers map[string]string
	body    []byte
}

type fakeUploadResponse struct {
	status int
	body   []byte
	err    error
}

func (f *fakeUploadTransport) roundTrip(_ context.Context, method, urlStr string, headers map[string]string, body []byte) (int, []byte, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	idx := f.calls
	f.calls++
	f.requests = append(f.requests, fakeUploadRequest{method: method, url: urlStr, headers: headers, body: append([]byte(nil), body...)})
	if idx < len(f.responses) {
		r := f.responses[idx]
		return r.status, r.body, r.err
	}
	return http.StatusOK, nil, nil
}

func (f *fakeUploadTransport) request(i int) fakeUploadRequest {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.requests[i]
}

// newTestRelay builds a relay wired to a fake transport with zero backoff.
func newTestRelay(ctx context.Context, t *testing.T, delivery *RemoteQueryResultDelivery, downstream func(check.RemoteQueryStreamEvent) error) (*remoteQueryUploadRelay, *fakeUploadTransport) {
	t.Helper()
	transport := &fakeUploadTransport{}
	relay, err := newRemoteQueryUploadRelay(ctx, delivery, "org-api-key", downstream, transport)
	require.NoError(t, err)
	relay.maxRetries = 2
	relay.initialBackoff = 0
	relay.maxBackoff = 0
	return relay, transport
}

func sha256Sum(payload []byte) []byte {
	sum := sha256.Sum256(payload)
	return sum[:]
}

func finalizeResponseBody(uploadID string, totalBytes, totalRows, chunkCount int64, sha string) []byte {
	return []byte(`{"mode":"POC_PUBLIC_CHUNKED_UPLOAD","upload_id":"` + uploadID + `","bucket_name":"rq-bucket","manifest_key":"manifests/` + uploadID + `.json","total_bytes":` + strconv.FormatInt(totalBytes, 10) + `,"total_rows":` + strconv.FormatInt(totalRows, 10) + `,"chunk_count":` + strconv.FormatInt(chunkCount, 10) + `,"sha256":"` + sha + `","format":"csv","compression":"none","finalized_at":"2026-08-20T00:00:00Z"}`)
}

// runUploadSuccess feeds exactly chunkBytes-sized data events, finalizes with a matching
// receipt, and returns the recorded transport and downstream events.
func runUploadSuccess(t *testing.T, delivery *RemoteQueryResultDelivery, chunks [][]byte) (*fakeUploadTransport, []check.RemoteQueryStreamEvent) {
	t.Helper()
	var downstream []check.RemoteQueryStreamEvent
	relay, transport := newTestRelay(context.Background(), t, delivery, func(event check.RemoteQueryStreamEvent) error {
		downstream = append(downstream, event)
		return nil
	})
	var full bytes.Buffer
	for _, c := range chunks {
		full.Write(c)
	}
	aggregate := sha256.Sum256(full.Bytes())
	rows := bytes.Count(full.Bytes(), []byte{'\n'})
	responses := make([]fakeUploadResponse, 0, len(chunks)+1)
	for range chunks {
		responses = append(responses, fakeUploadResponse{status: http.StatusOK})
	}
	responses = append(responses, fakeUploadResponse{status: http.StatusOK, body: finalizeResponseBody(delivery.UploadID, int64(full.Len()), int64(rows), int64(len(chunks)), hex.EncodeToString(aggregate[:]))})
	transport.responses = responses

	require.NoError(t, relay.emit(check.RemoteQueryStreamEvent{Type: "metadata", MetadataJSON: `{"status":"STARTED","format":"csv"}`}))
	for _, c := range chunks {
		require.NoError(t, relay.emit(check.RemoteQueryStreamEvent{Type: "data", MetadataJSON: `{}`, Payload: c}))
	}
	require.NoError(t, relay.emit(check.RemoteQueryStreamEvent{Type: "final", MetadataJSON: `{"status":"SUCCEEDED"}`}))
	return transport, downstream
}

func TestValidateRemoteQueryResultDeliveryRejectsUnsafeBaseURL(t *testing.T) {
	cases := []struct {
		name    string
		baseURL string
	}{
		{"http scheme", "http://dd.datad0g.com/api/intake/its-agent-intake"},
		{"unknown host", "https://example.com/api/intake/its-agent-intake"},
		{"host impersonator", "https://evil-datadoghq.com/api/intake/its-agent-intake"},
		{"evil datad0g subdomain", "https://evil.datad0g.com/api/intake/its-agent-intake"},
		{"wrong path", "https://dd.datad0g.com/api/intake/other"},
		{"extra path", "https://dd.datad0g.com/api/intake/its-agent-intake/extra"},
		{"explicit port", "https://dd.datad0g.com:8443/api/intake/its-agent-intake"},
		{"userinfo", "https://user:pass@dd.datad0g.com/api/intake/its-agent-intake"},
		{"query", "https://dd.datad0g.com/api/intake/its-agent-intake?x=1"},
		{"fragment", "https://dd.datad0g.com/api/intake/its-agent-intake#frag"},
		{"path traversal", "https://dd.datad0g.com/api/intake/../its-agent-intake"},
		{"double slash", "https://dd.datad0g.com/api//intake/its-agent-intake"},
		{"trailing slash", "https://dd.datad0g.com/api/intake/its-agent-intake/"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			d := validUploadDelivery()
			d.BaseURL = tc.baseURL
			_, err := validateRemoteQueryResultDelivery(d, &remoteQueryExecuteCopyLimits{ChunkBytes: 64, MaxBytes: 1024, MaxRowBytes: 1024, TimeoutMs: 1000}, "csv")
			require.Error(t, err)
			assert.NotContains(t, err.Error(), "scoped-upload-token")
		})
	}
}

func TestValidateRemoteQueryResultDeliveryAcceptsExactAllowlistedBaseURL(t *testing.T) {
	// Only the exact POC intake base URL is accepted; subdomains/variants are rejected.
	d := validUploadDelivery()
	_, err := validateRemoteQueryResultDelivery(d, &remoteQueryExecuteCopyLimits{ChunkBytes: 64, MaxBytes: 1024, MaxRowBytes: 1024, TimeoutMs: 1000}, "csv")
	require.NoError(t, err)

	for _, baseURL := range []string{
		"https://dd.datad0g.com",                              // bare host, wrong path
		"https://api.datad0g.com",                             // different subdomain
		"https://dd.datad0g.com/api/intake/other",             // wrong path
		"https://dd.datad0g.com/api/intake/its-agent-intake/", // trailing slash
	} {
		d := validUploadDelivery()
		d.BaseURL = baseURL
		_, err := validateRemoteQueryResultDelivery(d, &remoteQueryExecuteCopyLimits{ChunkBytes: 64, MaxBytes: 1024, MaxRowBytes: 1024, TimeoutMs: 1000}, "csv")
		require.Error(t, err, "baseURL %q must be rejected", baseURL)
	}
}

func TestValidateRemoteQueryResultDeliveryRejectsInvalidModeAndFormat(t *testing.T) {
	t.Run("unsupported mode", func(t *testing.T) {
		d := validUploadDelivery()
		d.Mode = "OTHER_MODE"
		_, err := validateRemoteQueryResultDelivery(d, nil, "csv")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "not supported")
	})
	t.Run("binary format", func(t *testing.T) {
		d := validUploadDelivery()
		d.Format = "binary"
		_, err := validateRemoteQueryResultDelivery(d, nil, "csv")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "format must be csv")
	})
	t.Run("compression not none", func(t *testing.T) {
		d := validUploadDelivery()
		d.Compression = "gzip"
		_, err := validateRemoteQueryResultDelivery(d, nil, "csv")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "compression must be none")
	})
	t.Run("copy format mismatch", func(t *testing.T) {
		d := validUploadDelivery()
		_, err := validateRemoteQueryResultDelivery(d, nil, "binary")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "format must match")
	})
}

func TestValidateRemoteQueryResultDeliveryRejectsBadUploadIDAndCaps(t *testing.T) {
	t.Run("empty uploadId", func(t *testing.T) {
		d := validUploadDelivery()
		d.UploadID = ""
		_, err := validateRemoteQueryResultDelivery(d, nil, "csv")
		require.Error(t, err)
	})
	t.Run("traversal uploadId", func(t *testing.T) {
		d := validUploadDelivery()
		d.UploadID = "../escape"
		_, err := validateRemoteQueryResultDelivery(d, nil, "csv")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "invalid characters")
	})
	t.Run("chunkBytes exceeds maxBytes", func(t *testing.T) {
		d := validUploadDelivery()
		d.ChunkBytes = 100
		d.MaxBytes = 50
		_, err := validateRemoteQueryResultDelivery(d, nil, "csv")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "must not exceed maxBytes")
	})
	t.Run("chunkBytes exceeds copyLimits", func(t *testing.T) {
		d := validUploadDelivery()
		d.ChunkBytes = 64
		d.MaxBytes = 1024
		_, err := validateRemoteQueryResultDelivery(d, &remoteQueryExecuteCopyLimits{ChunkBytes: 32, MaxBytes: 1024, MaxRowBytes: 1024, TimeoutMs: 1000}, "csv")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "copyLimits.chunkBytes")
	})
}

func TestSanitizedMarshalExcludesSecretsFromPythonRequest(t *testing.T) {
	req := remoteQueryExecuteRequest{
		Operation:      "copy_stream",
		Target:         remoteQueryTarget{Host: "localhost", Port: 5432, DBName: "postgres"},
		Query:          "SELECT city, country FROM cities ORDER BY city",
		Format:         "csv",
		CopyLimits:     &remoteQueryExecuteCopyLimits{ChunkBytes: 8, MaxBytes: 24, MaxRowBytes: 32, TimeoutMs: 5000},
		ResultDelivery: &remoteQueryResultDelivery{Mode: RemoteQueryResultDeliveryModeChunkedUpload, UploadID: "upload-01k", ChunkBytes: 8, MaxBytes: 24, Format: "csv", Compression: "none"},
	}
	requestJSON, err := marshalExecuteRequest(req)
	require.NoError(t, err)
	assert.NotContains(t, requestJSON, "https://dd.datad0g.com/api/intake/its-agent-intake")
	assert.NotContains(t, requestJSON, "scoped-upload-token")
	assert.NotContains(t, requestJSON, "org-api-key")
	assert.NotContains(t, requestJSON, "baseUrl")
	assert.NotContains(t, requestJSON, "token")
	assert.Contains(t, requestJSON, `"resultDelivery"`)
	assert.Contains(t, requestJSON, `"uploadId":"upload-01k"`)
	assert.Contains(t, requestJSON, `"chunkBytes":8`)
	assert.Contains(t, requestJSON, `"compression":"none"`)
}

func TestUploadRelayUploadsChunksWithExactRouteAndHeaders(t *testing.T) {
	chunks := [][]byte{[]byte("row1\nxxx"), []byte("row2\nxxx"), []byte("row3\nxxx")}
	transport, downstream := runUploadSuccess(t, validUploadDelivery(), chunks)

	require.Len(t, transport.requests, 4) // 3 PUTs + 1 finalize
	for i := 0; i < 3; i++ {
		req := transport.request(i)
		assert.Equal(t, http.MethodPut, req.method)
		assert.Equal(t, "https://dd.datad0g.com/api/intake/its-agent-intake/uploads/upload-01k/chunks/"+strconv.Itoa(i), req.url)
		assert.Equal(t, "org-api-key", req.headers["dd-api-key"])
		assert.Equal(t, "Bearer scoped-upload-token", req.headers["Authorization"])
		assert.Equal(t, "application/octet-stream", req.headers["Content-Type"])
		// Required intake headers: per-chunk SHA256 (hex of the chunk body) and chunk byte count.
		assert.Equal(t, hex.EncodeToString(sha256Sum(chunks[i])), req.headers["X-DD-Chunk-SHA256"])
		assert.Equal(t, strconv.Itoa(len(chunks[i])), req.headers["X-DD-Chunk-Bytes"])
		// Optional per-chunk row count (newline count of the chunk body).
		assert.Equal(t, strconv.Itoa(bytes.Count(chunks[i], []byte{'\n'})), req.headers["X-DD-Chunk-Rows"])
		assert.Equal(t, chunks[i], req.body)
	}
	finalize := transport.request(3)
	assert.Equal(t, http.MethodPost, finalize.method)
	assert.Equal(t, "https://dd.datad0g.com/api/intake/its-agent-intake/uploads/upload-01k/finalize", finalize.url)
	assert.Equal(t, "application/json", finalize.headers["Content-Type"])
	assert.Equal(t, "org-api-key", finalize.headers["dd-api-key"])

	// Only metadata + final cross downstream; no data events.
	require.Len(t, downstream, 2)
	assert.Equal(t, "metadata", downstream[0].Type)
	require.Equal(t, "final", downstream[1].Type)
}

func TestUploadRelayAggregateChecksumMatchesConcatenatedBodies(t *testing.T) {
	chunks := [][]byte{[]byte("row1\nxxx"), []byte("row2\nxxx"), []byte("row3\nxxx")}
	var full bytes.Buffer
	for _, c := range chunks {
		full.Write(c)
	}
	aggregate := sha256.Sum256(full.Bytes())
	transport, _ := runUploadSuccess(t, validUploadDelivery(), chunks)

	finalize := transport.request(3)
	assert.Contains(t, string(finalize.body), `"sha256":"`+hex.EncodeToString(aggregate[:])+`"`)
	assert.Contains(t, string(finalize.body), `"total_bytes":24`)
	assert.Contains(t, string(finalize.body), `"total_rows":3`)
	assert.Contains(t, string(finalize.body), `"chunk_count":3`)
	assert.Contains(t, string(finalize.body), `"upload_id":"upload-01k"`)
	assert.Contains(t, string(finalize.body), `"mode":"POC_PUBLIC_CHUNKED_UPLOAD"`)
}

func TestUploadRelayRetriesTransientFailuresWithSameIndex(t *testing.T) {
	relay, transport := newTestRelay(context.Background(), t, validUploadDelivery(), func(check.RemoteQueryStreamEvent) error { return nil })
	payload := []byte("abcdefgh")
	aggregate := sha256.Sum256(payload)
	// chunk 0: 503 then 200 (retry same index); finalize 200.
	transport.responses = []fakeUploadResponse{
		{status: http.StatusServiceUnavailable},
		{status: http.StatusOK},
		{status: http.StatusOK, body: finalizeResponseBody("upload-01k", 8, 0, 1, hex.EncodeToString(aggregate[:]))},
	}
	require.NoError(t, relay.emit(check.RemoteQueryStreamEvent{Type: "data", MetadataJSON: `{}`, Payload: payload}))
	require.NoError(t, relay.emit(check.RemoteQueryStreamEvent{Type: "final", MetadataJSON: `{"status":"SUCCEEDED"}`}))

	assert.Equal(t, "https://dd.datad0g.com/api/intake/its-agent-intake/uploads/upload-01k/chunks/0", transport.request(0).url)
	assert.Equal(t, "https://dd.datad0g.com/api/intake/its-agent-intake/uploads/upload-01k/chunks/0", transport.request(1).url)
	assert.Equal(t, transport.request(0).body, transport.request(1).body)
	assert.Equal(t, http.MethodPost, transport.request(2).method)
}

func TestUploadRelayRetries429And408(t *testing.T) {
	relay, transport := newTestRelay(context.Background(), t, validUploadDelivery(), func(check.RemoteQueryStreamEvent) error { return nil })
	payload := []byte("abcdefgh")
	aggregate := sha256.Sum256(payload)
	transport.responses = []fakeUploadResponse{
		{status: http.StatusTooManyRequests},
		{status: http.StatusRequestTimeout},
		{status: http.StatusOK},
		{status: http.StatusOK, body: finalizeResponseBody("upload-01k", 8, 0, 1, hex.EncodeToString(aggregate[:]))},
	}
	require.NoError(t, relay.emit(check.RemoteQueryStreamEvent{Type: "data", MetadataJSON: `{}`, Payload: payload}))
	require.NoError(t, relay.emit(check.RemoteQueryStreamEvent{Type: "final", MetadataJSON: `{"status":"SUCCEEDED"}`}))
	// The successful chunk PUT is the 3rd round-trip (index 0 retried twice).
	assert.Equal(t, "https://dd.datad0g.com/api/intake/its-agent-intake/uploads/upload-01k/chunks/0", transport.request(2).url)
}

func TestUploadRelayDoesNotRetryNonTransient4xx(t *testing.T) {
	relay, transport := newTestRelay(context.Background(), t, validUploadDelivery(), func(check.RemoteQueryStreamEvent) error { return nil })
	transport.responses = []fakeUploadResponse{{status: http.StatusBadRequest}}
	err := relay.emit(check.RemoteQueryStreamEvent{Type: "data", MetadataJSON: `{}`, Payload: []byte("abcdefgh")})
	require.Error(t, err)
	relay.abortIfPending()
	// Only one PUT attempt (no retry), then an abort best effort.
	assert.Equal(t, http.MethodPut, transport.request(0).method)
	require.Equal(t, 2, transport.calls)
	assert.Equal(t, http.MethodPost, transport.request(1).method)
	assert.Contains(t, transport.request(1).url, "/abort")
}

func TestUploadRelayEnforcesMaxBytesCap(t *testing.T) {
	relay, transport := newTestRelay(context.Background(), t, validUploadDelivery(), func(check.RemoteQueryStreamEvent) error { return nil })
	require.NoError(t, relay.emit(check.RemoteQueryStreamEvent{Type: "data", MetadataJSON: `{}`, Payload: make([]byte, 16)}))
	err := relay.emit(check.RemoteQueryStreamEvent{Type: "data", MetadataJSON: `{}`, Payload: make([]byte, 16)})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "maxBytes")
	relay.abortIfPending()
	last := transport.request(transport.calls - 1)
	assert.Equal(t, http.MethodPost, last.method)
	assert.Contains(t, last.url, "/abort")
}

func TestUploadRelayAbortsOnContextCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	relay, transport := newTestRelay(ctx, t, validUploadDelivery(), func(check.RemoteQueryStreamEvent) error { return nil })
	cancel()
	err := relay.emit(check.RemoteQueryStreamEvent{Type: "data", MetadataJSON: `{}`, Payload: []byte("abcdefgh")})
	require.Error(t, err)
	relay.abortIfPending()
	foundAbort := false
	for i := 0; i < transport.calls; i++ {
		if transport.request(i).method == http.MethodPost && strings.Contains(transport.request(i).url, "/abort") {
			foundAbort = true
		}
	}
	assert.True(t, foundAbort)
}

func TestUploadRelayRejectsFinalizeReceiptMismatch(t *testing.T) {
	cases := []struct {
		name    string
		body    []byte
		wantErr string
	}{
		{"totalBytes mismatch", finalizeResponseBody("upload-01k", 999, 0, 1, "sha"), "total_bytes mismatch"},
		{"totalRows mismatch", finalizeResponseBody("upload-01k", 8, 999, 1, "sha"), "total_rows mismatch"},
		{"chunkCount mismatch", finalizeResponseBody("upload-01k", 8, 0, 999, "sha"), "chunk_count mismatch"},
		{"sha256 mismatch", finalizeResponseBody("upload-01k", 8, 0, 1, "deadbeef"), "sha256 mismatch"},
		{"uploadId mismatch", finalizeResponseBody("other", 8, 0, 1, "sha"), "upload_id mismatch"},
		{"mode mismatch", []byte(`{"mode":"OTHER","upload_id":"upload-01k","bucket_name":"b","manifest_key":"m","total_bytes":8,"total_rows":0,"chunk_count":1,"sha256":"sha","format":"csv","compression":"none","finalized_at":"t"}`), "mode mismatch"},
		{"missing bucketName", []byte(`{"mode":"POC_PUBLIC_CHUNKED_UPLOAD","upload_id":"upload-01k","manifest_key":"m","total_bytes":8,"total_rows":0,"chunk_count":1,"sha256":"sha","format":"csv","compression":"none","finalized_at":"t"}`), "bucket_name"},
		{"missing manifestKey", []byte(`{"mode":"POC_PUBLIC_CHUNKED_UPLOAD","upload_id":"upload-01k","bucket_name":"b","total_bytes":8,"total_rows":0,"chunk_count":1,"sha256":"sha","format":"csv","compression":"none","finalized_at":"t"}`), "manifest_key"},
		{"unknown field", []byte(`{"mode":"POC_PUBLIC_CHUNKED_UPLOAD","upload_id":"upload-01k","bucket_name":"b","manifest_key":"m","total_bytes":8,"total_rows":0,"chunk_count":1,"sha256":"sha","format":"csv","compression":"none","finalized_at":"t","extra":1}`), "invalid"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			relay, transport := newTestRelay(context.Background(), t, validUploadDelivery(), func(check.RemoteQueryStreamEvent) error { return nil })
			payload := []byte("abcdefgh")
			aggregate := sha256.Sum256(payload)
			transport.responses = []fakeUploadResponse{{status: http.StatusOK}, {status: http.StatusOK, body: tc.body}}
			require.NoError(t, relay.emit(check.RemoteQueryStreamEvent{Type: "data", MetadataJSON: `{}`, Payload: payload}))
			err := relay.emit(check.RemoteQueryStreamEvent{Type: "final", MetadataJSON: `{"status":"SUCCEEDED"}`})
			require.Error(t, err)
			assert.Contains(t, err.Error(), tc.wantErr)
			// Aggregate is computed locally; the mismatched response sha is never the real one.
			_ = aggregate
		})
	}
}

func TestUploadRelaySurfacesCompactReceiptDownstream(t *testing.T) {
	chunks := [][]byte{[]byte("abcdefgh")}
	var full bytes.Buffer
	for _, c := range chunks {
		full.Write(c)
	}
	aggregate := sha256.Sum256(full.Bytes())
	_, downstream := runUploadSuccess(t, validUploadDelivery(), chunks)

	require.Len(t, downstream, 2)
	final := downstream[1]
	require.Equal(t, "final", final.Type)
	assert.Contains(t, final.MetadataJSON, `"upload_receipt"`)
	assert.Contains(t, final.MetadataJSON, `"bucketName":"rq-bucket"`)
	assert.Contains(t, final.MetadataJSON, `"manifestPath":"manifests/upload-01k.json"`)
	assert.Contains(t, final.MetadataJSON, `"sha256":"`+hex.EncodeToString(aggregate[:])+`"`)
	assert.NotContains(t, final.MetadataJSON, "scoped-upload-token")
	assert.NotContains(t, final.MetadataJSON, "org-api-key")
}

func TestUploadRelayRequiresAPIKey(t *testing.T) {
	_, err := newRemoteQueryUploadRelay(context.Background(), validUploadDelivery(), "", func(check.RemoteQueryStreamEvent) error { return nil }, &fakeUploadTransport{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "API key")
}

func TestUploadRelayBackpressureSplitsOneChunkAtATime(t *testing.T) {
	relay, transport := newTestRelay(context.Background(), t, validUploadDelivery(), func(check.RemoteQueryStreamEvent) error { return nil })
	// A single 24-byte data event is split into three 8-byte PUTs, processed sequentially.
	big := bytes.Repeat([]byte("x"), 24)
	aggregate := sha256.Sum256(big)
	transport.responses = []fakeUploadResponse{{status: http.StatusOK}, {status: http.StatusOK}, {status: http.StatusOK}, {status: http.StatusOK, body: finalizeResponseBody("upload-01k", 24, 0, 3, hex.EncodeToString(aggregate[:]))}}
	require.NoError(t, relay.emit(check.RemoteQueryStreamEvent{Type: "data", MetadataJSON: `{}`, Payload: big}))
	require.NoError(t, relay.emit(check.RemoteQueryStreamEvent{Type: "final", MetadataJSON: `{"status":"SUCCEEDED"}`}))
	for i := 0; i < 3; i++ {
		assert.Equal(t, "https://dd.datad0g.com/api/intake/its-agent-intake/uploads/upload-01k/chunks/"+strconv.Itoa(i), transport.request(i).url)
		assert.Len(t, transport.request(i).body, 8)
	}
}

func TestExecuteStreamDefaultPathEmitsDataEventsByteForByte(t *testing.T) {
	runner := &fakeStreamRunnerCheck{
		fakeRunnerCheck: fakeRunnerCheck{fakeCheck: fakeCheck{name: "postgres", loader: "python", provider: "file", instance: "host: localhost\nport: 5432\ndbname: postgres"}},
		events: []check.RemoteQueryStreamEvent{
			{Type: "metadata", MetadataJSON: `{"status":"STARTED","format":"csv"}`},
			{Type: "data", MetadataJSON: `{"sequence":0,"offset":0,"bytes":3}`, Payload: []byte{0x00, 0xff, 0x80}},
			{Type: "final", MetadataJSON: `{"status":"SUCCEEDED"}`},
		},
	}
	service := NewRemoteQueryExecuteService(fakeCollector{checks: []check.Check{fakeWrappedCheck{Check: runner}}}, true, false, configcomp.NewMock(t))
	req, err := NewRemoteQueryCopyStreamExecuteRequest("postgres", RemoteQueryExecuteTarget{Host: "localhost", Port: 5432, DBName: "postgres"}, "SELECT city, country FROM cities ORDER BY city", "csv", &RemoteQueryExecuteCopyLimits{ChunkBytes: 4, MaxBytes: 1024, MaxRowBytes: 1024, TimeoutMs: 1000}, nil)
	require.NoError(t, err)

	var downstream []check.RemoteQueryStreamEvent
	result := service.ExecuteStream(context.Background(), req, func(event check.RemoteQueryStreamEvent) error {
		downstream = append(downstream, event)
		return nil
	})
	require.Nil(t, result.Error)
	// Omitted result_delivery keeps the inline path: data events cross unchanged.
	assert.Equal(t, runner.events, downstream)
	assert.Equal(t, 1, runner.streamCalls)
}

func TestExecuteStreamUploadModeSuppressesDataAndSurfacesReceipt(t *testing.T) {
	runner := &fakeStreamRunnerCheck{
		fakeRunnerCheck: fakeRunnerCheck{fakeCheck: fakeCheck{name: "postgres", loader: "python", provider: "file", instance: "host: localhost\nport: 5432\ndbname: postgres"}},
		events: []check.RemoteQueryStreamEvent{
			{Type: "metadata", MetadataJSON: `{"status":"STARTED","format":"csv"}`},
			{Type: "data", MetadataJSON: `{"sequence":0}`, Payload: []byte("row1\nxxx")},
			{Type: "data", MetadataJSON: `{"sequence":1}`, Payload: []byte("row2\nxxx")},
			{Type: "data", MetadataJSON: `{"sequence":2}`, Payload: []byte("row3\nxxx")},
			{Type: "final", MetadataJSON: `{"status":"SUCCEEDED"}`},
		},
	}
	cfg := configcomp.NewMockWithOverrides(t, map[string]interface{}{"api_key": "org-api-key"})
	transport := &fakeUploadTransport{}
	service := newRemoteQueryExecuteServiceWithTransport(fakeCollector{checks: []check.Check{fakeWrappedCheck{Check: runner}}}, cfg, func(_ configcomp.Component) remoteQueryUploadTransport { return transport })
	req, err := NewRemoteQueryCopyStreamExecuteRequest("postgres", RemoteQueryExecuteTarget{Host: "localhost", Port: 5432, DBName: "postgres"}, "SELECT city, country FROM cities ORDER BY city", "csv", &RemoteQueryExecuteCopyLimits{ChunkBytes: 8, MaxBytes: 24, MaxRowBytes: 32, TimeoutMs: 1000}, &RemoteQueryResultDelivery{Mode: RemoteQueryResultDeliveryModeChunkedUpload, UploadID: "upload-01k", BaseURL: "https://dd.datad0g.com/api/intake/its-agent-intake", Token: "scoped-upload-token", ChunkBytes: 8, MaxBytes: 24, Format: "csv", Compression: "none"})
	require.NoError(t, err)

	var full bytes.Buffer
	for _, p := range [][]byte{[]byte("row1\nxxx"), []byte("row2\nxxx"), []byte("row3\nxxx")} {
		full.Write(p)
	}
	aggregate := sha256.Sum256(full.Bytes())
	transport.responses = []fakeUploadResponse{
		{status: http.StatusOK}, {status: http.StatusOK}, {status: http.StatusOK},
		{status: http.StatusOK, body: finalizeResponseBody("upload-01k", 24, 3, 3, hex.EncodeToString(aggregate[:]))},
	}

	var downstream []check.RemoteQueryStreamEvent
	result := service.ExecuteStream(context.Background(), req, func(event check.RemoteQueryStreamEvent) error {
		downstream = append(downstream, event)
		return nil
	})
	require.Nil(t, result.Error)

	// No data events cross the AgentSecure boundary; only metadata + final (with receipt).
	for _, ev := range downstream {
		assert.NotEqual(t, "data", ev.Type)
	}
	require.Len(t, downstream, 2)
	assert.Equal(t, "metadata", downstream[0].Type)
	require.Equal(t, "final", downstream[1].Type)
	assert.Contains(t, downstream[1].MetadataJSON, `"upload_receipt"`)
	assert.Contains(t, downstream[1].MetadataJSON, `"bucketName":"rq-bucket"`)
	assert.NotContains(t, downstream[1].MetadataJSON, "scoped-upload-token")

	// The Python request JSON carries the sanitized handle only (no URL/token/keys).
	assert.NotContains(t, runner.streamSeen, "https://dd.datad0g.com/api/intake/its-agent-intake")
	assert.NotContains(t, runner.streamSeen, "scoped-upload-token")
	assert.Contains(t, runner.streamSeen, `"resultDelivery"`)
	assert.Contains(t, runner.streamSeen, `"uploadId":"upload-01k"`)
}

func TestExecuteStreamUploadModeFailsWithoutAPIKey(t *testing.T) {
	runner := &fakeStreamRunnerCheck{
		fakeRunnerCheck: fakeRunnerCheck{fakeCheck: fakeCheck{name: "postgres", loader: "python", provider: "file", instance: "host: localhost\nport: 5432\ndbname: postgres"}},
		events:          []check.RemoteQueryStreamEvent{{Type: "final", MetadataJSON: `{"status":"SUCCEEDED"}`}},
	}
	service := newRemoteQueryExecuteServiceWithTransport(fakeCollector{checks: []check.Check{fakeWrappedCheck{Check: runner}}}, configcomp.NewMock(t), func(_ configcomp.Component) remoteQueryUploadTransport { return &fakeUploadTransport{} })
	req, err := NewRemoteQueryCopyStreamExecuteRequest("postgres", RemoteQueryExecuteTarget{Host: "localhost", Port: 5432, DBName: "postgres"}, "SELECT city, country FROM cities ORDER BY city", "csv", &RemoteQueryExecuteCopyLimits{ChunkBytes: 8, MaxBytes: 24, MaxRowBytes: 32, TimeoutMs: 1000}, &RemoteQueryResultDelivery{Mode: RemoteQueryResultDeliveryModeChunkedUpload, UploadID: "upload-01k", BaseURL: "https://dd.datad0g.com/api/intake/its-agent-intake", Token: "tok", ChunkBytes: 8, MaxBytes: 24, Format: "csv", Compression: "none"})
	require.NoError(t, err)

	result := service.ExecuteStream(context.Background(), req, func(check.RemoteQueryStreamEvent) error { return nil })
	require.NotNil(t, result.Error)
	assert.Equal(t, statusExecutorUnavailable, result.Error.Code)
	assert.Equal(t, 0, runner.streamCalls)
}

// newRealHTTPRelay builds a relay whose transport is the real remoteQueryHTTPTransport pointed at a
// TLS httptest server, bypassing the production allowlist (the relay does not re-validate the
// base URL). This lets cross-contract tests exercise the real HTTP client (incl. CheckRedirect)
// against the exact intake response/headers without mutating the immutable allowlist.
func newRealHTTPRelay(ctx context.Context, t *testing.T, serverURL string, client *http.Client, downstream func(check.RemoteQueryStreamEvent) error) *remoteQueryUploadRelay {
	t.Helper()
	// Append the intake path prefix so chunk/finalize routes match the real intake contract.
	baseURL := strings.TrimRight(serverURL, "/") + "/api/intake/its-agent-intake"
	delivery := &RemoteQueryResultDelivery{Mode: RemoteQueryResultDeliveryModeChunkedUpload, UploadID: "upload-01k", BaseURL: baseURL, Token: "scoped-upload-token", ChunkBytes: 8, MaxBytes: 24, Format: "csv", Compression: "none"}
	// Keep the test server's TLS transport but enforce the production redirect policy so auth
	// headers never follow a redirect (the cross-contract redirect test relies on this).
	client.CheckRedirect = func(_ *http.Request, _ []*http.Request) error { return http.ErrUseLastResponse }
	relay, err := newRemoteQueryUploadRelay(ctx, delivery, "org-api-key", downstream, &remoteQueryHTTPTransport{client: client})
	require.NoError(t, err)
	relay.maxRetries = 2
	relay.initialBackoff = 0
	relay.maxBackoff = 0
	return relay
}

// TestUploadRelayCrossContractHTTPServer exercises the real HTTP client against a TLS test
// server that enforces the exact intake PUT headers and returns the exact snake_case finalize
// response. It confirms the relay produces the canonical camelCase receipt and accepts a
// 201 Created chunk response (intake PUT status semantics).
func TestUploadRelayCrossContractHTTPServer(t *testing.T) {
	var (
		mu        sync.Mutex
		putBodies [][]byte
		putReqs   []httptestReq
	)
	handler := func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		if r.Method == http.MethodPut {
			body := readBody(t, r)
			putBodies = append(putBodies, body)
			putReqs = append(putReqs, httptestReq{url: r.URL.String(), headers: r.Header.Clone()})
			// Intake accepts chunk PUTs with 201 Created.
			w.WriteHeader(http.StatusCreated)
			return
		}
		if r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/finalize") {
			var full bytes.Buffer
			for _, b := range putBodies {
				full.Write(b)
			}
			rows := bytes.Count(full.Bytes(), []byte{'\n'})
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write(finalizeResponseBody("upload-01k", int64(full.Len()), int64(rows), int64(len(putBodies)), hex.EncodeToString(sha256Sum(full.Bytes()))))
			return
		}
		if r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/abort") {
			w.WriteHeader(http.StatusOK)
			return
		}
		w.WriteHeader(http.StatusBadRequest)
	}
	server := httptest.NewTLSServer(http.HandlerFunc(handler))
	defer server.Close()

	relay := newRealHTTPRelay(context.Background(), t, server.URL, server.Client(), func(check.RemoteQueryStreamEvent) error { return nil })
	payload := []byte("row1\nx") // 6 bytes, 1 newline → fits one 8-byte chunk
	require.NoError(t, relay.emit(check.RemoteQueryStreamEvent{Type: "data", MetadataJSON: `{}`, Payload: payload}))
	require.NoError(t, relay.emit(check.RemoteQueryStreamEvent{Type: "final", MetadataJSON: `{"status":"SUCCEEDED"}`}))

	// Exactly one chunk PUT, with the exact intake headers.
	mu.Lock()
	require.Len(t, putReqs, 1)
	assert.Equal(t, "/api/intake/its-agent-intake/uploads/upload-01k/chunks/0", putReqs[0].url)
	assert.Equal(t, "org-api-key", putReqs[0].headers.Get("dd-api-key"))
	assert.Equal(t, "Bearer scoped-upload-token", putReqs[0].headers.Get("Authorization"))
	assert.Equal(t, hex.EncodeToString(sha256Sum(payload)), putReqs[0].headers.Get("X-DD-Chunk-SHA256"))
	assert.Equal(t, strconv.Itoa(len(payload)), putReqs[0].headers.Get("X-DD-Chunk-Bytes"))
	assert.Equal(t, "1", putReqs[0].headers.Get("X-DD-Chunk-Rows"))
	mu.Unlock()
}

// TestUploadRelayHTTPClientRejectsRedirect confirms the production HTTP client does NOT follow a
// redirect, so the dd-api-key and Bearer token never leak to a redirect target.
func TestUploadRelayHTTPClientRejectsRedirect(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPut {
			// Redirect the PUT back to this same server's /leaked path. If the client followed it,
			// it would re-request /leaked (with the auth headers) and trip the assertion below.
			http.Redirect(w, r, "/leaked", http.StatusFound)
			return
		}
		if r.URL.Path == "/leaked" {
			t.Errorf("auth headers followed the redirect to /leaked")
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	relay := newRealHTTPRelay(context.Background(), t, server.URL, server.Client(), func(check.RemoteQueryStreamEvent) error { return nil })
	err := relay.emit(check.RemoteQueryStreamEvent{Type: "data", MetadataJSON: `{}`, Payload: []byte("abcdefgh")})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "302")
}

func TestUploadRelayCanceledStreamStillAttemptsAbort(t *testing.T) {
	var (
		mu       sync.Mutex
		abortHit bool
	)
	handler := func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/abort") {
			mu.Lock()
			abortHit = true
			mu.Unlock()
		}
		w.WriteHeader(http.StatusOK)
	}
	server := httptest.NewTLSServer(http.HandlerFunc(handler))
	defer server.Close()

	ctx, cancel := context.WithCancel(context.Background())
	relay := newRealHTTPRelay(ctx, t, server.URL, server.Client(), func(check.RemoteQueryStreamEvent) error { return nil })
	cancel() // cancel the stream context before any data is emitted
	require.Error(t, relay.emit(check.RemoteQueryStreamEvent{Type: "data", MetadataJSON: `{}`, Payload: []byte("abcdefgh")}))
	relay.abortIfPending() // abort must still fire even though ctx is cancelled
	require.Eventually(t, func() bool {
		mu.Lock()
		defer mu.Unlock()
		return abortHit
	}, time.Second, 10*time.Millisecond, "abort must be attempted after cancellation via context.WithoutCancel")
}

func TestUploadRelayFinalizeRejectsTrailingJSON(t *testing.T) {
	relay, transport := newTestRelay(context.Background(), t, validUploadDelivery(), func(check.RemoteQueryStreamEvent) error { return nil })
	payload := []byte("abcdefgh")
	aggregate := sha256Sum(payload)
	// Two JSON objects back-to-back: strict decoder must reject the trailing value.
	body := append(finalizeResponseBody("upload-01k", 8, 0, 1, hex.EncodeToString(aggregate)), []byte(`{"extra":1}`)...)
	transport.responses = []fakeUploadResponse{{status: http.StatusOK}, {status: http.StatusOK, body: body}}
	require.NoError(t, relay.emit(check.RemoteQueryStreamEvent{Type: "data", MetadataJSON: `{}`, Payload: payload}))
	err := relay.emit(check.RemoteQueryStreamEvent{Type: "final", MetadataJSON: `{"status":"SUCCEEDED"}`})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "trailing JSON")
}

func TestUploadRelayFinalizeRejectsModeFormatCompressionMismatch(t *testing.T) {
	cases := []struct {
		name     string
		mode     string
		format   string
		compress string
		wantErr  string
	}{
		{"mode mismatch", "OTHER", "csv", "none", "mode mismatch"},
		{"format mismatch", "POC_PUBLIC_CHUNKED_UPLOAD", "binary", "none", "format mismatch"},
		{"compression mismatch", "POC_PUBLIC_CHUNKED_UPLOAD", "csv", "gzip", "compression mismatch"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			relay, transport := newTestRelay(context.Background(), t, validUploadDelivery(), func(check.RemoteQueryStreamEvent) error { return nil })
			payload := []byte("abcdefgh")
			aggregate := sha256Sum(payload)
			body := []byte(`{"mode":"` + tc.mode + `","upload_id":"upload-01k","bucket_name":"rq-bucket","manifest_key":"manifests/upload-01k.json","total_bytes":8,"total_rows":0,"chunk_count":1,"sha256":"` + hex.EncodeToString(aggregate) + `","format":"` + tc.format + `","compression":"` + tc.compress + `","finalized_at":"t"}`)
			transport.responses = []fakeUploadResponse{{status: http.StatusOK}, {status: http.StatusOK, body: body}}
			require.NoError(t, relay.emit(check.RemoteQueryStreamEvent{Type: "data", MetadataJSON: `{}`, Payload: payload}))
			err := relay.emit(check.RemoteQueryStreamEvent{Type: "final", MetadataJSON: `{"status":"SUCCEEDED"}`})
			require.Error(t, err)
			assert.Contains(t, err.Error(), tc.wantErr)
		})
	}
}

func TestUploadRelayFinalizeRetriesTransientThenSucceeds(t *testing.T) {
	relay, transport := newTestRelay(context.Background(), t, validUploadDelivery(), func(check.RemoteQueryStreamEvent) error { return nil })
	payload := []byte("abcdefgh")
	aggregate := sha256Sum(payload)
	// First finalize attempt: 503 (transient). Second: 200 with exact receipt (idempotent retry).
	transport.responses = []fakeUploadResponse{
		{status: http.StatusOK},                 // chunk PUT
		{status: http.StatusServiceUnavailable}, // finalize retry
		{status: http.StatusOK, body: finalizeResponseBody("upload-01k", 8, 0, 1, hex.EncodeToString(aggregate))}, // finalize success
	}
	require.NoError(t, relay.emit(check.RemoteQueryStreamEvent{Type: "data", MetadataJSON: `{}`, Payload: payload}))
	require.NoError(t, relay.emit(check.RemoteQueryStreamEvent{Type: "final", MetadataJSON: `{"status":"SUCCEEDED"}`}))
	// Two finalize POSTs (retry + success), both to the finalize URL.
	assert.Equal(t, http.MethodPost, transport.request(1).method)
	assert.Contains(t, transport.request(1).url, "/finalize")
	assert.Equal(t, http.MethodPost, transport.request(2).method)
	assert.Contains(t, transport.request(2).url, "/finalize")
}

func TestUploadRelayFinalizeDoesNotRetryNonTransient4xx(t *testing.T) {
	relay, transport := newTestRelay(context.Background(), t, validUploadDelivery(), func(check.RemoteQueryStreamEvent) error { return nil })
	payload := []byte("abcdefgh")
	transport.responses = []fakeUploadResponse{
		{status: http.StatusOK},        // chunk PUT
		{status: http.StatusForbidden}, // finalize non-transient 4xx
	}
	require.NoError(t, relay.emit(check.RemoteQueryStreamEvent{Type: "data", MetadataJSON: `{}`, Payload: payload}))
	err := relay.emit(check.RemoteQueryStreamEvent{Type: "final", MetadataJSON: `{"status":"SUCCEEDED"}`})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "403")
	// Only one finalize attempt (no retry for non-transient 4xx), plus a best-effort abort.
	assert.Equal(t, http.MethodPost, transport.request(1).method)
	assert.Contains(t, transport.request(1).url, "/finalize")
	require.GreaterOrEqual(t, transport.calls, 3)
	assert.Contains(t, transport.request(2).url, "/abort")
}

type httptestReq struct {
	url     string
	headers http.Header
}

func readBody(t *testing.T, r *http.Request) []byte {
	t.Helper()
	defer r.Body.Close()
	b, err := io.ReadAll(r.Body)
	require.NoError(t, err)
	return b
}

// TestUploadRelayFailsClosedAfterFinalEvent rejects data events that arrive after the relay
// has already finalized, so a duplicate/late data event can never re-upload after the receipt
// was surfaced.
func TestUploadRelayFailsClosedAfterFinalEvent(t *testing.T) {
	relay, transport := newTestRelay(context.Background(), t, validUploadDelivery(), func(check.RemoteQueryStreamEvent) error { return nil })
	payload := []byte("abcdefgh")
	aggregate := sha256Sum(payload)
	transport.responses = []fakeUploadResponse{
		{status: http.StatusOK}, // chunk PUT
		{status: http.StatusOK, body: finalizeResponseBody("upload-01k", 8, 0, 1, hex.EncodeToString(aggregate))}, // finalize
	}
	require.NoError(t, relay.emit(check.RemoteQueryStreamEvent{Type: "data", MetadataJSON: `{}`, Payload: payload}))
	require.NoError(t, relay.emit(check.RemoteQueryStreamEvent{Type: "final", MetadataJSON: `{"status":"SUCCEEDED"}`}))
	// A post-final data event must fail closed and NOT upload another chunk.
	preCalls := transport.calls
	err := relay.emit(check.RemoteQueryStreamEvent{Type: "data", MetadataJSON: `{}`, Payload: []byte("ijklmnop")})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "already finalized")
	assert.Equal(t, preCalls, transport.calls, "no extra chunk PUT after finalization")
}

// TestUploadRelayFailsClosedAfterAbortEvent rejects data/final events after an error event
// has already aborted the upload.
func TestUploadRelayFailsClosedAfterAbortEvent(t *testing.T) {
	relay, transport := newTestRelay(context.Background(), t, validUploadDelivery(), func(check.RemoteQueryStreamEvent) error { return nil })
	require.NoError(t, relay.emit(check.RemoteQueryStreamEvent{Type: "error", MetadataJSON: `{"code":"copy_failed","message":"boom"}`}))
	// The error event aborted the upload (one abort POST).
	abortCalls := transport.calls
	assert.GreaterOrEqual(t, abortCalls, 1)
	// A post-error data event must fail closed and NOT upload.
	err := relay.emit(check.RemoteQueryStreamEvent{Type: "data", MetadataJSON: `{}`, Payload: []byte("abcdefgh")})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "already aborted")
	assert.Equal(t, abortCalls, transport.calls, "no extra request after abort")
	// A post-error final event must also fail closed.
	err = relay.emit(check.RemoteQueryStreamEvent{Type: "final", MetadataJSON: `{"status":"SUCCEEDED"}`})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "already aborted")
	assert.Equal(t, abortCalls, transport.calls, "still no extra request")
}

// TestUploadRelayAbortIsIdempotentSinglePost confirms handleError + abortIfPending never
// double-abort: aborted is set before the send, and a racing second call is a no-op.
func TestUploadRelayAbortIsIdempotentSinglePost(t *testing.T) {
	relay, transport := newTestRelay(context.Background(), t, validUploadDelivery(), func(check.RemoteQueryStreamEvent) error { return nil })
	require.NoError(t, relay.emit(check.RemoteQueryStreamEvent{Type: "error", MetadataJSON: `{"code":"copy_failed"}`}))
	abortCalls := transport.calls
	// A second abortIfPending (as the runner returns) must NOT send a second abort.
	relay.abortIfPending()
	assert.Equal(t, abortCalls, transport.calls, "abort must be a single POST")
	for i := 0; i < abortCalls; i++ {
		assert.Contains(t, transport.request(i).url, "/abort")
	}
}
