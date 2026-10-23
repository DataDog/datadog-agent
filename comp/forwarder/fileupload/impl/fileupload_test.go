// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package fileuploadimpl

import (
	"bytes"
	"context"
	"errors"
	"io"
	"mime"
	"mime/multipart"
	"net"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	fileupload "github.com/DataDog/datadog-agent/comp/forwarder/fileupload/def"
	configmock "github.com/DataDog/datadog-agent/pkg/config/mock"
	"github.com/DataDog/datadog-agent/pkg/config/model"
	"github.com/DataDog/datadog-agent/pkg/zstd"
)

const testPrefix = "network_pcap.forwarder."

// receivedPart is one decoded multipart part.
type receivedPart struct {
	formName    string
	filename    string
	disposition string
	contentType string
	content     []byte
}

// receivedRequest is one request decoded by an intake.
type receivedRequest struct {
	path            string
	apiKey          string
	contentEncoding string
	parts           []receivedPart
	decodeErr       error
}

// intake is a test intake that answers each request with the next status in
// statuses (the last one repeating) and records what it received.
type intake struct {
	srv      *httptest.Server
	mu       sync.Mutex
	requests []receivedRequest
	calls    atomic.Int32
}

func newIntake(t *testing.T, statuses ...int) *intake {
	t.Helper()
	if len(statuses) == 0 {
		statuses = []int{http.StatusAccepted}
	}
	in := &intake{}
	in.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		i := int(in.calls.Add(1)) - 1
		req := decodeRequest(r)
		in.mu.Lock()
		in.requests = append(in.requests, req)
		in.mu.Unlock()
		w.WriteHeader(statuses[min(i, len(statuses)-1)])
	}))
	t.Cleanup(in.srv.Close)
	return in
}

func (in *intake) received() []receivedRequest {
	in.mu.Lock()
	defer in.mu.Unlock()
	return append([]receivedRequest(nil), in.requests...)
}

func decodeRequest(r *http.Request) receivedRequest {
	req := receivedRequest{
		path:            r.URL.Path,
		apiKey:          r.Header.Get("DD-API-KEY"),
		contentEncoding: r.Header.Get("Content-Encoding"),
	}
	_, params, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if err != nil {
		req.decodeErr = err
		return req
	}
	zr, err := zstd.NewReader(r.Body)
	if err != nil {
		req.decodeErr = err
		return req
	}
	defer zr.Close()
	mr := multipart.NewReader(zr, params["boundary"])
	for {
		p, err := mr.NextPart()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			req.decodeErr = err
			return req
		}
		content, err := io.ReadAll(p)
		if err != nil {
			req.decodeErr = err
			return req
		}
		req.parts = append(req.parts, receivedPart{
			formName:    p.FormName(),
			filename:    p.FileName(),
			disposition: p.Header.Get("Content-Disposition"),
			contentType: p.Header.Get("Content-Type"),
			content:     content,
		})
	}
	return req
}

func newTestConfig(t *testing.T, in *intake) model.ReaderWriter {
	t.Helper()
	cfg := configmock.New(t)
	cfg.SetInTest("api_key", "main-api-key")
	cfg.SetInTest(testPrefix+"logs_dd_url", in.srv.URL)
	return cfg
}

func newTestUploader(cfg model.Reader) *uploader {
	u := newUploader(cfg, getTracks())
	u.retryBackoff = time.Millisecond
	return u
}

// countingSource returns an attachment that counts how often it is opened.
func countingSource(content []byte) (fileupload.Attachment, *atomic.Int32) {
	var opens atomic.Int32
	return fileupload.Attachment{
		Name:        "pcap",
		Filename:    "capture.pcap",
		ContentType: "application/octet-stream",
		Open: func() (io.ReadCloser, error) {
			opens.Add(1)
			return io.NopCloser(bytes.NewReader(content)), nil
		},
	}, &opens
}

func pcapRequest(content []byte) (fileupload.UploadRequest, *atomic.Int32) {
	attachment, opens := countingSource(content)
	return fileupload.UploadRequest{
		Track:       fileupload.TrackNetworkPcap,
		Event:       []byte(`{"capture_id":"cap-1"}`),
		Attachments: []fileupload.Attachment{attachment},
	}, opens
}

func TestUploadSendsZstdMultipart(t *testing.T) {
	in := newIntake(t)
	u := newTestUploader(newTestConfig(t, in))
	content := bytes.Repeat([]byte("pcap"), 64<<10)
	req, _ := pcapRequest(content)

	require.NoError(t, u.Upload(context.Background(), req))

	got := in.received()
	require.Len(t, got, 1)
	r := got[0]
	require.NoError(t, r.decodeErr)
	assert.Equal(t, "/api/v2/networkpcap", r.path)
	assert.Equal(t, "zstd", r.contentEncoding)
	assert.Equal(t, "main-api-key", r.apiKey)

	require.Len(t, r.parts, 2)
	event := r.parts[0]
	assert.Equal(t, "event", event.formName)
	// The intake only accepts the event part with a filename attribute.
	assert.Contains(t, event.disposition, `filename=""`)
	assert.Equal(t, "application/json", event.contentType)
	assert.JSONEq(t, `{"capture_id":"cap-1"}`, string(event.content))

	attachment := r.parts[1]
	assert.Equal(t, "pcap", attachment.formName)
	assert.Equal(t, "capture.pcap", attachment.filename)
	assert.Equal(t, "application/octet-stream", attachment.contentType)
	assert.Equal(t, content, attachment.content)
}

