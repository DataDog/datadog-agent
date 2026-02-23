// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf

package uploader

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/textproto"
	"time"
)

// DiagnosticMessage is the message sent to the DataDog backend conveying diagnostic information
type DiagnosticMessage struct {
	Service   string         `json:"service"`
	DDSource  debuggerSource `json:"ddsource"`
	Timestamp int64          `json:"timestamp"`

	Debugger debugger `json:"debugger"`
}

// debugger wraps the diagnostics message and provides JSON marshaling in the
// format expected by the backend.
type debugger struct {
	Diagnostic `json:"diagnostics"`
}

func (d debugger) MarshalJSON() ([]byte, error) {
	type alias struct {
		Diagnostic `json:"diagnostics"`
		Type       string `json:"type"`
	}
	return json.Marshal(alias{
		Diagnostic: d.Diagnostic,
		Type:       "diagnostic",
	})
}

type debuggerSource struct{}

var debuggerSourceJSON = []byte(`"dd_debugger"`)

func (d debuggerSource) MarshalJSON() ([]byte, error) {
	return debuggerSourceJSON, nil
}

func (d debuggerSource) UnmarshalJSON(b []byte) error {
	if !bytes.Equal(b, debuggerSourceJSON) {
		return fmt.Errorf("unexpected debugger source: %s", string(b))
	}
	return nil
}

// NewDiagnosticMessage creates a new DiagnosticMessage with the given service
// and diagnostic.
func NewDiagnosticMessage(service string, d Diagnostic) *DiagnosticMessage {
	return &DiagnosticMessage{
		Service: service,
		Debugger: debugger{
			Diagnostic: d,
		},
	}
}

// Status conveys the status of a probe
type Status string

const (
	StatusReceived  Status = "RECEIVED"  // StatusReceived means the probe configuration was received
	StatusInstalled Status = "INSTALLED" // StatusInstalled means the probe was installed
	StatusEmitting  Status = "EMITTING"  // StatusEmitting means the probe is emitting events
	StatusError     Status = "ERROR"     // StatusError means the probe has an issue
)

// Diagnostic contains fields relevant for conveying the status of a probe
type Diagnostic struct {
	RuntimeID    string `json:"runtimeId"`
	ProbeID      string `json:"probeId"`
	Status       Status `json:"status"`
	ProbeVersion int    `json:"probeVersion"`

	*DiagnosticException `json:"exception,omitempty"`
}

// DiagnosticException is used for diagnosing errors in probes
type DiagnosticException struct {
	Type    string `json:"type"`
	Message string `json:"message"`
}

// DiagnosticsUploader is an uploader for sending diagnostics batches.
type DiagnosticsUploader struct {
	*batcher
}

// NewDiagnosticsUploader creates a new uploader for sending diagnostics batches.
func NewDiagnosticsUploader(opts ...Option) *DiagnosticsUploader {
	cfg := defaultConfig()
	for _, opt := range opts {
		opt(&cfg)
	}
	sender := newDiagnosticsSender(cfg.client, cfg.url.String(), cfg.sendTimeout)
	return &DiagnosticsUploader{
		batcher: newBatcher("diagnostics", sender, cfg.batcherConfig, &Metrics{}),
	}
}

// Enqueue adds a message to the uploader's queue.
func (u *DiagnosticsUploader) Enqueue(diag *DiagnosticMessage) error {
	diag.Timestamp = time.Now().UnixMilli()
	data, err := json.Marshal(diag)
	if err != nil {
		return err
	}
	u.enqueue(data)
	return nil
}

// Stop gracefully stops the uploader.
func (u *DiagnosticsUploader) Stop() {
	u.stop()
}

// Stats returns the uploader's metrics.
func (u *DiagnosticsUploader) Stats() map[string]int64 {
	return u.state.metrics.Stats()
}

type diagnosticsSender struct {
	client      *http.Client
	url         string
	sendTimeout time.Duration
}

func newDiagnosticsSender(
	client *http.Client, url string, sendTimeout time.Duration,
) *diagnosticsSender {
	return &diagnosticsSender{
		client:      client,
		url:         url,
		sendTimeout: sendTimeout,
	}
}

func (s *diagnosticsSender) send(batch []json.RawMessage) error {
	var buf bytes.Buffer
	writer := multipart.NewWriter(&buf)

	header := make(textproto.MIMEHeader)
	header.Set("Content-Disposition", `form-data; name="event"; filename="event.json"`)
	header.Set("Content-Type", "application/json")
	fw, err := writer.CreatePart(header)
	if err != nil {
		return fmt.Errorf("failed to create form file: %w", err)
	}
	if err := encodeJSON(fw, batch); err != nil {
		return fmt.Errorf("failed to encode JSON: %w", err)
	}
	if err := writer.Close(); err != nil {
		return fmt.Errorf("failed to close multipart writer: %w", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), s.sendTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, s.url, &buf)
	if err != nil {
		return fmt.Errorf("failed to build request: %w", err)
	}
	req.Header.Set("Content-Type", writer.FormDataContentType())

	resp, err := s.client.Do(req)
	if err != nil {
		return fmt.Errorf("failed to send batch: %w", err)
	}
	return responseToError(resp)
}

func encodeJSON(w io.Writer, data []json.RawMessage) error {
	if _, err := w.Write([]byte("[")); err != nil {
		return err
	}
	for i, msg := range data {
		if i > 0 {
			if _, err := w.Write([]byte(",")); err != nil {
				return err
			}
		}
		if _, err := w.Write(msg); err != nil {
			return err
		}
	}
	if _, err := w.Write([]byte("]")); err != nil {
		return err
	}
	return nil
}
