// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf

package uploader

import (
	"encoding/json"
	"io"
	"mime"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type testServer struct {
	requests  <-chan receivedRequest
	server    *httptest.Server
	serverURL *url.URL
	close     chan struct{}
}

func (s *testServer) Close() {
	close(s.close)
	s.server.Close()
}

type receivedRequest struct {
	w    http.ResponseWriter
	r    *http.Request
	done chan struct{}
}

func newTestServer() *testServer {
	requestsC := make(chan receivedRequest)
	closeC := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		doneC := make(chan struct{})
		select {
		case requestsC <- receivedRequest{w: w, r: r, done: doneC}:
		case <-closeC:
		case <-r.Context().Done():
			return
		}
		select {
		case <-doneC:
		case <-closeC:
		case <-r.Context().Done():
			return
		}
	}))
	serverURL, _ := url.Parse(server.URL)
	ts := &testServer{
		server:    server,
		serverURL: serverURL,
		requests:  requestsC,
		close:     closeC,
	}
	return ts
}

func validateDiagnosticsRequest(
	t *testing.T, expectedMessages []*DiagnosticMessage, req *http.Request,
) {
	contentType, params, err := mime.ParseMediaType(req.Header.Get("Content-Type"))
	require.NoError(t, err)
	require.Equal(t, "multipart/form-data", contentType)

	reader := multipart.NewReader(req.Body, params["boundary"])
	part, err := reader.NextPart()
	require.NoError(t, err)
	require.Equal(t, "event", part.FormName())
	require.Equal(t, "event.json", part.FileName())

	data, err := io.ReadAll(part)
	require.NoError(t, err)

	var batch []*DiagnosticMessage
	require.NoError(t, json.Unmarshal(data, &batch))
	require.Len(t, batch, len(expectedMessages))
	require.EqualValues(t, expectedMessages, batch)
}

func validateLogsRequest(t *testing.T, expectedMessages []json.RawMessage, req *http.Request) {
	contentType := req.Header.Get("Content-Type")
	require.Equal(t, "application/json", contentType)

	data, err := io.ReadAll(req.Body)
	require.NoError(t, err)

	var batch []json.RawMessage
	require.NoError(t, json.Unmarshal(data, &batch))
	require.Len(t, batch, len(expectedMessages))

	for i, msg := range expectedMessages {
		assert.Equal(t, string(msg), string(batch[i]))
	}
}