func TestUploadDefaultsAttachmentContentType(t *testing.T) {
	in := newIntake(t)
	u := newTestUploader(newTestConfig(t, in))
	req, _ := pcapRequest([]byte("x"))
	req.Attachments[0].ContentType = ""

	require.NoError(t, u.Upload(context.Background(), req))
	got := in.received()
	require.Len(t, got, 1)
	require.Len(t, got[0].parts, 2)
	assert.Equal(t, "application/octet-stream", got[0].parts[1].contentType)
}

// Each attempt opens the source again, so a retry resends the whole content.
func TestUploadRetryReopensTheSource(t *testing.T) {
	in := newIntake(t, http.StatusServiceUnavailable, http.StatusAccepted)
	u := newTestUploader(newTestConfig(t, in))
	content := bytes.Repeat([]byte{0xab}, 128<<10)
	req, opens := pcapRequest(content)

	require.NoError(t, u.Upload(context.Background(), req))

	assert.EqualValues(t, 2, opens.Load())
	got := in.received()
	require.Len(t, got, 2)
	for _, r := range got {
		require.NoError(t, r.decodeErr)
		require.Len(t, r.parts, 2)
		assert.Equal(t, content, r.parts[1].content)
	}
}

func TestUploadRetriesRetryableStatuses(t *testing.T) {
	for _, status := range []int{
		http.StatusRequestTimeout,
		http.StatusTooManyRequests,
		http.StatusInternalServerError,
		http.StatusBadGateway,
		http.StatusServiceUnavailable,
		http.StatusGatewayTimeout,
	} {
		t.Run(http.StatusText(status), func(t *testing.T) {
			in := newIntake(t, status, http.StatusAccepted)
			u := newTestUploader(newTestConfig(t, in))
			req, _ := pcapRequest([]byte("pcap"))
			require.NoError(t, u.Upload(context.Background(), req))
			assert.EqualValues(t, 2, in.calls.Load())
		})
	}
}

func TestUploadDoesNotRetryTooLarge(t *testing.T) {
	in := newIntake(t, http.StatusRequestEntityTooLarge)
	u := newTestUploader(newTestConfig(t, in))
	req, _ := pcapRequest([]byte("pcap"))

	err := u.Upload(context.Background(), req)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "too large (413)")
	assert.EqualValues(t, 1, in.calls.Load())
}

func TestUploadDoesNotRetryClientErrors(t *testing.T) {
	in := newIntake(t, http.StatusBadRequest)
	u := newTestUploader(newTestConfig(t, in))
	req, _ := pcapRequest([]byte("pcap"))

	require.Error(t, u.Upload(context.Background(), req))
	assert.EqualValues(t, 1, in.calls.Load())
}

func TestUploadGivesUpAfterMaxAttempts(t *testing.T) {
	in := newIntake(t, http.StatusServiceUnavailable)
	u := newTestUploader(newTestConfig(t, in))
	req, opens := pcapRequest([]byte("pcap"))

	err := u.Upload(context.Background(), req)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "after 4 attempts")
	assert.EqualValues(t, defaultMaxAttempts, in.calls.Load())
	assert.EqualValues(t, defaultMaxAttempts, opens.Load())
}

func TestUploadStopsRetryingWhenContextIsDone(t *testing.T) {
	in := newIntake(t, http.StatusServiceUnavailable)
	u := newTestUploader(newTestConfig(t, in))
	u.retryBackoff = time.Hour
	req, _ := pcapRequest([]byte("pcap"))

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	err := u.Upload(ctx, req)
	require.ErrorIs(t, err, context.DeadlineExceeded)
	assert.EqualValues(t, 1, in.calls.Load())
}

// An unreadable attachment fails the upload before anything is sent.
func TestUploadOpenFailureSendsNothing(t *testing.T) {
	in := newIntake(t)
	u := newTestUploader(newTestConfig(t, in))
	req := fileupload.UploadRequest{
		Track: fileupload.TrackNetworkPcap,
		Event: []byte(`{}`),
		Attachments: []fileupload.Attachment{{
			Name: "pcap",
			Open: func() (io.ReadCloser, error) { return nil, errors.New("no such file") },
		}},
	}

	err := u.Upload(context.Background(), req)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no such file")
	assert.EqualValues(t, 0, in.calls.Load())
}

