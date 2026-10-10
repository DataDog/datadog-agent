// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package store

import (
	"compress/gzip"
	"encoding/gob"
	"fmt"
	"os"
	"path/filepath"
)

const snapshotVersion = 1

type snapshot struct {
	Version int
	Logs    []*Log
	Spans   []*Span
	Series  []*Series
}

func init() {
	// attribute values decoded from JSON
	gob.Register(map[string]any{})
	gob.Register([]any{})
}

// SaveSnapshot writes the store to path atomically (gzip-compressed gob).
func (s *Store) SaveSnapshot(path string) error {
	s.mu.RLock()
	snap := snapshot{Version: snapshotVersion, Logs: s.logs, Spans: s.spans}
	for _, ser := range s.series {
		snap.Series = append(snap.Series, ser)
	}
	defer s.mu.RUnlock()

	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), ".snapshot-*")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	zw := gzip.NewWriter(tmp)
	if err := gob.NewEncoder(zw).Encode(&snap); err != nil {
		tmp.Close()
		return err
	}
	if err := zw.Close(); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), path)
}

// LoadSnapshot restores a snapshot written by SaveSnapshot. A missing file is not an error.
func (s *Store) LoadSnapshot(path string) error {
	f, err := os.Open(path)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	defer f.Close()
	zr, err := gzip.NewReader(f)
	if err != nil {
		return err
	}
	var snap snapshot
	if err := gob.NewDecoder(zr).Decode(&snap); err != nil {
		return err
	}
	if snap.Version != snapshotVersion {
		return fmt.Errorf("unsupported snapshot version %d", snap.Version)
	}
	s.AddLogs(snap.Logs)
	s.AddSpans(snap.Spans)
	for _, ser := range snap.Series {
		s.AddPoints(ser.Name, ser.Type, ser.Host, ser.Unit, ser.Interval, ser.Tags, ser.Points)
	}
	return nil
}
