// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package recorderimpl

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"

	observer "github.com/DataDog/datadog-agent/comp/observer/def"
)

const lifecycleFileName = "observer-lifecycle.jsonl"

// lifecycleRecord is the JSON structure written as one line per event.
type lifecycleRecord struct {
	Timestamp     int64  `json:"ts"`
	EventType     string `json:"type"`
	ContainerID   string `json:"container_id"`
	ExitCode      *int32 `json:"exit_code,omitempty"`
	ContainerName string `json:"container_name"`
	Image         string `json:"image"`
	Runtime       string `json:"runtime"`
}

// lifecycleJSONLWriter appends container lifecycle events as JSON Lines.
// It is safe for concurrent use by multiple goroutines.
type lifecycleJSONLWriter struct {
	mu   sync.Mutex
	file *os.File
}

// newLifecycleJSONLWriter opens (or creates) the lifecycle JSONL file in outputDir.
// The file is opened in append mode so existing data is preserved across restarts.
func newLifecycleJSONLWriter(outputDir string) (*lifecycleJSONLWriter, error) {
	if err := os.MkdirAll(outputDir, 0755); err != nil {
		return nil, fmt.Errorf("creating output directory: %w", err)
	}

	path := filepath.Join(outputDir, lifecycleFileName)
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return nil, fmt.Errorf("opening lifecycle file: %w", err)
	}

	return &lifecycleJSONLWriter{file: f}, nil
}

// WriteEvent serializes a LifecycleView as a single JSON line and appends it to
// the file. For non-delete events the exit_code field is omitted.
func (w *lifecycleJSONLWriter) WriteEvent(event observer.LifecycleView) error {
	rec := lifecycleRecord{
		Timestamp:     event.GetTimestampUnix(),
		EventType:     event.GetEventType(),
		ContainerID:   event.GetContainerID(),
		ExitCode:      event.GetExitCode(),
		ContainerName: event.GetContainerName(),
		Image:         event.GetImage(),
		Runtime:       event.GetRuntime(),
	}

	data, err := json.Marshal(rec)
	if err != nil {
		return fmt.Errorf("marshaling lifecycle event: %w", err)
	}
	data = append(data, '\n')

	w.mu.Lock()
	defer w.mu.Unlock()

	if _, err := w.file.Write(data); err != nil {
		return fmt.Errorf("writing lifecycle event: %w", err)
	}
	return nil
}

// Close closes the underlying file.
func (w *lifecycleJSONLWriter) Close() error {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.file.Close()
}
