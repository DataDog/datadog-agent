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

// LogsUploaderFactory is a factory for creating and managing log uploaders for different tags.
type LogsUploaderFactory struct {
	mu            sync.Mutex
	uploaders     map[LogsUploaderMetadata]*refCountedUploader
	maxUploaderID uint64
	cfg           config
}

// LogsUploaderMetadata is the metadata applied to the requests sent by this
// uploader.
type LogsUploaderMetadata struct {
	Tags        string
	EntityID    string
	ContainerID string
}

func (m LogsUploaderMetadata) String() string {
	return fmt.Sprintf(
		"{tags: %q, entityID: %q, containerID: %q}",
		m.Tags, m.EntityID, m.ContainerID,
	)
}

type refCountedUploader struct {
	*LogsUploader
	refCount int
}

// LogsUploader is an uploader for sending log-like batches with a specific set of tags.
type LogsUploader struct {
	*batcher
	metadata LogsUploaderMetadata
	factory  *LogsUploaderFactory
}

// NewLogsUploaderFactory creates a new uploader factory.
func NewLogsUploaderFactory(opts ...Option) *LogsUploaderFactory {
	lu := &LogsUploaderFactory{
		uploaders: make(map[LogsUploaderMetadata]*refCountedUploader),
		cfg:       defaultConfig(),
	}
	for _, opt := range opts {
		opt(&lu.cfg)
	}
	return lu
}

// See https://github.com/DataDog/dd-trace-java/blob/90a02cea/dd-java-agent/agent-debugger/src/main/java/com/datadog/debugger/uploader/BatchUploader.java#L78-L79
const (
	ddHeaderEntityID    = "Datadog-Entity-ID"
	ddHeaderContainerID = "Datadog-Container-ID"
)

// GetUploader returns a reference-counted uploader for the given tags and
// entity/container IDs.
//
// The caller is responsible for calling Close() on the returned uploader.
func (u *LogsUploaderFactory) GetUploader(metadata LogsUploaderMetadata) *LogsUploader {
	if log.ShouldLog(log.TraceLvl) {
		log.Tracef("getting uploader for tags: %s", metadata)
	}
	u.mu.Lock()
	defer u.mu.Unlock()

	if rc, ok := u.uploaders[metadata]; ok {
		rc.refCount++
		return rc.LogsUploader
	}

	u.maxUploaderID++
	uploaderID := u.maxUploaderID

	var headers map[string]string
	addHeader := func(key, value string) {
		if value == "" {
			return
		}
		if headers == nil {
			headers = make(map[string]string)
		}
		headers[key] = value
	}
	var logsURL, name string
	if metadata.Tags == "" {
		logsURL = u.cfg.url.String()
	} else {
		query, _ := url.ParseQuery(u.cfg.url.RawQuery)
		// If we failed to parse the query, we'll use an empty query.
		query.Set("ddtags", metadata.Tags)
		tagURL := *u.cfg.url
		tagURL.RawQuery = query.Encode()
		logsURL = tagURL.String()
		addHeader(ddHeaderEntityID, metadata.EntityID)
		addHeader(ddHeaderContainerID, metadata.ContainerID)
	}
	name = fmt.Sprintf("logs:%d", uploaderID)
	log.Debugf("creating uploader %s with metadata %v", name, metadata)

	sender := newLogSender(u.cfg.client, logsURL, headers)
	taggedUploader := &LogsUploader{
		batcher:  newBatcher(name, sender, u.cfg.batcherConfig),
		metadata: metadata,
		factory:  u,
	}

	u.uploaders[metadata] = &refCountedUploader{
		LogsUploader: taggedUploader,
		refCount:     1,
	}

	return taggedUploader
}

// Enqueue adds a message to the uploader's queue.
func (u *LogsUploader) Enqueue(data json.RawMessage) {
	u.enqueue(data)
}

// Close decrements the reference count of the uploader. If the ref count reaches zero,
// the uploader is stopped and removed from the factory.
func (u *LogsUploader) Close() {
	u.factory.mu.Lock()
	defer u.factory.mu.Unlock()

	rc, ok := u.factory.uploaders[u.metadata]
	if !ok {
		log.Warnf(
			"closing a tagged uploader (%s) that is not in the factory: metadata=%v",
			u.name, u.metadata,
		)
		return
	}

	rc.refCount--
	if rc.refCount <= 0 {
		log.Debugf("stopping uploader %s with metadata %v", u.name, u.metadata)
		delete(u.factory.uploaders, u.metadata)
		rc.LogsUploader.stop()
	}
}

// Stop gracefully stops all managed uploaders.
func (u *LogsUploaderFactory) Stop() {
	u.mu.Lock()
	defer u.mu.Unlock()

	for tags, rc := range u.uploaders {
		rc.LogsUploader.stop()
		delete(u.uploaders, tags)
	}
}

// Stats returns the combined metrics of all managed uploaders.
func (u *LogsUploaderFactory) Stats() map[string]int64 {
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
	client  *http.Client
	url     string
	headers map[string]string
}

func newLogSender(client *http.Client, url string, headers map[string]string) *logSender {
	return &logSender{
		client:  client,
		url:     url,
		headers: headers,
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
	for k, v := range s.headers {
		req.Header.Set(k, v)
	}

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
