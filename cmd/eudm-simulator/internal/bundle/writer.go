// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package bundle

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/DataDog/datadog-agent/cmd/eudm-simulator/internal/schema"
)

// WireReference records a serializer/forwarder request after sanitization.
// Authorization headers are excluded by the recording transport.
type WireReference struct {
	Path    string      `json:"path"`
	Headers http.Header `json:"headers"`
	Body    []byte      `json:"body"`
}

// Writer accepts only already-sanitized samples from capture transformers.
// The absence of COMPLETE makes failed or interrupted captures non-replayable.
type Writer struct {
	mu        sync.Mutex
	directory string
	manifest  Manifest
	closed    bool
}

func NewWriter(directory string, manifest Manifest) (*Writer, error) {
	if err := os.Mkdir(directory, 0700); err != nil {
		return nil, fmt.Errorf("capture requires a new output directory: %w", err)
	}
	manifest.SchemaVersion = SchemaVersion
	manifest.SanitizerVersion = SanitizerVersion
	manifest.Complete = false
	manifest.Samples = nil
	manifest.Files = map[string]string{}
	return &Writer{directory: directory, manifest: manifest}, nil
}

// Append preserves the observed offset of each native stream, including chunks.
func (w *Writer) Append(stream schema.Stream, offset time.Duration, sanitized any, references []WireReference) error {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.closed {
		return fmt.Errorf("bundle writer is closed")
	}
	if len(references) == 0 {
		return fmt.Errorf("capture sample requires serialized Agent wire references")
	}
	data, err := json.Marshal(sanitized)
	if err != nil {
		return fmt.Errorf("cannot encode sanitized sample")
	}
	prefix := fmt.Sprintf("sample-%06d", len(w.manifest.Samples))
	ref := SampleRef{Stream: stream, Offset: offset, File: prefix + ".json"}
	if err := w.write(ref.File, data); err != nil {
		return err
	}
	for i, wire := range references {
		// Headers are copied from an explicit allowlist even when the recorder
		// accidentally provides a credential-bearing header.
		headers := http.Header{}
		for _, key := range []string{"Content-Type", "Content-Encoding", "DD-Agent-Payload", "X-Dd-Hostname", "X-Dd-Processagentversion", "X-Dd-Request-Id", "X-DD-Agent-Timestamp", "X-DD-Agent-Start-Time", "X-DD-Payload-Source", "X-DD-Processes-Enabled", "X-DD-Service-Discovery-Enabled"} {
			if v := wire.Headers.Get(key); v != "" {
				headers.Set(key, v)
			}
		}
		wire.Headers = headers
		data, err := json.Marshal(wire)
		if err != nil {
			return fmt.Errorf("cannot encode sanitized wire reference")
		}
		name := fmt.Sprintf("%s-wire-%03d.json", prefix, i)
		if err := w.write(name, data); err != nil {
			return err
		}
		ref.WireFiles = append(ref.WireFiles, name)
	}
	w.manifest.Samples = append(w.manifest.Samples, ref)
	return nil
}

func (w *Writer) write(name string, data []byte) error {
	f, err := os.OpenFile(filepath.Join(w.directory, name), os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return err
	}
	_, writeErr := f.Write(data)
	syncErr := f.Sync()
	closeErr := f.Close()
	if writeErr != nil {
		return writeErr
	}
	if syncErr != nil {
		return syncErr
	}
	if closeErr != nil {
		return closeErr
	}
	w.manifest.Files[name] = schema.Digest(data)
	return nil
}

// Complete writes the manifest followed by its completion marker. A load check
// verifies coverage and all bytes before completion can be reported to a caller.
func (w *Writer) Complete(duration time.Duration, profile schema.Profile, cadences map[schema.Stream]time.Duration) (*Loaded, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.closed {
		return nil, fmt.Errorf("bundle writer is closed")
	}
	w.closed = true
	w.manifest.Duration = duration
	w.manifest.Profile = profile
	w.manifest.Cadences = cadences
	w.manifest.Complete = true
	data, err := json.MarshalIndent(w.manifest, "", "  ")
	if err != nil {
		return nil, err
	}
	// These files are not included in the manifest's own file inventory.
	if err := os.WriteFile(filepath.Join(w.directory, "manifest.json"), data, 0600); err != nil {
		return nil, err
	}
	if err := os.WriteFile(filepath.Join(w.directory, "COMPLETE"), []byte(schema.Digest(data)+"\n"), 0600); err != nil {
		return nil, err
	}
	loaded, err := Load(w.directory, w.manifest.AgentCommit)
	if err != nil {
		_ = os.Remove(filepath.Join(w.directory, "COMPLETE"))
		return nil, err
	}
	return loaded, nil
}
