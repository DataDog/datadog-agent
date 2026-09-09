// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build darwin

package notableevents

import "sync/atomic"

// CollectorStats is an aggregate snapshot of collector health.
//
// Every field is a scalar. The snapshot deliberately carries no directory
// paths, user names, or per-directory breakdowns because it is surfaced in
// flares, which leave the host. Directory identities stay hashed everywhere
// else in this package for the same reason.
type CollectorStats struct {
	// PendingEvents is the number of collected events awaiting acknowledgement
	// from the core Agent. Reaching PendingEventsMax stalls new delivery.
	PendingEvents    int
	PendingEventsMax int

	// TrackedFiles is the number of diagnostic reports recorded across all
	// directories in the bookmark.
	TrackedFiles    int
	TrackedFilesMax int

	// TrackedDirectories is the number of report directories in the bookmark.
	TrackedDirectories    int
	TrackedDirectoriesMax int

	// SaturatedDirectories counts directories holding more reports than their
	// bookmark budget allows. Their contents are baselined rather than
	// delivered until they fit again.
	SaturatedDirectories int

	// RetryDirectories counts directories awaiting a rescan after a transient
	// failure.
	RetryDirectories int

	// AcknowledgedIdentities is the size of the retained deduplication set.
	AcknowledgedIdentities    int
	AcknowledgedIdentitiesMax int

	// BookmarkUnsaved reports that in-memory state has not yet reached disk.
	BookmarkUnsaved bool

	// BookmarkStagePending reports that a bookmark save failed and the
	// collector is holding an unpublished candidate. While set, acknowledgement
	// is refused and every scan defers to retry, so delivery is stalled.
	BookmarkStagePending bool

	// WatcherActive reports whether an FSEvents stream is currently attached.
	// When false the collector relies solely on the periodic reconcile.
	WatcherActive bool

	// PersistenceErrors counts failed bookmark saves.
	PersistenceErrors uint64

	// CapacityDeferrals counts events that could not be queued because pending
	// delivery was full.
	CapacityDeferrals uint64

	// BaselineSuppressedFirstRun counts reports recorded without delivery
	// because their directory had never been scanned. This is expected on
	// install and after a bookmark reset.
	BaselineSuppressedFirstRun uint64

	// BaselineSuppressedAfterSaturation counts reports recorded without
	// delivery because their directory was recovering from saturation. Unlike
	// first-run baselining, a growing value here means real events were
	// dropped.
	BaselineSuppressedAfterSaturation uint64

	// FSEventsDrops counts callbacks where the kernel or the daemon could not
	// deliver a complete event history, forcing a full rescan.
	FSEventsDrops uint64

	// WatcherErrors counts asynchronous watcher failures other than drops.
	WatcherErrors uint64

	// WatcherRestarts counts FSEvents streams recreated after a failure.
	WatcherRestarts uint64
}

// collectorCounters holds the process-lifetime counters and the gauges that
// scanMu owns. Mirroring the scanMu-owned gauges here lets Stats run without
// acquiring scanMu, which a scan can hold across report I/O.
//
// The scan-derived counters record decisions at the moment they are taken, not
// after the owning bookmark save succeeds. A scan whose save fails is
// re-derived on the next pass, so those counters can exceed the number of
// distinct reports involved.
type collectorCounters struct {
	retryDirectories atomic.Int64
	watcherActive    atomic.Bool

	persistenceErrors                 atomic.Uint64
	capacityDeferrals                 atomic.Uint64
	baselineSuppressedFirstRun        atomic.Uint64
	baselineSuppressedAfterSaturation atomic.Uint64
	fseventsDrops                     atomic.Uint64
	watcherErrors                     atomic.Uint64
	watcherRestarts                   atomic.Uint64
}

// addUint64 increments a counter only when there is something to add, keeping
// the common zero case free of atomic traffic.
func addUint64(counter *atomic.Uint64, delta int) {
	if delta > 0 {
		counter.Add(uint64(delta))
	}
}
