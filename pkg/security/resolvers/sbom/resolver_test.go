// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build linux

package sbom

import (
	"errors"
	"testing"
	"time"

	"github.com/DataDog/datadog-go/v5/statsd"
	"github.com/twmb/murmur3"

	"github.com/DataDog/datadog-agent/pkg/sbom/usage"
	"github.com/DataDog/datadog-agent/pkg/security/config"
	"github.com/DataDog/datadog-agent/pkg/security/secl/containerutils"
	"github.com/DataDog/datadog-agent/pkg/security/secl/model"
)

const testContainer = containerutils.ContainerID("cafe")

// errStreamDown stands for a transport that is not carrying reports.
var errStreamDown = errors.New("stream is down")

// fakeSource stands in for the core agent: it hands over the indexes a test
// gives it and keeps what the resolver sends back.
type fakeSource struct {
	indexes      chan *usage.Index
	capabilities usage.Capabilities
	known        bool
	sendErr      error
}

func newFakeSource() *fakeSource {
	return &fakeSource{
		indexes:      make(chan *usage.Index, 4),
		capabilities: usage.Capabilities{ContainerImage: true},
		known:        true,
	}
}

func (f *fakeSource) Indexes() <-chan *usage.Index             { return f.indexes }
func (f *fakeSource) Capabilities() (usage.Capabilities, bool) { return f.capabilities, f.known }

func (f *fakeSource) Refresh(usage.ScanID, containerutils.ContainerID) error { return nil }

func (f *fakeSource) Report(*usage.Report) error { return f.sendErr }

// containerIndex names two files of one container. An index of a container's own
// filesystem binds itself to that container, so a test can resolve an access
// against it without a workloadmeta lookup.
func containerIndex(generation uint64) *usage.Index {
	idx := &usage.Index{
		Scan:       usage.ContainerScan(string(testContainer)),
		Generation: generation,
		IndexID:    "urn:uuid:index",
		Status:     usage.Ready,
		Components: []usage.Component{
			{Purl: "pkg:deb/base-files@12", Reportable: true, Name: "base-files"},
			{Purl: "pkg:deb/coreutils@9.1", Reportable: true, Name: "coreutils"},
		},
	}

	type entry struct {
		hash uint64
		ref  uint32
	}
	entries := []entry{
		{murmur3.StringSum64("/usr/lib/os-release"), 0},
		{murmur3.StringSum64("/usr/bin/touch"), 1},
	}
	// The lookup is a binary search, so the table has to be sorted by hash.
	if entries[0].hash > entries[1].hash {
		entries[0], entries[1] = entries[1], entries[0]
	}
	for _, e := range entries {
		idx.Hashes = append(idx.Hashes, e.hash)
		idx.Refs = append(idx.Refs, e.ref)
	}
	return idx
}

func newTestResolver(t *testing.T, source IndexSource) *Resolver {
	t.Helper()

	cfg := &config.RuntimeSecurityConfig{SBOMResolverEnrichmentInterval: time.Millisecond}
	r, err := NewSBOMResolver(cfg, &statsd.NoOpClient{}, nil, source)
	if err != nil {
		t.Fatal(err)
	}
	return r
}

// access returns the context and file event of a root process reaching path in
// the test container.
func access(path string, mode uint16) (*model.ProcessContext, *model.FileEvent) {
	pc := &model.ProcessContext{}
	pc.Process.ContainerContext.ContainerID = testContainer
	return pc, &model.FileEvent{
		PathnameStr:           path,
		IsPathnameStrResolved: true,
		FileFields:            model.FileFields{Mode: mode},
	}
}

func TestResolvePackageAttributesAndReports(t *testing.T) {
	r := newTestResolver(t, newFakeSource())
	idx := containerIndex(1)
	r.setIndex(idx)

	pc, file := access("/usr/lib/os-release", 0)
	comp := r.ResolvePackage(pc, file)
	if comp == nil {
		t.Fatal("no component resolved for an indexed file")
	}
	if comp.Name != "base-files" {
		t.Errorf("component = %q, want base-files", comp.Name)
	}

	// A file no index names is left unattributed rather than guessed at.
	_, unknown := access("/etc/hosts", 0)
	if got := r.ResolvePackage(pc, unknown); got != nil {
		t.Errorf("unindexed file resolved to %q", got.Name)
	}

	report := r.workloadFor(testContainer).report()
	if report == nil {
		t.Fatal("no report after an access")
	}
	if report.Scan != idx.Scan || report.Generation != 1 || report.IndexID != idx.IndexID {
		t.Errorf("report answers {%s %d %q}, want {%s 1 %q}", report.Scan, report.Generation, report.IndexID, idx.Scan, idx.IndexID)
	}
	if len(report.Usage) != 1 || report.Usage[0].Ref != 0 {
		t.Errorf("report usage = %+v, want one entry for ref 0", report.Usage)
	}
}

func TestResolvePackageDoesNotReportUnstampableComponent(t *testing.T) {
	r := newTestResolver(t, newFakeSource())
	idx := containerIndex(1)
	idx.Components[0].Reportable = false
	r.setIndex(idx)
	if baseline := r.workloadFor(testContainer).report(); baseline == nil || len(baseline.Usage) != 0 {
		t.Fatalf("initial baseline = %+v, want an empty report", baseline)
	}

	pc, file := access("/usr/lib/os-release", 0)
	if comp := r.ResolvePackage(pc, file); comp == nil || comp.Name != "base-files" {
		t.Fatalf("package resolution lost unstampable component: %#v", comp)
	}
	if report := r.workloadFor(testContainer).report(); report != nil {
		t.Errorf("unstampable component entered usage report: %+v", report)
	}
}

