// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

// Package bundle reads revision-bound telemetry capture directories.
package bundle

import (
	"bytes"
	"encoding/json"
	"errors"
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
	SchemaVersion    = 6
	maxManifestBytes = 4 << 20
	maxFileBytes     = 64 << 20
)

// SampleRef identifies a captured sample and its collection cycle.
type SampleRef struct {
	Stream     schema.Stream `json:"stream"`
	Offset     time.Duration `json:"offset_ns"`
	ProducerID string        `json:"producer_id"`
	CycleID    uint64        `json:"cycle_id"`
	Sequence   uint64        `json:"sequence"`
	ChunkIndex int           `json:"chunk_index"`
	ChunkCount int           `json:"chunk_count"`
	File       string        `json:"file"`
}

// BuildIdentity binds portable replay to the tool that captured the bundle.
// Installed producer builds are validated independently.
type BuildIdentity struct {
	Version string `json:"version"`
	Commit  string `json:"commit"`
}

// Producer records the acknowledged boundaries of one participating process.
type Producer struct {
	Role                 string          `json:"role"`
	InstanceID           string          `json:"instance_id"`
	Version              string          `json:"version"`
	Commit               string          `json:"commit"`
	ProtocolVersion      int             `json:"protocol_version"`
	Streams              []schema.Stream `json:"streams"`
	StartOffset          time.Duration   `json:"start_offset_ns"`
	StopOffset           time.Duration   `json:"stop_offset_ns"`
	FinalSequence        uint64          `json:"final_sequence"`
	AcknowledgedSequence uint64          `json:"acknowledged_sequence"`
	Stopped              bool            `json:"stopped"`
	Failures             uint64          `json:"failures"`
	Drops                uint64          `json:"drops"`
}

// Manifest is written last, after all captured output is persisted.
type Manifest struct {
	SchemaVersion  int                             `json:"schema_version"`
	CaptureTool    BuildIdentity                   `json:"capture_tool"`
	SessionID      string                          `json:"session_id"`
	Producers      []Producer                      `json:"producers"`
	Profile        schema.Profile                  `json:"profile"`
	Duration       time.Duration                   `json:"duration_ns"`
	Cadences       map[schema.Stream]time.Duration `json:"cadences_ns"`
	MetricCadences map[string]time.Duration        `json:"metric_cadences_ns"`
	Samples        []SampleRef                     `json:"samples"`
	Files          map[string]string               `json:"files"`
	Complete       bool                            `json:"complete"`
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
	return schema.BundleRef{Digest: b.Digest, CaptureToolCommit: b.Manifest.CaptureTool.Commit, Profile: b.Manifest.Profile}
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
		return errors.New("expected exactly one JSON document")
	}
	return nil
}

// Load verifies the complete bundle, including every declared digest, before
// returning any samples. The replay host's operating system is irrelevant.
func Load(directory, captureToolCommit string) (*Loaded, error) {
	root, err := os.OpenRoot(directory)
	if err != nil {
		return nil, fmt.Errorf("open capture bundle: %w", err)
	}
	defer root.Close()
	read := func(name string, limit int64) ([]byte, error) {
		if !fs.ValidPath(name) || strings.Contains(name, "\\") {
			return nil, errors.New("invalid bundle file name")
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
			return nil, errors.New("bundle file exceeds size limit")
		}
		return data, err
	}
	data, err := read("manifest.json", maxManifestBytes)
	if err != nil {
		return nil, fmt.Errorf("read bundle manifest: %w", err)
	}
	b := &Loaded{Digest: schema.Digest(data), Files: map[string][]byte{}}
	// Read the version before strict decoding so older bundles receive the
	// explicit instruction to recapture with the current format.
	var version struct {
		SchemaVersion int `json:"schema_version"`
	}
	if err := json.Unmarshal(data, &version); err != nil {
		return nil, errors.New("decode bundle manifest")
	}
	if version.SchemaVersion != SchemaVersion {
		return nil, errors.New("unsupported bundle schema; recapture with this version of the simulator")
	}
	if err := DecodeJSON(data, &b.Manifest); err != nil {
		return nil, fmt.Errorf("decode bundle manifest: %w", err)
	}
	m := &b.Manifest
	if m.SchemaVersion != SchemaVersion {
		return nil, errors.New("unsupported bundle schema; recapture")
	}
	if !validCommit(captureToolCommit) || m.CaptureTool.Commit != captureToolCommit {
		return nil, errors.New("bundle capture-tool commit mismatch; recapture using this exact replay revision")
	}
	marker, err := read("COMPLETE", 65)
	if err != nil || !m.Complete || string(marker) != b.Digest+"\n" {
		return nil, errors.New("capture bundle is incomplete or completion digest does not match")
	}
	if m.Duration <= 0 || (m.Profile.OS != "windows" && m.Profile.OS != "macos") || (m.Profile.Architecture != "amd64" && m.Profile.Architecture != "arm64") {
		return nil, errors.New("invalid capture profile or duration")
	}
	if len(m.Samples) == 0 || len(m.Files) == 0 {
		return nil, errors.New("bundle contains no samples")
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
	counts, err := m.validateProvenance()
	if err != nil {
		return nil, err
	}
	used := map[string]bool{}
	for _, sample := range m.Samples {
		if !slices.Contains(m.Profile.Streams, sample.Stream) || sample.Offset < 0 || sample.Offset > m.Duration {
			return nil, errors.New("invalid stream or sample offset")
		}
		if _, ok := b.Files[sample.File]; !ok || used[sample.File] {
			return nil, fmt.Errorf("missing or duplicate typed sample %q", sample.File)
		}
		used[sample.File] = true

	}
	if len(used) != len(b.Files) {
		return nil, errors.New("bundle contains unreferenced files")
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
		case schema.HostMetadata, schema.AgentInventory, schema.HostInventory, schema.HostSystemInfo, schema.Software:
		default:
			return nil, fmt.Errorf("unsupported capture stream %q", stream)
		}
		if counts[stream] < requiredCount || m.Cadences[stream] <= 0 {
			return nil, fmt.Errorf("stream %s lacks complete coverage or cadence", stream)
		}
	}
	for _, stream := range []schema.Stream{schema.Metrics, schema.HostMetadata, schema.AgentInventory, schema.HostInventory, schema.Processes, schema.Software} {
		if !streams[stream] {
			return nil, fmt.Errorf("incomplete bundle: missing %s", stream)
		}
	}
	if m.Profile.OS == "windows" && !streams[schema.Connections] {
		return nil, errors.New("incomplete Windows bundle: missing connections")
	}
	if len(m.Cadences) != len(streams) {
		return nil, errors.New("capture cadence inventory differs from stream inventory")
	}
	if err := b.validateTyped(); err != nil {
		return nil, err
	}
	if err := b.validateMetricCoverage(); err != nil {
		return nil, err
	}
	return b, nil
}
