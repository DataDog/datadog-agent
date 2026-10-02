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

	"github.com/DataDog/datadog-agent/cmd/eudm-simulator/internal/schema"
	"github.com/DataDog/datadog-agent/pkg/telemetrycapture"
)

func TestInventoryBundleRequiresRecaptureAndCompleteEvidence(t *testing.T) {
	t.Run("schema 2 recapture", func(t *testing.T) {
		dir := t.TempDir()
		if err := os.WriteFile(filepath.Join(dir, "manifest.json"), []byte(`{"schema_version":2}`), 0600); err != nil {
			t.Fatal(err)
		}
		if _, err := Load(dir, strings.Repeat("a", 40)); err == nil || !strings.Contains(err.Error(), "recapture") || !strings.Contains(err.Error(), "agent_inventory") {
			t.Fatalf("schema 2 did not explain required recapture: %v", err)
		}
	})
	for _, stream := range []schema.Stream{schema.AgentInventory, schema.HostInventory} {
		for _, missing := range []string{"stream", "cadence", "owner", "route", "protocol", "wire endpoint", "metric membership"} {
			t.Run(string(stream)+"/"+missing, func(t *testing.T) {
				dir, loaded := fixture(t, "macos")
				m := &loaded.Manifest
				index := slices.IndexFunc(m.Samples, func(ref SampleRef) bool { return ref.Stream == stream })
				ref := &m.Samples[index]
				switch missing {
				case "stream":
					m.Profile.Streams = slices.DeleteFunc(m.Profile.Streams, func(s schema.Stream) bool { return s == stream })
				case "cadence":
					delete(m.Cadences, stream)
				case "owner":
					m.Producers[0].Streams = slices.DeleteFunc(m.Producers[0].Streams, func(s schema.Stream) bool { return s == stream })
				case "route":
					ref.Routes = nil
				case "protocol":
					ref.Routes[0].Protocol = "metadata-v1"
				case "wire endpoint":
					writeBundleFile(t, dir, loaded, ref.WireFiles[0], WireReference{Path: "/intake/", Body: []byte(`{}`)})
				case "metric membership":
					ref.Routes[0].Ordinals = []uint64{1}
				}
				writeManifest(t, dir, *m)
				if _, err := Load(dir, strings.Repeat("a", 40)); err == nil {
					t.Fatal("accepted incomplete inventory evidence")
				}
			})
		}
	}
}

func TestInventoryBundleReconcilesIdentityBuildAndHardware(t *testing.T) {
	for _, test := range []struct {
		name   string
		stream schema.Stream
		mutate func(*telemetrycapture.Inventory)
	}{
		{"Agent uuid", schema.AgentInventory, func(v *telemetrycapture.Inventory) { v.UUID = "different-capture-uuid" }},
		{"host uuid", schema.HostInventory, func(v *telemetrycapture.Inventory) { v.UUID = "different-capture-uuid" }},
		{"hostname", schema.HostInventory, func(v *telemetrycapture.Inventory) { v.Hostname = "different-capture-host" }},
		{"Agent build", schema.AgentInventory, func(v *telemetrycapture.Inventory) { v.Agent.AgentVersion = "7.84.0" }},
		{"host build", schema.HostInventory, func(v *telemetrycapture.Inventory) { v.Host.AgentVersion = "7.84.0" }},
		{"platform", schema.HostInventory, func(v *telemetrycapture.Inventory) { v.Host.OS = "windows" }},
		{"logical CPU count", schema.HostInventory, func(v *telemetrycapture.Inventory) { v.Host.CPULogicalProcessors = 16 }},
		{"memory", schema.HostInventory, func(v *telemetrycapture.Inventory) { v.Host.MemoryTotalKb++ }},
		{"architecture", schema.HostInventory, func(v *telemetrycapture.Inventory) { v.Host.CPUArchitecture = "x86_64" }},
		{"collection boundary", schema.HostInventory, func(v *telemetrycapture.Inventory) { v.Timestamp = int64(time.Minute) + 1 }},
	} {
		t.Run(test.name, func(t *testing.T) {
			dir, loaded := fixture(t, "macos")
			ref := loaded.Manifest.Samples[slices.IndexFunc(loaded.Manifest.Samples, func(ref SampleRef) bool { return ref.Stream == test.stream })]
			value := telemetrycapture.CloneInventory(loaded.Samples[ref.File].Inventory)
			test.mutate(value)
			writeBundleFile(t, dir, loaded, ref.File, value)
			writeManifest(t, dir, loaded.Manifest)
			if _, err := Load(dir, strings.Repeat("a", 40)); err == nil {
				t.Fatal("accepted contradictory inventory")
			}
		})
	}
	t.Run("native OS case and optional architecture", func(t *testing.T) {
		dir, loaded := fixture(t, "macos")
		ref := loaded.Manifest.Samples[slices.IndexFunc(loaded.Manifest.Samples, func(ref SampleRef) bool { return ref.Stream == schema.HostInventory })]
		value := telemetrycapture.CloneInventory(loaded.Samples[ref.File].Inventory)
		value.Host.OS, value.Host.CPUArchitecture = "Darwin", ""
		// Native hardware inventory reports KiB, while process evidence reports bytes.
		loaded.Manifest.Profile.MemoryBytes += 511
		for _, ref := range loaded.Manifest.Samples {
			if ref.Stream == schema.Processes {
				process := loaded.Samples[ref.File].Processes
				process.Info.TotalMemory += 511
				writeBundleFile(t, dir, loaded, ref.File, process)
			}
		}
		writeBundleFile(t, dir, loaded, ref.File, value)
		writeManifest(t, dir, loaded.Manifest)
		if _, err := Load(dir, strings.Repeat("a", 40)); err != nil {
			t.Fatal(err)
		}
	})
}
