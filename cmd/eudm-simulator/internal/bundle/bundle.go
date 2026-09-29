// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

// Package bundle reads revision-bound sanitized capture directories.
package bundle

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"os"
	"slices"
	"strings"
	"time"

	"github.com/DataDog/datadog-agent/cmd/eudm-simulator/internal/schema"
	"github.com/DataDog/datadog-agent/cmd/eudm-simulator/internal/telemetry"
)

const (
	SchemaVersion    = 1
	SanitizerVersion = 1
	maxManifestBytes = 4 << 20
	maxFileBytes     = 64 << 20
)

// SampleRef links a sanitized typed sample to its serialized wire references.
type SampleRef struct {
	Stream    schema.Stream `json:"stream"`
	Offset    time.Duration `json:"offset_ns"`
	File      string        `json:"file"`
	WireFiles []string      `json:"wire_files"`
}

// Manifest is written last, after all sanitized output is persisted.
type Manifest struct {
	SchemaVersion    int                             `json:"schema_version"`
	SanitizerVersion int                             `json:"sanitizer_version"`
	AgentVersion     string                          `json:"agent_version"`
	AgentCommit      string                          `json:"agent_commit"`
	Profile          schema.Profile                  `json:"profile"`
	Duration         time.Duration                   `json:"duration_ns"`
	Cadences         map[schema.Stream]time.Duration `json:"cadences_ns"`
	Samples          []SampleRef                     `json:"samples"`
	Files            map[string]string               `json:"files"`
	Complete         bool                            `json:"complete"`
}

// Loaded owns verified sample bytes, so replay never reopens mutable files.
type Loaded struct {
	Manifest Manifest
	Digest   string
	Files    map[string][]byte
	Samples  map[string]*telemetry.Sample
}

// Ref returns the portable content identity used by run plans.
func (b *Loaded) Ref() schema.BundleRef {
	return schema.BundleRef{Digest: b.Digest, AgentCommit: b.Manifest.AgentCommit, Profile: b.Manifest.Profile}
}

// DecodeJSON rejects unknown fields and concatenated JSON documents.
func DecodeJSON(data []byte, out any) error {
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()
	if err := dec.Decode(out); err != nil {
		return err
	}
	var extra any
	if err := dec.Decode(&extra); err != io.EOF {
		return fmt.Errorf("expected exactly one JSON document")
	}
	return nil
}