func TestUploadUnknownTrack(t *testing.T) {
	in := newIntake(t)
	u := newTestUploader(newTestConfig(t, in))

	err := u.Upload(context.Background(), fileupload.UploadRequest{Track: "nope"})
	require.Error(t, err)
	assert.EqualValues(t, 0, in.calls.Load())
}

func TestUploadWithoutAPIKeyFails(t *testing.T) {
	in := newIntake(t)
	cfg := newTestConfig(t, in)
	cfg.SetInTest("api_key", "")
	u := newTestUploader(cfg)
	req, _ := pcapRequest([]byte("pcap"))

	require.Error(t, u.Upload(context.Background(), req))
	assert.EqualValues(t, 0, in.calls.Load())
}

// Additional endpoints each receive the upload with their own API key. A
// reliable endpoint's failure fails the upload; an unreliable one's does not.
func TestUploadAdditionalEndpoints(t *testing.T) {
	for _, tc := range []struct {
		name       string
		reliable   bool
		wantFailed bool
	}{
		{name: "reliable", reliable: true, wantFailed: true},
		{name: "unreliable", reliable: false, wantFailed: false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			main := newIntake(t)
			healthy := newIntake(t)
			failing := newIntake(t, http.StatusBadRequest)
			cfg := newTestConfig(t, main)
			cfg.SetInTest(testPrefix+"additional_endpoints", []map[string]interface{}{
				{"api_key": "healthy-api-key", "host": hostOf(t, healthy), "port": portOf(t, healthy), "use_ssl": false},
				{"api_key": "failing-api-key", "host": hostOf(t, failing), "port": portOf(t, failing), "use_ssl": false, "is_reliable": tc.reliable},
			})
			u := newTestUploader(cfg)
			req, _ := pcapRequest([]byte("pcap"))

			err := u.Upload(context.Background(), req)
			if tc.wantFailed {
				require.Error(t, err)
				assert.Contains(t, err.Error(), failing.srv.URL)
			} else {
				require.NoError(t, err)
			}

			for in, wantKey := range map[*intake]string{
				main:    "main-api-key",
				healthy: "healthy-api-key",
				failing: "failing-api-key",
			} {
				got := in.received()
				require.Len(t, got, 1)
				assert.Equal(t, wantKey, got[0].apiKey)
				assert.Equal(t, "/api/v2/networkpcap", got[0].path)
			}
		})
	}
}

// The MRF endpoint only receives uploads while a failover is active.
func TestUploadMRFEndpointFollowsFailover(t *testing.T) {
	main := newIntake(t)
	mrf := newIntake(t)
	cfg := newTestConfig(t, main)
	cfg.SetInTest("multi_region_failover.enabled", true)
	cfg.SetInTest("multi_region_failover.dd_url", mrf.srv.URL)
	cfg.SetInTest("multi_region_failover.api_key", "mrf-api-key")
	u := newTestUploader(cfg)
	req, _ := pcapRequest([]byte("pcap"))

	require.NoError(t, u.Upload(context.Background(), req))
	assert.EqualValues(t, 1, main.calls.Load())
	assert.EqualValues(t, 0, mrf.calls.Load())

	cfg.SetInTest("multi_region_failover.failover_logs", true)
	require.NoError(t, u.Upload(context.Background(), req))
	assert.EqualValues(t, 2, main.calls.Load())
	got := mrf.received()
	require.Len(t, got, 1)
	assert.Equal(t, "mrf-api-key", got[0].apiKey)
}

func TestDefaultEndpointUsesTrackHostPrefix(t *testing.T) {
	cfg := configmock.New(t)
	cfg.SetInTest("api_key", "main-api-key")
	cfg.SetInTest("site", "datadoghq.eu")
	u := newTestUploader(cfg)

	endpoints, err := u.getEndpoints(fileupload.TrackNetworkPcap)
	require.NoError(t, err)
	require.Len(t, endpoints.Endpoints, 1)
	// The default host is fully qualified (trailing dot), as for every other
	// event platform sender, so it skips the DNS search list.
	assert.Equal(t, "https://cnm-intake.datadoghq.eu./api/v2/networkpcap", buildURL(&endpoints.Endpoints[0], fileupload.TrackNetworkPcap))
}

func TestEndpointOverrideKeepsPortAndPath(t *testing.T) {
	cfg := configmock.New(t)
	cfg.SetInTest("api_key", "main-api-key")
	cfg.SetInTest(testPrefix+"logs_dd_url", "intake.example.com:8443")
	u := newTestUploader(cfg)

	endpoints, err := u.getEndpoints(fileupload.TrackNetworkPcap)
	require.NoError(t, err)
	assert.Equal(t, "https://intake.example.com:8443/api/v2/networkpcap", buildURL(&endpoints.Main, fileupload.TrackNetworkPcap))
}

func hostOf(t *testing.T, in *intake) string {
	t.Helper()
	return in.srv.Listener.Addr().(*net.TCPAddr).IP.String()
}

func portOf(t *testing.T, in *intake) int {
	t.Helper()
	return in.srv.Listener.Addr().(*net.TCPAddr).Port
}
