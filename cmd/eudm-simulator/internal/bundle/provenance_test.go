// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package bundle

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	model "github.com/DataDog/agent-payload/v5/process"
	"github.com/DataDog/datadog-agent/cmd/eudm-simulator/internal/schema"
	"github.com/DataDog/datadog-agent/cmd/eudm-simulator/internal/telemetry"
)

func TestRejectIncompleteOrUnsafeProvenance(t *testing.T) {
	for _, test := range []struct {
		name   string
		mutate func(*Manifest)
	}{
		{"missing metric schedules", func(m *Manifest) { m.MetricCadences = nil }},
		{"unknown metric family", func(m *Manifest) { m.MetricCadences["private-check"] = time.Second }},
		{"invalid metric cadence", func(m *Manifest) { m.MetricCadences["cpu"] = 0 }},
		{"uncaptured slow check", func(m *Manifest) { m.MetricCadences["battery"] = 5 * time.Minute }},
		{"missing session", func(m *Manifest) { m.SessionID = "" }},
		{"missing inventory", func(m *Manifest) { m.Producers = nil }},
		{"missing producer identity", func(m *Manifest) { m.Producers[0].InstanceID = "" }},
		{"unknown producer role", func(m *Manifest) { m.Producers[0].Role = "capture-tool" }},
		{"duplicate producer role", func(m *Manifest) { m.Producers[1].Role = m.Producers[0].Role }},
		{"duplicate producer identity", func(m *Manifest) { m.Producers[1].InstanceID = m.Producers[0].InstanceID }},
		{"incompatible producer", func(m *Manifest) { m.Producers[0].ProtocolVersion++ }},
		{"invalid producer commit", func(m *Manifest) { m.Producers[0].Commit = "short" }},
		{"metadata build mismatch", func(m *Manifest) { m.Producers[0].Version = "7.84.0" }},
		{"missing stop", func(m *Manifest) { m.Producers[0].Stopped = false }},
		{"missing acknowledgement", func(m *Manifest) { m.Producers[0].AcknowledgedSequence-- }},
		{"acknowledged unseen sequence", func(m *Manifest) { m.Producers[0].AcknowledgedSequence++ }},
		{"failed producer", func(m *Manifest) { m.Producers[0].Failures = 1 }},
		{"dropped record", func(m *Manifest) { m.Producers[0].Drops = 1 }},
		{"start after origin", func(m *Manifest) { m.Producers[0].StartOffset = time.Nanosecond }},
		{"missing stop boundary", func(m *Manifest) { m.Producers[0].StopOffset = 0 }},
		{"stop before duration", func(m *Manifest) { m.Producers[0].StopOffset = m.Duration - 1 }},
		{"excessive activation skew", func(m *Manifest) { m.Producers[0].StartOffset = -6 * time.Second }},
		{"origin after all activations", func(m *Manifest) {
			for i := range m.Producers {
				m.Producers[i].StartOffset = -time.Second
			}
		}},
		{"sample at end boundary", func(m *Manifest) { m.Samples[0].Offset = m.Duration }},
		{"sample before fully armed", func(m *Manifest) { m.Producers[0].StartOffset = -time.Second; m.Samples[0].Offset = -time.Nanosecond }},
		{"wrong stream owner", func(m *Manifest) { m.Producers[0].Streams = append(m.Producers[0].Streams, schema.Processes) }},
		{"missing stream owner", func(m *Manifest) { m.Producers[0].Streams = m.Producers[0].Streams[:1] }},
		{"unknown sample producer", func(m *Manifest) { m.Samples[0].ProducerID = "unknown-producer-id" }},
		{"missing cycle", func(m *Manifest) { m.Samples[0].CycleID = 0 }},
		{"duplicate cycle", func(m *Manifest) { m.Samples[1].CycleID = m.Samples[0].CycleID }},
		{"missing sequence", func(m *Manifest) { m.Samples[0].Sequence = 0 }},
		{"sequence beyond final", func(m *Manifest) { m.Samples[0].Sequence = m.Producers[0].FinalSequence + 1 }},
		{"reused sequence", func(m *Manifest) { m.Samples[1].Sequence = m.Samples[0].Sequence }},
		{"missing chunk count", func(m *Manifest) { m.Samples[0].ChunkCount = 0 }},
		{"invalid singleton index", func(m *Manifest) { m.Samples[0].ChunkIndex = 1 }},
		{"chunked metrics", func(m *Manifest) { m.Samples[0].ChunkCount = 2 }},
	} {
		t.Run(test.name, func(t *testing.T) {
			dir, loaded := fixture(t, "macos")
			test.mutate(&loaded.Manifest)
			writeManifest(t, dir, loaded.Manifest)
			if _, err := Load(dir); err == nil {
				t.Fatal("accepted invalid live provenance")
			}
		})
	}
}