// Load verifies the complete bundle, including every declared digest, before
// returning any samples. The replay host's operating system is irrelevant.
func Load(directory, agentCommit string) (*Loaded, error) {
	root, err := os.OpenRoot(directory)
	if err != nil {
		return nil, fmt.Errorf("open capture bundle: %w", err)
	}
	defer root.Close()
	read := func(name string, limit int64) ([]byte, error) {
		if !fs.ValidPath(name) || strings.Contains(name, "\\") {
			return nil, fmt.Errorf("invalid bundle file name")
		}
		info, err := root.Lstat(name)
		if err != nil {
			return nil, err
		}
		if !info.Mode().IsRegular() || info.Size() > limit {
			return nil, fmt.Errorf("bundle file %q must be a bounded regular file", name)
		}
		file, err := root.Open(name)
		if err != nil {
			return nil, err
		}
		defer file.Close()
		data, err := io.ReadAll(io.LimitReader(file, limit+1))
		if int64(len(data)) > limit {
			return nil, fmt.Errorf("bundle file exceeds size limit")
		}
		return data, err
	}
	data, err := read("manifest.json", maxManifestBytes)
	if err != nil {
		return nil, fmt.Errorf("read bundle manifest: %w", err)
	}
	b := &Loaded{Digest: schema.Digest(data), Files: map[string][]byte{}}
	if err := DecodeJSON(data, &b.Manifest); err != nil {
		return nil, fmt.Errorf("decode bundle manifest: %w", err)
	}
	m := &b.Manifest
	if m.SchemaVersion != SchemaVersion || m.SanitizerVersion != SanitizerVersion {
		return nil, fmt.Errorf("unsupported bundle schema or sanitizer version; recapture")
	}
	if len(agentCommit) != 40 || m.AgentCommit != agentCommit {
		return nil, fmt.Errorf("bundle Agent commit mismatch; recapture using this exact Agent revision")
	}
	marker, err := read("COMPLETE", 65)
	if err != nil || !m.Complete || string(marker) != b.Digest+"\n" {
		return nil, fmt.Errorf("capture bundle is incomplete or completion digest does not match")
	}
	if m.Duration <= 0 || m.AgentVersion == "" || (m.Profile.OS != "windows" && m.Profile.OS != "macos") || (m.Profile.Architecture != "amd64" && m.Profile.Architecture != "arm64") {
		return nil, fmt.Errorf("invalid capture profile or duration")
	}
	if len(m.Samples) == 0 || len(m.Files) == 0 {
		return nil, fmt.Errorf("bundle contains no samples")
	}
	for name, digest := range m.Files {
		if name == "manifest.json" || name == "COMPLETE" {
			return nil, fmt.Errorf("reserved bundle file %q", name)
		}
		data, err := read(name, maxFileBytes)
		if err != nil {
			return nil, fmt.Errorf("read bundle file %q: %w", name, err)
		}
		if schema.Digest(data) != digest {
			return nil, fmt.Errorf("checksum mismatch for bundle file %q", name)
		}
		b.Files[name] = data
	}
	counts := map[schema.Stream]int{}
	last := map[schema.Stream]time.Duration{}
	used := map[string]bool{}
	for _, sample := range m.Samples {
		if !slices.Contains(m.Profile.Streams, sample.Stream) || sample.Offset < 0 || sample.Offset > m.Duration || sample.Offset < last[sample.Stream] {
			return nil, fmt.Errorf("invalid stream or sample offset")
		}
		if counts[sample.Stream] == 0 || last[sample.Stream] != sample.Offset {
			counts[sample.Stream]++
		}
		last[sample.Stream] = sample.Offset
		if _, ok := b.Files[sample.File]; !ok || used[sample.File] {
			return nil, fmt.Errorf("missing or duplicate typed sample %q", sample.File)
		}
		used[sample.File] = true
		if len(sample.WireFiles) == 0 {
			return nil, fmt.Errorf("sample %q lacks serialized references", sample.File)
		}
		for _, wire := range sample.WireFiles {
			if _, ok := b.Files[wire]; !ok || used[wire] {
				return nil, fmt.Errorf("missing or duplicate wire reference %q", wire)
			}
			used[wire] = true
		}
	}
	if len(used) != len(b.Files) {
		return nil, fmt.Errorf("bundle contains unreferenced files")
	}
	streams := map[schema.Stream]bool{}
	for _, stream := range m.Profile.Streams {
		if streams[stream] {
			return nil, fmt.Errorf("duplicate stream %s", stream)
		}
		streams[stream] = true
		requiredCount := 1
		switch stream {
		case schema.Metrics, schema.Processes, schema.Connections:
			requiredCount = 2
		case schema.HostMetadata, schema.Software:
		default:
			return nil, fmt.Errorf("unsupported capture stream %q", stream)
		}
		if counts[stream] < requiredCount || m.Cadences[stream] <= 0 {
			return nil, fmt.Errorf("stream %s lacks complete coverage or cadence", stream)
		}
		if stream == schema.Connections && m.Profile.OS != "windows" {
			return nil, fmt.Errorf("connections are unsupported for captured platform %s", m.Profile.OS)
		}
	}
	for _, stream := range []schema.Stream{schema.Metrics, schema.HostMetadata, schema.Processes, schema.Software} {
		if !streams[stream] {
			return nil, fmt.Errorf("incomplete bundle: missing %s", stream)
		}
	}
	if len(m.Cadences) != len(streams) {
		return nil, fmt.Errorf("capture cadence inventory differs from stream inventory")
	}
	if err := b.validateTyped(); err != nil {
		return nil, err
	}
	return b, nil
}
