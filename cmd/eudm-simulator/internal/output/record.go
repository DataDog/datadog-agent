// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

// Package output connects sanitized telemetry to Agent delivery packages.
package output

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"sync"
)

const recorderMaxBytes = 128 << 20

// This includes the reference, spare slice capacity, and header map. Header
// entries and their strings are charged separately. The derived count bound
// permits an entire process group, whose request IDs have 14-bit chunk indices.
const recorderReferenceOverhead = 512

var recordedHeaders = [...]string{"Content-Type", "Content-Encoding", "DD-Agent-Payload", "X-Dd-Hostname", "X-Dd-Processagentversion", "X-Dd-Request-Id", "X-DD-Agent-Timestamp", "X-DD-Agent-Start-Time", "X-DD-Payload-Source", "X-DD-Processes-Enabled", "X-DD-Service-Discovery-Enabled"}

// RecordedRequest holds an outgoing request for offline replay tests.
// The recorder excludes authorization and other sensitive headers.
type RecordedRequest struct {
	Path    string
	Headers http.Header
	Body    []byte
}

// Recorder is an HTTP transport with no network capability. It must only be
// installed behind a pipeline receiving already sanitized payloads. It never
// retains credentials, URLs with query strings, or the
// original request. Replay tests use ordinary Agent serializers and forwarders.
type Recorder struct {
	mu       sync.Mutex
	requests []RecordedRequest
	bytes    int
	changed  chan struct{}
}

func NewRecorder() *Recorder { return &Recorder{changed: make(chan struct{})} }

func (r *Recorder) RoundTrip(req *http.Request) (*http.Response, error) {
	data, err := io.ReadAll(io.LimitReader(req.Body, 64<<20))
	if err != nil {
		return nil, errors.New("read sanitized recording request")
	}
	if len(data) >= 64<<20 {
		return nil, errors.New("serialized request exceeds recording size limit")
	}
	size := recorderReferenceOverhead + cap(data) + len(req.URL.Path)
	for _, key := range recordedHeaders {
		if value := req.Header.Get(key); value != "" {
			size += 256 + len(key) + len(value)
		}
	}
	r.mu.Lock()
	if len(r.requests) >= recorderMaxBytes/recorderReferenceOverhead || size > recorderMaxBytes-r.bytes {
		r.mu.Unlock()
		return nil, errors.New("sanitized recording cycle exceeds memory limit")
	}
	headers := http.Header{}
	for _, key := range recordedHeaders {
		if value := req.Header.Get(key); value != "" {
			headers.Set(key, strings.Clone(value))
		}
	}
	r.requests = append(r.requests, RecordedRequest{Path: strings.Clone(req.URL.Path), Headers: headers, Body: data})
	r.bytes += size
	close(r.changed)
	r.changed = make(chan struct{})
	r.mu.Unlock()
	return &http.Response{StatusCode: http.StatusOK, Header: make(http.Header), Body: io.NopCloser(bytes.NewBufferString("{}")), Request: req}, nil
}

// Drain transfers owned references after Pipeline.Wait has acknowledged all
// sends for the current logical cycle. No prior cycle remains retained.
func (r *Recorder) Drain() []RecordedRequest {
	r.mu.Lock()
	defer r.mu.Unlock()
	result := r.requests
	r.requests, r.bytes = nil, 0
	return result
}

// Wait returns recorded references after at least count requests have arrived.
// Final transaction acceptance is tracked independently by the delivery layer.
func (r *Recorder) Wait(ctx context.Context, count int) ([]RecordedRequest, error) {
	for {
		r.mu.Lock()
		if len(r.requests) >= count {
			result := make([]RecordedRequest, len(r.requests))
			for i, v := range r.requests {
				result[i] = RecordedRequest{Path: v.Path, Headers: v.Headers.Clone(), Body: bytes.Clone(v.Body)}
			}
			r.mu.Unlock()
			return result, nil
		}
		changed := r.changed
		r.mu.Unlock()
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-changed:
		}
	}
}