func TestDistinctCyclesNotTimestampEqualityDetermineCoverage(t *testing.T) {
	dir, loaded := fixture(t, "macos")
	loaded.Manifest.Samples[1].Offset = loaded.Manifest.Samples[0].Offset
	writeManifest(t, dir, loaded.Manifest)
	if _, err := Load(dir); err != nil {
		t.Fatal(err)
	}
}

func TestConcurrentCollectionMayArriveOutOfTimeOrder(t *testing.T) {
	dir, loaded := fixture(t, "macos")
	loaded.Manifest.Samples[0], loaded.Manifest.Samples[1] = loaded.Manifest.Samples[1], loaded.Manifest.Samples[0]
	writeManifest(t, dir, loaded.Manifest)
	if _, err := Load(dir); err != nil {
		t.Fatal(err)
	}
}

func TestConsumedNoncoverageSequencesMayBeAbsent(t *testing.T) {
	dir, loaded := fixture(t, "macos")
	loaded.Manifest.Producers[0].FinalSequence = 100
	loaded.Manifest.Producers[0].AcknowledgedSequence = 100
	loaded.Manifest.Samples[0].Sequence = 90
	writeManifest(t, dir, loaded.Manifest)
	if _, err := Load(dir); err != nil {
		t.Fatal(err)
	}
}

func TestLegacyManifestRequiresRecaptureBeforeUnknownFieldDecode(t *testing.T) {
	dir := t.TempDir()
	data := []byte(`{"schema_version":1,"agent_version":"7.85.0","agent_commit":"legacy"}`)
	if err := os.WriteFile(filepath.Join(dir, "manifest.json"), data, 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(dir); err == nil || !strings.Contains(err.Error(), "recapture") || strings.Contains(err.Error(), "unknown field") {
		t.Fatalf("legacy capture did not produce a recapture instruction: %v", err)
	}
}

func TestWindowsRequiresConnectionCoverage(t *testing.T) {
	dir, loaded := fixture(t, "windows")
	m := &loaded.Manifest
	m.Profile.Streams = slices.DeleteFunc(m.Profile.Streams, func(s schema.Stream) bool { return s == schema.Connections })
	m.Profile.ConnectionSelectors = nil
	delete(m.Cadences, schema.Connections)
	m.Producers = slices.DeleteFunc(m.Producers, func(p Producer) bool { return p.Role == "system-probe" })
	m.Samples = slices.DeleteFunc(m.Samples, func(ref SampleRef) bool {
		if ref.Stream != schema.Connections {
			return false
		}
		delete(m.Files, ref.File)
		return true
	})
	writeManifest(t, dir, *m)
	if _, err := Load(dir); err == nil || !strings.Contains(err.Error(), "missing connections") {
		t.Fatalf("Windows bundle without connections accepted: %v", err)
	}
}

func writeBundleFile(t *testing.T, dir string, loaded *Loaded, name string, value any) {
	t.Helper()
	data, err := telemetry.Encode(value)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, name), data, 0600); err != nil {
		t.Fatal(err)
	}
	loaded.Manifest.Files[name] = schema.Digest(data)
}

