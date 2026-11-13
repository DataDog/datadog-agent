// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package metrics

import (
	"sort"
	"sync"
	"time"

	"github.com/benbjohnson/clock"
)

const (
	// How long a scan that hit logs_config.open_files_limit keeps the issue open.
	// Wildcards that hover around the limit hit it on some scans and not others;
	// holding the last hit for an hour keeps the issue from flapping on every
	// 15-minute check.
	untailedFilesWindow = time.Hour

	// A path pattern is user config and legitimately longer than a name.
	untailedFilesMaxPathLen = 256
)

// UntailedFilesSource is one log source's share of a scan that hit the open files
// limit: how many of its files matched, and how many of those got no tailer.
type UntailedFilesSource struct {
	Source   string
	Service  string
	Path     string
	Matched  int
	Untailed int
}

// UntailedFilesState is the latest file scan that left files untailed because
// logs_config.open_files_limit was reached.
type UntailedFilesState struct {
	Limit    int
	Tailed   int
	Untailed int
	// Sources lists the sources that lost files, largest Untailed first, at most
	// lossMaxKeys of them; SourcesOmitted counts the rest.
	Sources        []UntailedFilesSource
	SourcesOmitted int
	LastSeenAt     time.Time
}

type untailedFilesTracker struct {
	mu    sync.Mutex
	clk   clock.Clock
	state *UntailedFilesState
}

func newUntailedFilesTracker(clk clock.Clock) *untailedFilesTracker {
	return &untailedFilesTracker{clk: clk}
}

func (t *untailedFilesTracker) record(limit, tailed int, sources []UntailedFilesSource) {
	untailed := 0
	kept := make([]UntailedFilesSource, 0, len(sources))
	for _, s := range sources {
		if s.Untailed <= 0 {
			continue
		}
		untailed += s.Untailed
		kept = append(kept, UntailedFilesSource{
			Source:   boundName(s.Source),
			Service:  boundName(s.Service),
			Path:     boundNameTo(s.Path, untailedFilesMaxPathLen),
			Matched:  s.Matched,
			Untailed: s.Untailed,
		})
	}
	if untailed == 0 {
		return
	}

	// Largest first; ties on source, service, then path so checks render identically.
	sort.Slice(kept, func(i, j int) bool {
		a, b := kept[i], kept[j]
		if a.Untailed != b.Untailed {
			return a.Untailed > b.Untailed
		}
		if a.Source != b.Source {
			return a.Source < b.Source
		}
		if a.Service != b.Service {
			return a.Service < b.Service
		}
		return a.Path < b.Path
	})
	omitted := 0
	if len(kept) > lossMaxKeys {
		omitted = len(kept) - lossMaxKeys
		// A copy, not a reslice: the state outlives this call, and a reslice would
		// keep every omitted source's strings alive with it.
		bounded := make([]UntailedFilesSource, lossMaxKeys)
		copy(bounded, kept)
		kept = bounded
	}

	state := &UntailedFilesState{
		Limit:          limit,
		Tailed:         tailed,
		Untailed:       untailed,
		Sources:        kept,
		SourcesOmitted: omitted,
		LastSeenAt:     t.clk.Now(),
	}

	t.mu.Lock()
	defer t.mu.Unlock()
	t.state = state
}

// current returns the last scan that hit the limit, if it is inside the window.
func (t *untailedFilesTracker) current() (UntailedFilesState, bool) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.state == nil {
		return UntailedFilesState{}, false
	}
	if t.clk.Since(t.state.LastSeenAt) > untailedFilesWindow {
		t.state = nil
		return UntailedFilesState{}, false
	}
	state := *t.state
	state.Sources = append([]UntailedFilesSource(nil), t.state.Sources...)
	return state, true
}

func (t *untailedFilesTracker) reset() {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.state = nil
}

// Process-wide because runner.HealthCheckFunc takes no arguments.
var untailedFiles = newUntailedFilesTracker(clock.New())

// RecordUntailedFiles records a file scan that matched more files than
// logs_config.open_files_limit lets the Agent tail. Scans that tail every match
// record nothing: the state ages out on its own after untailedFilesWindow.
func RecordUntailedFiles(limit, tailed int, sources []UntailedFilesSource) {
	untailedFiles.record(limit, tailed, sources)
}

// CurrentUntailedFiles returns the last scan that left files untailed, and false
// when no scan did within the last hour.
func CurrentUntailedFiles() (UntailedFilesState, bool) {
	return untailedFiles.current()
}
