// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package usage

import (
	"slices"
	"sync"
	"testing"
	"time"

	"github.com/DataDog/agent-payload/v5/cyclonedx_v1_4"
	"github.com/google/go-cmp/cmp"
)

func component(bomRef, purl string) *cyclonedx_v1_4.Component {
	c := &cyclonedx_v1_4.Component{Name: bomRef, BomRef: &bomRef}
	if purl != "" {
		c.Purl = &purl
	}
	return c
}

func testBOM(indexID string, components ...*cyclonedx_v1_4.Component) *cyclonedx_v1_4.Bom {
	return &cyclonedx_v1_4.Bom{SerialNumber: &indexID, Components: components}
}

func properties(t *testing.T, bom *cyclonedx_v1_4.Bom, bomRef string) map[string]string {
	t.Helper()
	for _, comp := range bom.GetComponents() {
		if comp.GetBomRef() != bomRef {
			continue
		}
		out := map[string]string{}
		for _, p := range comp.GetProperties() {
			switch p.GetName() {
			case LastSeenRunningProperty, HasSetSuidBitProperty, RunningAsRootProperty:
				out[p.GetName()] = p.GetValue()
			}
		}
		return out
	}
	t.Fatalf("component %q missing from bom", bomRef)
	return nil
}

// testIndex anchors gzip and coreutils and leaves the lockfile entry
// unanchored, which is the shape a scan of an image with a stray lock file.
func testIndex() *Index {
	return &Index{
		Scan:       "image:sha256:image",
		Generation: 1,
		IndexID:    "urn:uuid:index-1",
		Components: []Component{
			{BOMRef: "gzip-ref", Purl: "pkg:rpm/gzip@1.12", Name: "gzip"},
			{BOMRef: "coreutils-ref", Purl: "pkg:rpm/coreutils@9.1", Name: "coreutils"},
			{BOMRef: "left-pad-ref", Purl: "pkg:npm/left-pad@1.3.0", Name: "left-pad"},
		},
		Hashes: []uint64{10, 20},
		Refs:   []uint32{0, 1},
	}
}

func currentReport(idx *Index, observed ...Usage) *Report {
	return &Report{Scan: idx.Scan, Generation: idx.Generation, IndexID: idx.IndexID, Usage: observed}
}

func TestStampStates(t *testing.T) {
	seen := time.Unix(1700000000, 0)
	idx := testIndex()
	table := NewTable(idx)
	if result := table.Apply(currentReport(idx, Usage{Ref: 0, LastSeen: seen, Suid: true, AsRoot: true})); !result.Applied {
		t.Fatal("Apply rejected a report for the current index")
	}

	bom := testBOM(idx.IndexID,
		component("gzip-ref", "pkg:rpm/gzip@1.12"),
		component("coreutils-ref", "pkg:rpm/coreutils@9.1"),
		component("left-pad-ref", "pkg:npm/left-pad@1.3.0"),
	)
	stamped := table.Stamp(bom)

	tests := []struct {
		name   string
		bomRef string
		want   map[string]string
	}{
		{
			name:   "anchored and reported carries the observation",
			bomRef: "gzip-ref",
			want: map[string]string{
				LastSeenRunningProperty: "1700000000",
				HasSetSuidBitProperty:   "true",
				RunningAsRootProperty:   "true",
			},
		},
		{
			name:   "anchored and unreported is idle",
			bomRef: "coreutils-ref",
			want: map[string]string{
				LastSeenRunningProperty: "0",
				HasSetSuidBitProperty:   "false",
				RunningAsRootProperty:   "false",
			},
		},
		{name: "unanchored carries no usage", bomRef: "left-pad-ref", want: map[string]string{}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if diff := cmp.Diff(tt.want, properties(t, stamped, tt.bomRef)); diff != "" {
				t.Errorf("properties mismatch (-want +got):\n%s", diff)
			}
		})
	}
	if got := table.Anchored(); got != 2 {
		t.Errorf("anchored = %d, want 2", got)
	}
}

