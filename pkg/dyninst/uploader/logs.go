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
	"net/url"
	"sync"

	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// LogsUploader is a factory for creating and managing log uploaders for different tags.
type LogsUploader struct {
	mu        sync.Mutex
	uploaders map[string]*refCountedUploader
	cfg       config
}

type refCountedUploader struct {
	*TaggedLogsUploader
	refCount int
}

// TaggedLogsUploader is an uploader for sending log-like batches with a specific set of tags.
type TaggedLogsUploader struct {
	*batcher
	tags    string
	factory *LogsUploader
}

// NewLogsUploader creates a new uploader factory.
func NewLogsUploader(opts ...Option) *LogsUploader {
	lu := &LogsUploader{
		uploaders: make(map[string]*refCountedUploader),
		cfg:       defaultConfig(),
	}
	for _, opt := range opts {
		opt(&lu.cfg)
	}
	return lu
}

// GetUploader returns a reference-counted uploader for the given tags.
// The caller is responsible for calling Close() on the returned uploader.
func (u *LogsUploader) GetUploader(tags string) *TaggedLogsUploader {
	log.Tracef("getting uploader for tags: %s", tags)
	u.mu.Lock()
	defer u.mu.Unlock()

	if rc, ok := u.uploaders[tags]; ok {
		rc.refCount++
		return rc.TaggedLogsUploader
	}

	var logsURL, name string
	if tags == "" {
		logsURL = u.cfg.url.String()
		name = "logs"
	} else {
		query, _ := url.ParseQuery(u.cfg.url.RawQuery)
		// If we failed to parse the query, we'll use an empty query.
		query.Set("ddtags", tags)
		tagURL := *u.cfg.url
		tagURL.RawQuery = query.Encode()
		logsURL = tagURL.String()
		name = fmt.Sprintf("logs:%s", tags)
	}

	sender := newLogSender(u.cfg.client, logsURL)
	taggedUploader := &TaggedLogsUploader{
		batcher: newBatcher(name, sender, u.cfg.batcherConfig),
		tags:    tags,
		factory: u,
	}

	u.uploaders[tags] = &refCountedUploader{
		TaggedLogsUploader: taggedUploader,
		refCount:           1,
	}

	return taggedUploader
}

// Enqueue adds a message to the uploader's queue.
func (u *TaggedLogsUploader) Enqueue(data json.RawMessage) {
	u.enqueue(data)
}

// Close decrements the reference count of the uploader. If the ref count reaches zero,
// the uploader is stopped and removed from the factory.
func (u *TaggedLogsUploader) Close() {
	u.factory.mu.Lock()
	defer u.factory.mu.Unlock()

	rc, ok := u.factory.uploaders[u.tags]
	if !ok {
		log.Warnf("closing a tagged uploader that is not in the factory: tags=%q", u.tags)
		return
	}

	rc.refCount--
	if rc.refCount <= 0 {
		delete(u.factory.uploaders, u.tags)
		rc.TaggedLogsUploader.stop()
	}
}

// Stop gracefully stops all managed uploaders.
func (u *LogsUploader) Stop() {
	u.mu.Lock()
	defer u.mu.Unlock()

	for tags, rc := range u.uploaders {
		rc.TaggedLogsUploader.stop()
		delete(u.uploaders, tags)
	}
}

// Stats returns the combined metrics of all managed uploaders.
func (u *LogsUploader) Stats() map[string]int64 {
	u.mu.Lock()
	defer u.mu.Unlock()

	totalStats := make(map[string]int64)
	for _, rc := range u.uploaders {
		stats := rc.state.metrics.Stats()
		for k, v := range stats {
			totalStats[k] += v
		}
	}
	return totalStats
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
