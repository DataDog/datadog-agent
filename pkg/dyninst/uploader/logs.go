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
	"net/http"
)

// LogsUploader is an uploader for sending log-like batches.
type LogsUploader struct {
	*batcher
}

// NewLogsUploader creates a new uploader for sending log-like batches.
func NewLogsUploader(opts ...Option) *LogsUploader {
	cfg := defaultConfig()
	for _, opt := range opts {
		opt(cfg)
	}
	sender := newLogSender(cfg.client, cfg.url)
	return &LogsUploader{
		batcher: newBatcher("logs", sender, opts...),
	}
}

// Enqueue adds a message to the uploader's queue.
func (u *LogsUploader) Enqueue(data json.RawMessage) {
	u.enqueue(data)
}

// Stop gracefully stops the uploader.
func (u *LogsUploader) Stop() {
	u.stop()
}

// Stats returns the uploader's metrics.
func (u *LogsUploader) Stats() map[string]int64 {
	return u.state.metrics.Stats()
}

type logSender struct {
	client *http.Client
	url    string
}

func newLogSender(client *http.Client, url string) *logSender {
	return &logSender{
		client: client,
		url:    url,
	}
}

func (s *logSender) send(batch []json.RawMessage) error {
	var buf bytes.Buffer
	if err := encodeJSON(&buf, batch); err != nil {
		return fmt.Errorf("failed to encode JSON: %w", err)
	}

	req, err := http.NewRequestWithContext(context.Background(), http.MethodPost, s.url, &buf)
	if err != nil {
		return fmt.Errorf("failed to create http request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := s.client.Do(req)
	if err != nil {
		return fmt.Errorf("failed to send batch: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		return fmt.Errorf("uploader received error response: status=%d", resp.StatusCode)
	}

	return nil
}