func TestStampDistinguishesDuplicatePURLs(t *testing.T) {
	const purl = "pkg:gem/actionpack@7.0.0"
	idx := &Index{
		Scan:       "image:x",
		Generation: 1,
		IndexID:    "urn:uuid:duplicates",
		Components: []Component{
			{BOMRef: "occurrence-a", Purl: purl},
			{BOMRef: "occurrence-b", Purl: purl},
		},
		Hashes: []uint64{1, 2},
		Refs:   []uint32{0, 1},
	}
	table := NewTable(idx)
	result := table.Apply(currentReport(idx, Usage{Ref: 0, LastSeen: time.Unix(1700000000, 0)}))
	if !result.Applied {
		t.Fatal("duplicate-PURL report was rejected")
	}

	stamped := table.Stamp(testBOM(idx.IndexID,
		component("occurrence-a", purl), component("occurrence-b", purl),
	))
	if got := properties(t, stamped, "occurrence-a")[LastSeenRunningProperty]; got != "1700000000" {
		t.Errorf("observed occurrence timestamp = %q, want 1700000000", got)
	}
	if got := properties(t, stamped, "occurrence-b")[LastSeenRunningProperty]; got != "0" {
		t.Errorf("idle occurrence timestamp = %q, want 0", got)
	}
}

func TestStampPURLLessComponent(t *testing.T) {
	idx := &Index{
		Scan:       "image:x",
		Generation: 1,
		IndexID:    "urn:uuid:purl-less",
		Components: []Component{{BOMRef: "purl-less-ref", Name: "local"}},
		Hashes:     []uint64{1},
		Refs:       []uint32{0},
	}
	table := NewTable(idx)
	result := table.Apply(currentReport(idx, Usage{Ref: 0, LastSeen: time.Unix(42, 0)}))
	if !result.Applied {
		t.Fatal("PURL-less component report was rejected")
	}
	stamped := table.Stamp(testBOM(idx.IndexID, component("purl-less-ref", "")))
	if got := properties(t, stamped, "purl-less-ref")[LastSeenRunningProperty]; got != "42" {
		t.Errorf("LastSeenRunning = %q, want 42", got)
	}
}

func TestStampPreservesComponents(t *testing.T) {
	idx := &Index{
		Scan:       "image:x",
		Generation: 1,
		IndexID:    "urn:uuid:multiarch",
		Components: []Component{
			{BOMRef: "amd64-ref", Purl: "pkg:deb/libc6@2.36?arch=amd64", Name: "libc6"},
			{BOMRef: "i386-ref", Purl: "pkg:deb/libc6@2.36?arch=i386", Name: "libc6"},
		},
		Hashes: []uint64{10, 20},
		Refs:   []uint32{0, 1},
	}
	table := NewTable(idx)
	table.Apply(currentReport(idx, Usage{Ref: 0, LastSeen: time.Unix(1700000000, 0)}))
	bom := testBOM(idx.IndexID,
		component("amd64-ref", idx.Components[0].Purl), component("i386-ref", idx.Components[1].Purl),
	)
	stamped := table.Stamp(bom)
	if got := len(stamped.GetComponents()); got != len(bom.GetComponents()) {
		t.Fatalf("component count = %d, want %d", got, len(bom.GetComponents()))
	}
	if got := properties(t, stamped, "amd64-ref")[LastSeenRunningProperty]; got != "1700000000" {
		t.Errorf("amd64 LastSeenRunning = %q, want 1700000000", got)
	}
	if got := properties(t, stamped, "i386-ref")[LastSeenRunningProperty]; got != "0" {
		t.Errorf("i386 LastSeenRunning = %q, want 0", got)
	}
}

func TestStampBeforeAnyReport(t *testing.T) {
	idx := testIndex()
	bom := testBOM(idx.IndexID, component("gzip-ref", idx.Components[0].Purl))
	if diff := cmp.Diff(map[string]string{}, properties(t, NewTable(idx).Stamp(bom), "gzip-ref")); diff != "" {
		t.Errorf("properties mismatch (-want +got):\n%s", diff)
	}
}