func TestResolvePackageKeepsFlagsSticky(t *testing.T) {
	r := newTestResolver(t, newFakeSource())
	r.setIndex(containerIndex(1))

	// A setuid binary of a package runs, then a plain file of the same package is
	// reached. The setuid observation describes the package rather than the last
	// file touched, so it holds.
	pc, setuid := access("/usr/bin/touch", 0o4755)
	r.ResolvePackage(pc, setuid)
	_, plain := access("/usr/bin/touch", 0o755)
	r.ResolvePackage(pc, plain)

	report := r.workloadFor(testContainer).report()
	if report == nil || len(report.Usage) != 1 {
		t.Fatalf("report = %+v, want one entry", report)
	}
	if !report.Usage[0].Suid {
		t.Error("a later plain access cleared the setuid observation")
	}
	if !report.Usage[0].AsRoot {
		t.Error("the root observation was lost")
	}
}

func TestNewGenerationDropsEarlierUsage(t *testing.T) {
	r := newTestResolver(t, newFakeSource())
	r.setIndex(containerIndex(1))

	pc, file := access("/usr/lib/os-release", 0)
	r.ResolvePackage(pc, file)

	// The workload was read again, so what was observed against the table this
	// one replaces can no longer be trusted.
	r.setIndex(containerIndex(2))

	w := r.workloadFor(testContainer)
	if got := w.index.Generation; got != 2 {
		t.Fatalf("generation = %d, want 2", got)
	}
	report := w.report()
	if report == nil || len(report.Usage) != 0 || report.Generation != 2 {
		t.Errorf("new-generation baseline = %+v, want an empty generation-2 report", report)
	}
}

func TestNewIndexIDDropsEarlierUsage(t *testing.T) {
	r := newTestResolver(t, newFakeSource())
	r.setIndex(containerIndex(1))
	pc, file := access("/usr/lib/os-release", 0)
	r.ResolvePackage(pc, file)

	next := containerIndex(1)
	next.IndexID = "urn:uuid:after-core-restart"
	r.setIndex(next)
	report := r.workloadFor(testContainer).report()
	if report == nil || len(report.Usage) != 0 || report.IndexID != next.IndexID {
		t.Errorf("new-index baseline = %+v, want an empty report for %q", report, next.IndexID)
	}
}

func TestQueuedAccessesReplayOnIndexArrival(t *testing.T) {
	r := newTestResolver(t, newFakeSource())

	// The access arrives before the table that describes it.
	pc, file := access("/usr/lib/os-release", 0)
	if got := r.ResolvePackage(pc, file); got != nil {
		t.Fatalf("resolved %q with no index", got.Name)
	}

	r.setIndex(containerIndex(1))

	report := r.workloadFor(testContainer).report()
	if report == nil || len(report.Usage) != 1 {
		t.Fatalf("report = %+v, want the queued access replayed", report)
	}
	if report.Usage[0].Ref != 0 {
		t.Errorf("replayed ref = %d, want 0", report.Usage[0].Ref)
	}
}

func TestGoneIndexReleasesTheWorkload(t *testing.T) {
	r := newTestResolver(t, newFakeSource())
	r.setIndex(containerIndex(1))

	gone := containerIndex(2)
	gone.Status = usage.Gone
	r.setIndex(gone)

	if w := r.workloadFor(testContainer); w != nil {
		t.Error("the workload outlived its index")
	}
}

func TestLacksWorkloadIndexes(t *testing.T) {
	tests := []struct {
		name    string
		source  IndexSource
		lacking bool
	}{
		{
			name:    "no source at all",
			source:  nil,
			lacking: true,
		},
		{
			// Rules can load before the core agent answers, and treating silence
			// as a definite no would reject a valid rule with no way back.
			name:    "answer outstanding",
			source:  &fakeSource{},
			lacking: false,
		},
		{
			name:    "answered, scans container images",
			source:  &fakeSource{known: true, capabilities: usage.Capabilities{ContainerImage: true}},
			lacking: false,
		},
		{
			name:    "answered, scans only the host",
			source:  &fakeSource{known: true, capabilities: usage.Capabilities{Host: true}},
			lacking: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := newTestResolver(t, tt.source)
			if got := r.LacksWorkloadIndexes(); got != tt.lacking {
				t.Errorf("LacksWorkloadIndexes = %t, want %t", got, tt.lacking)
			}
		})
	}
}

func TestObservationSurvivesAFailedSend(t *testing.T) {
	source := newFakeSource()
	source.sendErr = errStreamDown
	r := newTestResolver(t, source)
	r.setIndex(containerIndex(1))

	pc, file := access("/usr/lib/os-release", 0)
	r.ResolvePackage(pc, file)

	w := r.workloadFor(testContainer)
	report := w.report()
	if report == nil {
		t.Fatal("no report to send")
	}
	// A one-off access produces no later event to mark the workload dirty again,
	// so a failed send has to put it back rather than drop what was observed.
	if err := source.Report(report); err == nil {
		t.Fatal("the fake source accepted the report")
	}
	w.redirty()

	if w.report() == nil {
		t.Error("the observation was lost to a transport error")
	}
}
