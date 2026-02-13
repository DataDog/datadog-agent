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
	// The URL to send logs to (i.e. messages without snapshots).
	logsURL *url.URL
	// The URL to send snapshots to. They go to a different intake URL than logs
	// because they go through redaction on the backend.
	snapshotsURL *url.URL
	cfg          config

	mu            sync.Mutex
	uploaders     map[LogsUploaderMetadata]*refCountedUploader
	maxUploaderID uint64
}

// LogsUploaderMetadata is the metadata applied to the requests sent by an
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

// LogsUploader uploads logs and snapshots in batches, adding headers
// corresponding to a set of tags. It wraps logs batchers and provides a Close()
// method to remove itself from the parent LogsUploaderFactory.
type LogsUploader struct {
	logsBatcher      *batcher
	snapshotsBatcher *batcher
	// onClose is called when the uploader is closed to decrement its refcount
	// in the parent LogsUploaderFactory.
	onClose func()
}

// NewLogsUploaderFactory creates a new uploader factory.
func NewLogsUploaderFactory(logsURL *url.URL, snapshotsURL *url.URL, opts ...Option) *LogsUploaderFactory {
	lu := &LogsUploaderFactory{
		uploaders:    make(map[LogsUploaderMetadata]*refCountedUploader),
		cfg:          defaultConfig(),
		logsURL:      logsURL,
		snapshotsURL: snapshotsURL,
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
	for _, keyVal := range u.cfg.headers {
		addHeader(keyVal[0], keyVal[1])
	}

	logsURL := makeIntakeURL(u.logsURL, metadata.Tags)
	snapshotsURL := makeIntakeURL(u.snapshotsURL, metadata.Tags)

	if metadata.EntityID != "" {
		addHeader(ddHeaderEntityID, metadata.EntityID)
	}
	if metadata.ContainerID != "" {
		addHeader(ddHeaderContainerID, metadata.ContainerID)
	}
	name := fmt.Sprintf("logs:%d", uploaderID)
	log.Debugf("creating uploader %s with metadata %v", name, metadata)

	taggedUploader := &LogsUploader{
		logsBatcher:      newBatcher(name, newLogSender(u.cfg.client, logsURL, headers), u.cfg.batcherConfig),
		snapshotsBatcher: newBatcher(name, newLogSender(u.cfg.client, snapshotsURL, headers), u.cfg.batcherConfig),
		onClose: func() {
			u.closeUploader(metadata)
		},
	}

	u.uploaders[metadata] = &refCountedUploader{
		LogsUploader: taggedUploader,
		refCount:     1,
	}

	return taggedUploader
}

func makeIntakeURL(baseURL *url.URL, tags string) string {
	if tags == "" {
		return baseURL.String()
	}
	query, _ := url.ParseQuery(baseURL.RawQuery)
	// If we failed to parse the query, we'll use an empty query.
	query.Set("ddtags", tags)
	tagURL := *baseURL
	tagURL.RawQuery = query.Encode()
	return tagURL.String()
}

// EnqueueLog adds a message to the uploader's queue.
func (u *LogsUploader) EnqueueLog(data json.RawMessage) {
	u.logsBatcher.enqueue(data)
}

// EnqueueSnapshot adds a snapshot to the uploader's queue.
func (u *LogsUploader) EnqueueSnapshot(data json.RawMessage) {
	u.snapshotsBatcher.enqueue(data)
}

// Close decrements the reference count of the uploader. If the ref count reaches zero,
// the uploader is stopped and removed from the factory.
func (u *LogsUploader) Close() {
	u.onClose()
}

// closeUploader decrements the reference count of the uploader with the given
// metadata. If the ref count reaches zero, the uploader is stopped and removed
// from the factory.
func (u *LogsUploaderFactory) closeUploader(metadata LogsUploaderMetadata) {
	u.mu.Lock()
	defer u.mu.Unlock()

	rc, ok := u.uploaders[metadata]
	if !ok {
		log.Warnf(
			"closing a tagged uploader that is not in the factory: metadata=%v",
			metadata,
		)
		return
	}

	rc.refCount--
	if rc.refCount <= 0 {
		log.Debugf("stopping uploader with metadata %v", metadata)
		delete(u.uploaders, metadata)
		rc.LogsUploader.logsBatcher.stop()
		rc.LogsUploader.snapshotsBatcher.stop()
	}
}

// Stop gracefully stops all managed uploaders.
func (u *LogsUploaderFactory) Stop() {
	u.mu.Lock()
	defer u.mu.Unlock()

	for tags, rc := range u.uploaders {
		rc.LogsUploader.logsBatcher.stop()
		rc.LogsUploader.snapshotsBatcher.stop()
		delete(u.uploaders, tags)
	}
}

// Stats returns the combined metrics of all managed uploaders.
func (u *LogsUploaderFactory) Stats() map[string]int64 {
	u.mu.Lock()
	defer u.mu.Unlock()

	totalStats := make(map[string]int64)
	for _, rc := range u.uploaders {
		stats := rc.logsBatcher.state.metrics.Stats()
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