func TestApplyRejectsWrongIdentity(t *testing.T) {
	idx := testIndex()
	idx.Generation = 2
	tests := []struct {
		name   string
		report *Report
	}{
		{name: "scan", report: &Report{Scan: "image:other", Generation: 2, IndexID: idx.IndexID}},
		{name: "generation", report: &Report{Scan: idx.Scan, Generation: 1, IndexID: idx.IndexID}},
		{name: "index ID", report: &Report{Scan: idx.Scan, Generation: 2, IndexID: "urn:uuid:old"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			table := NewTable(idx)
			if result := table.Apply(tt.report); result.Applied {
				t.Error("Apply accepted a report for another index")
			}
			if table.Reported() {
				t.Error("a rejected report marked the table reported")
			}
		})
	}
}

func TestApplyRejectsInvalidRefsAtomically(t *testing.T) {
	idx := testIndex()
	table := NewTable(idx)
	result := table.Apply(currentReport(idx,
		Usage{Ref: 0, LastSeen: time.Unix(1, 0)},
		Usage{Ref: 2, LastSeen: time.Unix(2, 0)},
		Usage{Ref: 99, LastSeen: time.Unix(3, 0)},
	))
	if result.Applied {
		t.Error("Apply accepted a report with invalid refs")
	}
	if diff := cmp.Diff([]uint32{2, 99}, result.InvalidRefs); diff != "" {
		t.Errorf("invalid refs mismatch (-want +got):\n%s", diff)
	}
	if table.Reported() || len(table.Seen()) != 0 {
		t.Error("a malformed report was partially applied")
	}
}

func TestApplyIgnoresIndexedUnstampableRefs(t *testing.T) {
	idx := testIndex()
	idx.Components[1].BOMRef = ""
	table := NewTable(idx)
	result := table.Apply(currentReport(idx,
		Usage{Ref: 0, LastSeen: time.Unix(1, 0)},
		Usage{Ref: 1, LastSeen: time.Unix(2, 0)},
	))
	if !result.Applied || len(result.InvalidRefs) != 0 {
		t.Fatalf("report containing an indexed unstampable ref was rejected: %#v", result)
	}
	if diff := cmp.Diff([]string{"gzip-ref"}, sortedKeys(table.Seen())); diff != "" {
		t.Errorf("seen refs mismatch (-want +got):\n%s", diff)
	}
}

func sortedKeys[V any](values map[string]V) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	slices.Sort(keys)
	return keys
}

func TestStampRejectsDifferentBOMInstance(t *testing.T) {
	idx := testIndex()
	table := NewTable(idx)
	if result := table.Apply(currentReport(idx, Usage{Ref: 0, LastSeen: time.Unix(42, 0)})); !result.Applied {
		t.Fatal("current report was rejected")
	}

	bom := testBOM("urn:uuid:another-index", component("gzip-ref", idx.Components[0].Purl))
	stamped := table.Stamp(bom)
	if stamped != bom {
		t.Error("Stamp copied a BOM belonging to another index")
	}
	if diff := cmp.Diff(map[string]string{}, properties(t, stamped, "gzip-ref")); diff != "" {
		t.Errorf("foreign BOM was stamped (-want +got):\n%s", diff)
	}
}

func TestDuplicateBOMRefsAreIgnored(t *testing.T) {
	idx := &Index{
		Scan:       "image:x",
		Generation: 1,
		IndexID:    "urn:uuid:duplicates",
		Components: []Component{{BOMRef: "duplicate"}, {BOMRef: "duplicate"}},
		Hashes:     []uint64{1, 2},
		Refs:       []uint32{0, 1},
	}
	table := NewTable(idx)
	if table.Anchored() != 0 {
		t.Fatal("ambiguous BOM refs were anchored")
	}
	result := table.Apply(currentReport(idx, Usage{Ref: 0}))
	if !result.Applied || len(result.InvalidRefs) != 0 || len(table.Seen()) != 0 {
		t.Errorf("ambiguous BOM-ref report result = %#v seen=%v, want accepted and ignored", result, table.Seen())
	}
}