func TestDiagnosticsUploader(t *testing.T) {
	computeExpectedBytes := func(messages []*DiagnosticMessage) int {
		var expectedBytes int
		for _, msg := range messages {
			msg := *msg
			msg.Timestamp = time.Now().UnixMilli()
			msgBytes, err := json.Marshal(&msg)
			require.NoError(t, err)
			expectedBytes += len(msgBytes)
		}
		return expectedBytes
	}

	t.Run("success", func(t *testing.T) {
		ts := newTestServer()
		defer ts.Close()

		uploader := NewDiagnosticsUploader(
			WithURL(ts.serverURL),
			WithMaxBatchItems(2),
			WithMaxBufferDuration(0), // disable timers
		)
		defer uploader.Stop()

		expectedMessages := []*DiagnosticMessage{
			NewDiagnosticMessage("service1", Diagnostic{
				ProbeID: "probe1",
				Status:  StatusReceived,
			}),
			NewDiagnosticMessage("service2", Diagnostic{
				ProbeID: "probe2",
				Status:  StatusInstalled,
			}),
		}

		expectedBytes := computeExpectedBytes(expectedMessages)
		for _, msg := range expectedMessages {
			require.NoError(t, uploader.Enqueue(msg))
		}

		req := <-ts.requests
		validateDiagnosticsRequest(t, expectedMessages, req.r)
		req.w.WriteHeader(http.StatusOK)
		close(req.done)

		require.EventuallyWithT(t, func(c *assert.CollectT) {
			assert.Equal(c, map[string]int64{
				"batches_sent": 1,
				"bytes_sent":   int64(expectedBytes),
				"items_sent":   int64(len(expectedMessages)),
				"errors":       0,
			}, uploader.Stats())
		}, 1*time.Second, 10*time.Millisecond)
	})

	t.Run("success with timer", func(t *testing.T) {
		ts := newTestServer()
		defer ts.Close()

		uploader := NewDiagnosticsUploader(
			WithURL(ts.serverURL),
			WithMaxBatchItems(2),
			WithMaxBufferDuration(10*time.Millisecond),
		)
		defer uploader.Stop()

		expectedMessages := []*DiagnosticMessage{
			NewDiagnosticMessage("service1", Diagnostic{
				ProbeID: "probe1",
				Status:  StatusReceived,
				DiagnosticException: &DiagnosticException{
					Message: "test",
				},
			}),
		}

		expectedBytes := computeExpectedBytes(expectedMessages)
		for _, msg := range expectedMessages {
			require.NoError(t, uploader.Enqueue(msg))
		}

		req := <-ts.requests
		validateDiagnosticsRequest(t, expectedMessages, req.r)
		req.w.WriteHeader(http.StatusOK)
		close(req.done)

		require.EventuallyWithT(t, func(c *assert.CollectT) {
			assert.Equal(c, map[string]int64{
				"batches_sent": 1,
				"bytes_sent":   int64(expectedBytes),
				"items_sent":   int64(len(expectedMessages)),
				"errors":       0,
			}, uploader.Stats())
		}, 1*time.Second, 10*time.Millisecond)
	})

	t.Run("failure", func(t *testing.T) {
		ts := newTestServer()
		defer ts.Close()

		uploader := NewDiagnosticsUploader(
			WithURL(ts.serverURL),
			WithMaxBatchItems(1),
		)
		defer uploader.Stop()

		msg1 := NewDiagnosticMessage(
			"service1", Diagnostic{ProbeID: "probe1", Status: StatusInstalled},
		)
		require.NoError(t, uploader.Enqueue(msg1))

		req := <-ts.requests
		validateDiagnosticsRequest(t, []*DiagnosticMessage{msg1}, req.r)
		req.w.WriteHeader(http.StatusInternalServerError)
		close(req.done)

		require.EventuallyWithT(t, func(c *assert.CollectT) {
			assert.Equal(c, map[string]int64{
				"batches_sent": 0,
				"bytes_sent":   0,
				"items_sent":   0,
				"errors":       1,
			}, uploader.Stats())
		}, 1*time.Second, 10*time.Millisecond)
	})
}

func TestLogsUploader(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		ts := newTestServer()
		defer ts.Close()

		uploaderFactory := NewLogsUploaderFactory(
			WithURL(ts.serverURL),
			WithMaxBatchItems(2),
		)
		defer uploaderFactory.Stop()
		uploader := uploaderFactory.GetUploader(LogsUploaderMetadata{
			Tags: "service:test",
		})

		msg1 := json.RawMessage(`{"key":"value1"}`)
		msg2 := json.RawMessage(`{"key":"value2"}`)

		uploader.Enqueue(msg1)
		uploader.Enqueue(msg2)

		// receive and validate request
		req := <-ts.requests
		validateLogsRequest(t, []json.RawMessage{msg1, msg2}, req.r)

		// send response and unblock handler
		req.w.WriteHeader(http.StatusOK)
		close(req.done)

		require.EventuallyWithT(t, func(c *assert.CollectT) {
			assert.Equal(c, map[string]int64{
				"batches_sent": 1,
				"bytes_sent":   int64(len(msg1) + len(msg2)),
				"items_sent":   2,
				"errors":       0,
			}, uploaderFactory.Stats())
		}, 1*time.Second, 10*time.Millisecond)
	})

	t.Run("failure", func(t *testing.T) {
		ts := newTestServer()
		defer ts.Close()

		uploaderFactory := NewLogsUploaderFactory(
			WithURL(ts.serverURL),
			WithMaxBatchItems(1),
		)
		defer uploaderFactory.Stop()
		uploader := uploaderFactory.GetUploader(LogsUploaderMetadata{
			Tags: "service:test",
		})

		msg1 := json.RawMessage(`{"key":"value1"}`)
		uploader.Enqueue(msg1)

		// receive request
		req := <-ts.requests
		validateLogsRequest(t, []json.RawMessage{msg1}, req.r)

		// send failure response
		req.w.WriteHeader(http.StatusInternalServerError)
		close(req.done)

		require.EventuallyWithT(t, func(c *assert.CollectT) {
			assert.Equal(c, map[string]int64{
				"batches_sent": 0,
				"bytes_sent":   0,
				"items_sent":   0,
				"errors":       1,
			}, uploaderFactory.Stats())
		}, 1*time.Second, 10*time.Millisecond)
	})
}
