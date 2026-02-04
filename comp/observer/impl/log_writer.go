// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package observerimpl

import (
	"encoding/json"
	"fmt"
	"os"
	"sync"
)

// LogEntry represents a single log entry in JSON format.
type LogEntry struct {
	Timestamp int64    `json:"timestamp"`
	Content   string   `json:"content"`
	Tags      []string `json:"tags,omitempty"`
	Source    string   `json:"source,omitempty"`
	Level     string   `json:"level,omitempty"`
}

// LogWriter writes logs to a JSON lines file.
type LogWriter struct {
	file *os.File
	enc  *json.Encoder
	mu   sync.Mutex
}

// NewLogWriter creates a new log writer that writes to the specified file.
func NewLogWriter(path string) (*LogWriter, error) {
	file, err := os.Create(path)
	if err != nil {
		return nil, fmt.Errorf("creating log file: %w", err)
	}

	return &LogWriter{
		file: file,
		enc:  json.NewEncoder(file),
	}, nil
}

// WriteLog writes a single log entry.
func (w *LogWriter) WriteLog(timestamp int64, content string, tags []string, source, level string) error {
	w.mu.Lock()
	defer w.mu.Unlock()

	entry := LogEntry{
		Timestamp: timestamp,
		Content:   content,
		Tags:      tags,
		Source:    source,
		Level:     level,
	}

	return w.enc.Encode(entry)
}

// Close closes the log writer.
func (w *LogWriter) Close() error {
	w.mu.Lock()
	defer w.mu.Unlock()

	if w.file != nil {
		return w.file.Close()
	}
	return nil
}

// Flush flushes any buffered data to disk.
func (w *LogWriter) Flush() error {
	w.mu.Lock()
	defer w.mu.Unlock()

	if w.file != nil {
		return w.file.Sync()
	}
	return nil
}