func TestApplyKeepsUsageMonotonic(t *testing.T) {
	idx := testIndex()
	table := NewTable(idx)
	later := time.Unix(1700000100, 0)
	table.Apply(currentReport(idx, Usage{Ref: 0, LastSeen: later, Suid: true}))
	table.Apply(currentReport(idx, Usage{Ref: 0, LastSeen: time.Unix(1700000000, 0), AsRoot: true}))
	want := Usage{Ref: 0, LastSeen: later, Suid: true, AsRoot: true}
	if diff := cmp.Diff(want, table.Seen()["gzip-ref"]); diff != "" {
		t.Errorf("usage mismatch (-want +got):\n%s", diff)
	}
}

func TestStampIsIdempotent(t *testing.T) {
	idx := testIndex()
	table := NewTable(idx)
	table.Apply(currentReport(idx, Usage{Ref: 0, LastSeen: time.Unix(1700000000, 0)}))
	bom := testBOM(idx.IndexID, component("gzip-ref", idx.Components[0].Purl))
	once := table.Stamp(bom)
	twice := table.Stamp(once)
	if got := len(twice.GetComponents()[0].GetProperties()); got != 3 {
		t.Errorf("property count after a second stamp = %d, want 3", got)
	}
	if got := len(bom.GetComponents()[0].GetProperties()); got != 0 {
		t.Errorf("stamp mutated its input, property count = %d", got)
	}
	if got := properties(t, once, "gzip-ref")[LastSeenRunningProperty]; got != "1700000000" {
		t.Errorf("second stamp rewrote first result, LastSeenRunning = %q", got)
	}
}

func TestLookupPrefersApplication(t *testing.T) {
	idx := &Index{
		Components: []Component{{Name: "dep"}, {Name: "app", Application: true}},
		Hashes:     []uint64{7, 7},
		Refs:       []uint32{0, 1},
	}
	got := idx.Lookup(7)
	if len(got) != 2 {
		t.Fatalf("Lookup returned %d refs, want 2", len(got))
	}
	if name := idx.Component(got[0]).Name; name != "app" {
		t.Errorf("first component = %q, want app", name)
	}
	if idx.Lookup(8) != nil {
		t.Error("Lookup found a component for an unindexed hash")
	}
}

func TestTableIsSafeUnderConcurrentUse(t *testing.T) {
	idx := testIndex()
	table := NewTable(idx)
	bom := testBOM(idx.IndexID,
		component("gzip-ref", idx.Components[0].Purl), component("coreutils-ref", idx.Components[1].Purl),
	)
	var wg sync.WaitGroup
	for i := range 50 {
		wg.Go(func() {
			table.Apply(currentReport(idx, Usage{Ref: 0, LastSeen: time.Unix(int64(1700000000+i), 0)}))
		})
		wg.Go(func() {
			table.Stamp(bom)
			table.Seen()
			table.Anchored()
			table.Revision()
		})
	}
	wg.Wait()
	if got := table.Revision(); got != 50 {
		t.Errorf("revision = %d, want 50", got)
	}
}

func TestRevisionTracksAppliedReports(t *testing.T) {
	idx := testIndex()
	table := NewTable(idx)
	if got := table.Revision(); got != 0 {
		t.Errorf("revision before any report = %d, want 0", got)
	}
	table.Apply(currentReport(idx))
	table.Apply(&Report{Scan: idx.Scan, Generation: 99, IndexID: idx.IndexID})
	if got := table.Revision(); got != 1 {
		t.Errorf("revision = %d, want 1", got)
	}
}

func TestScanIDCarriesItsKind(t *testing.T) {
	if got := ImageScan("sha256:abc"); got.IsContainer() {
		t.Errorf("%q reported as a container scan", got)
	}
	if got := ContainerScan("abc"); !got.IsContainer() {
		t.Errorf("%q not reported as a container scan", got)
	}
	if Host.IsContainer() {
		t.Error("the host scan reported as a container scan")
	}
	if ImageScan("abc") == ContainerScan("abc") {
		t.Error("an image and a container of the same name share a ScanID")
	}
	if ImageScan("") != "" || ContainerScan("") != "" {
		t.Error("an unnamed workload got a ScanID")
	}
}