func fixtureProcessGroup(t *testing.T) (string, *Loaded, int) {
	t.Helper()
	dir, loaded := fixture(t, "macos")
	index := slices.IndexFunc(loaded.Manifest.Samples, func(ref SampleRef) bool { return ref.Stream == schema.Processes })
	first := &loaded.Manifest.Samples[index]
	first.ChunkCount = 2
	value := fixtureSample(t, schema.Processes, "macos").(*model.CollectorProc)
	value.GroupId, value.GroupSize = 51, 2
	second := *first
	second.ChunkIndex, second.File = 1, "extra-process.json"
	writeBundleFile(t, dir, loaded, second.File, value)
	value.Processes = nil // A metadata-only chunk is valid within this nonempty group.
	writeBundleFile(t, dir, loaded, first.File, value)
	loaded.Manifest.Samples = slices.Insert(loaded.Manifest.Samples, index+1, second)
	writeManifest(t, dir, loaded.Manifest)
	return dir, loaded, index
}

func TestCompleteOrderedProcessGroups(t *testing.T) {
	dir, _, _ := fixtureProcessGroup(t)
	if _, err := Load(dir); err != nil {
		t.Fatal(err)
	}
	for _, test := range []struct {
		name   string
		mutate func(*Loaded, int)
	}{
		{"reordered chunks", func(b *Loaded, i int) {
			b.Manifest.Samples[i], b.Manifest.Samples[i+1] = b.Manifest.Samples[i+1], b.Manifest.Samples[i]
		}},
		{"duplicate chunk", func(b *Loaded, i int) { b.Manifest.Samples[i+1].ChunkIndex = 0 }},
		{"missing chunk", func(b *Loaded, i int) { b.Manifest.Samples[i+1].CycleID = 99 }},
		{"chunk offset mismatch", func(b *Loaded, i int) { b.Manifest.Samples[i+1].Offset++ }},
		{"chunk sequence mismatch", func(b *Loaded, i int) { b.Manifest.Samples[i+1].Sequence++ }},
		{"chunk count mismatch", func(b *Loaded, i int) { b.Manifest.Samples[i+1].ChunkCount++ }},
	} {
		t.Run(test.name, func(t *testing.T) {
			dir, loaded, index := fixtureProcessGroup(t)
			test.mutate(loaded, index)
			writeManifest(t, dir, loaded.Manifest)
			if _, err := Load(dir); err == nil {
				t.Fatal("accepted incomplete group")
			}
		})
	}
}

func TestRejectInvalidTypedGroups(t *testing.T) {
	for _, kind := range []string{"empty complete group", "group id mismatch", "group size mismatch"} {
		t.Run(kind, func(t *testing.T) {
			dir, loaded, index := fixtureProcessGroup(t)
			value := fixtureSample(t, schema.Processes, "macos").(*model.CollectorProc)
			value.GroupId, value.GroupSize = 51, 2
			switch kind {
			case "empty complete group":
				value.Processes = nil
			case "group id mismatch":
				value.GroupId++
			case "group size mismatch":
				value.GroupSize++
			}
			writeBundleFile(t, dir, loaded, loaded.Manifest.Samples[index+1].File, value)
			writeManifest(t, dir, loaded.Manifest)
			if _, err := Load(dir); err == nil {
				t.Fatal("accepted invalid typed group")
			}
		})
	}
}

func TestRecordedWindowExcludesActivationSkewAndStopCleanup(t *testing.T) {
	dir, loaded := fixture(t, "macos")
	loaded.Manifest.Producers[0].StartOffset = -5 * time.Second
	for i := range loaded.Manifest.Producers {
		loaded.Manifest.Producers[i].StopOffset = loaded.Manifest.Duration + 10*time.Second
	}
	writeManifest(t, dir, loaded.Manifest)
	result, err := Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	if result.Manifest.Duration != time.Minute {
		t.Fatal("producer setup or cleanup changed recorded duration")
	}
}
