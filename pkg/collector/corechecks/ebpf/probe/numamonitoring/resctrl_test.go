// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package numamonitoring

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"
)

func makeResctrl(t *testing.T, features string) string {
	t.Helper()
	root := t.TempDir()
	for _, directory := range []string{"info/L3_MON", "mon_groups"} {
		if err := os.MkdirAll(filepath.Join(root, directory), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(root, "info/L3_MON/mon_features"), []byte(features), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "tasks"), nil, 0o644); err != nil {
		t.Fatal(err)
	}
	return root
}

func TestCapabilityMatrices(t *testing.T) {
	tests := []struct {
		name     string
		features string
		want     []string
	}{
		{"intel-amd", "llc_occupancy\nmbm_total_bytes\nmbm_local_bytes\n", []string{featureLLCOccupancy, featureMBMLocal, featureMBMTotal}},
		{"vera-like", "llc_occupancy\nmbm_total_bytes\n", []string{featureLLCOccupancy, featureMBMTotal}},
		{"grace-like", "llc_occupancy\n", []string{featureLLCOccupancy}},
		{"no-monitoring", "", nil},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			manager := newResctrlManager(makeResctrl(t, test.features), 16)
			if !slices.Equal(manager.features, test.want) {
				t.Fatalf("features = %v, want %v", manager.features, test.want)
			}
			if manager.supported() != (len(test.want) > 0) {
				t.Fatalf("supported = %t", manager.supported())
			}
		})
	}
}

func TestResctrlRatesAndUnavailable(t *testing.T) {
	root := makeResctrl(t, "llc_occupancy mbm_total_bytes mbm_local_bytes")
	manager := newResctrlManager(root, 1)
	manager.rotate(map[uint64][]int{42: {101}})
	domain := filepath.Join(manager.groupPath(42), "mon_data", "mon_L3_00")
	if err := os.MkdirAll(domain, 0o755); err != nil {
		t.Fatal(err)
	}
	write := func(name, value string) {
		t.Helper()
		if err := os.WriteFile(filepath.Join(domain, name), []byte(value), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write(featureLLCOccupancy, "4096\n")
	write(featureMBMTotal, "1000\n")
	write(featureMBMLocal, "Unavailable\n")
	first := manager.read(42, time.Unix(10, 0))
	if len(first) != 1 || first[0].LLCOccupancy == nil || first[0].TotalBandwidth != nil || first[0].LocalBandwidth != nil {
		t.Fatalf("unexpected first sample: %+v", first)
	}
	write(featureMBMTotal, "3000\n")
	write(featureMBMLocal, "1200\n")
	second := manager.read(42, time.Unix(12, 0))
	if len(second) != 1 || second[0].TotalBandwidth == nil || *second[0].TotalBandwidth != 1000 {
		t.Fatalf("unexpected second sample: %+v", second)
	}
	if second[0].LocalBandwidth != nil || second[0].RemoteBandwidth != nil {
		t.Fatalf("local and remote should wait for a prior local sample: %+v", second[0])
	}
	write(featureMBMTotal, "5000\n")
	write(featureMBMLocal, "2000\n")
	third := manager.read(42, time.Unix(14, 0))
	if third[0].RemoteBandwidth == nil || *third[0].RemoteBandwidth != 600 {
		t.Fatalf("remote bandwidth = %+v, want 600", third[0].RemoteBandwidth)
	}
	if manager.readErrors != 0 {
		t.Fatalf("unavailable counter counted as read error: %d", manager.readErrors)
	}
}

func TestForeignOwnershipCapacityRotationAndCleanup(t *testing.T) {
	root := makeResctrl(t, featureLLCOccupancy)
	foreign := filepath.Join(root, "mon_groups", "foreign")
	if err := os.Mkdir(foreign, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(foreign, "tasks"), []byte("202\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	stale := filepath.Join(root, "mon_groups", agentGroupPrefix+"999")
	if err := os.Mkdir(stale, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(stale, "tasks"), []byte("999\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	manager := newResctrlManager(root, 2)
	if _, err := os.Stat(stale); !os.IsNotExist(err) {
		t.Fatalf("stale Agent group was not reclaimed: %v", err)
	}
	manager.rotate(map[uint64][]int{1: {101}, 2: {202}})
	if len(manager.groups) != 1 {
		t.Fatalf("active groups = %v, want one", manager.groups)
	}
	if _, found := manager.groups[1]; !found {
		t.Fatalf("non-conflicting group not selected: %v", manager.groups)
	}
	if manager.conflicts != 1 {
		t.Fatalf("foreign conflicts = %d, want 1", manager.conflicts)
	}

	manager.maxGroups = 1
	manager.rotate(map[uint64][]int{3: {303}})
	if _, err := os.Stat(manager.groupPath(1)); !os.IsNotExist(err) {
		t.Fatalf("rotated group still exists: %v", err)
	}
	if _, found := manager.groups[3]; !found {
		t.Fatalf("replacement group not active: %v", manager.groups)
	}
	if manager.conflicts != 0 {
		t.Fatalf("resolved foreign conflict remains active: %d", manager.conflicts)
	}
	manager.rotate(map[uint64][]int{3: {303, 304}})
	if !slices.Equal(manager.groups[3], []int{303, 304}) {
		t.Fatalf("group tasks were not refreshed: %v", manager.groups[3])
	}
	manager.close()
	entries, err := os.ReadDir(filepath.Join(root, "mon_groups"))
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		if strings.HasPrefix(entry.Name(), agentGroupPrefix) {
			t.Fatalf("Agent group remains after shutdown: %s", entry.Name())
		}
	}
	if _, err := os.Stat(foreign); err != nil {
		t.Fatalf("foreign group was modified: %v", err)
	}
}
