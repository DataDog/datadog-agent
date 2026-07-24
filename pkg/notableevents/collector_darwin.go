// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build darwin

// Package notableevents collects sanitized, durable notable events on macOS.
package notableevents

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	notableeventtypes "github.com/DataDog/datadog-agent/pkg/notableevents/types"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"golang.org/x/sys/unix"
)

const (
	defaultDarwinReconcileInterval   = 5 * time.Minute
	defaultDarwinRetryInterval       = 30 * time.Second
	defaultDarwinIdentityRetention   = 30 * 24 * time.Hour
	darwinDirectoryHeartbeatInterval = 24 * time.Hour
	defaultDarwinMaxAcknowledged     = 4096
	darwinBookmarkSchemaVersion      = 1
	// Pending delivery is deliberately bounded so the 4 MiB bookmark remains
	// durable under sustained intake: at most 128 events of at most 16 KiB each.
	// Pending exposes 100 at a time to bound each poll payload.
	maxDarwinPendingEvents     = 128
	maxDarwinPendingBatch      = 100
	maxDarwinEventWireSize     = notableeventtypes.MaxEventWireSize
	maxDarwinDirectories       = 256
	maxDarwinFilesPerDirectory = 128
	maxDarwinTotalFiles        = 2048
	maxDarwinDirectoryEntries  = 1024
	maxDarwinFingerprintBytes  = 128
	diagnosticReportsDirName   = "Library/Logs/DiagnosticReports"
	systemDiagnosticReportsDir = "/Library/Logs/DiagnosticReports"
	usersDir                   = "/Users"
)

var errDarwinBookmarkCorrupt = errors.New("corrupt Darwin notable events bookmark")

type reportDirectory struct {
	path  string
	scope string
}

type reportFileState struct {
	Fingerprint  string `json:"fingerprint"`
	EventID      string `json:"event_id,omitempty"`
	BaselineOnly bool   `json:"baseline_only,omitempty"`
}

type directoryBookmarkState struct {
	Initialized bool                       `json:"initialized"`
	Saturated   bool                       `json:"saturated,omitempty"`
	Files       map[string]reportFileState `json:"files"`
	LastSeen    int64                      `json:"last_seen,omitempty"`
}

type darwinBookmarkState struct {
	Version      int                                `json:"version,omitempty"`
	Directories  map[string]*directoryBookmarkState `json:"directories"`
	Acknowledged map[string]int64                   `json:"acknowledged,omitempty"`
	Pending      map[string]Event                   `json:"pending,omitempty"`
}

type darwinBookmarkStore interface {
	Load() (*darwinBookmarkState, error)
	Save(*darwinBookmarkState) error
}

type stagedDarwinScan struct {
	candidate *darwinBookmarkState
}

type stagedDarwinScanRuntime struct {
	dirs         []reportDirectory
	retryResults map[string]bool
}

type darwinCommitKind string

const (
	darwinCommitScan           darwinCommitKind = "scan"
	darwinCommitStagedRecovery darwinCommitKind = "staged-recovery"
	darwinCommitAck            darwinCommitKind = "ack"
)

type darwinCommitReservation struct {
	id         uint64
	kind       darwinCommitKind
	generation uint64
}

// Collector monitors macOS DiagnosticReports and owns durable at-least-once
// delivery state. It is safe for concurrent use by a long-lived module.
type Collector struct {
	// scanMu serializes scanning and owns watcher, discovery, and directory-
	// retry state. stateMu owns published delivery and lifecycle
	// state. It may be acquired while scanMu is held; ACK and lifecycle paths
	// only acquire stateMu, so the reverse order is forbidden.
	scanMu  sync.Mutex
	stateMu sync.Mutex

	discoverDirs      func() []reportDirectory
	scanDirectory     func(context.Context, reportDirectory, *darwinBookmarkState) (directoryScanResult, error)
	reconcileInterval time.Duration
	retryInterval     time.Duration
	store             darwinBookmarkStore
	createWatcher     darwinReportWatcherFactory
	watcher           darwinReportWatcher
	knownDirs         map[string]reportDirectory
	retryDirs         map[string]reportDirectory
	stagedRuntime     *stagedDarwinScanRuntime

	state             *darwinBookmarkState
	stagedScan        *stagedDarwinScan
	unsaved           bool
	generation        uint64
	commitReservation *darwinCommitReservation
	nextCommitID      uint64

	now               func() time.Time
	identityRetention time.Duration
	maxAcknowledged   int

	started   bool
	closed    bool
	cancel    context.CancelFunc
	wg        sync.WaitGroup
	closeDone chan struct{}
	closeErr  error
}

// NewCollector creates a Darwin notable-event collector using its private
// root-owned state directory. Start begins filesystem monitoring.
func NewCollector() (*Collector, error) {
	return newDarwinCollectorWithDeps(
		defaultDiagnosticReportDirs,
		defaultDarwinReconcileInterval,
		defaultDarwinRetryInterval,
		newProductionDarwinBookmarkStore(),
		newDarwinReportWatcher,
	)
}

// newDarwinCollectorWithDeps constructs a collector with injectable storage, watcher, and clock dependencies.
func newDarwinCollectorWithDeps(
	discoverDirs func() []reportDirectory,
	reconcileInterval time.Duration,
	retryInterval time.Duration,
	store darwinBookmarkStore,
	createWatcher darwinReportWatcherFactory,
) (*Collector, error) {
	if reconcileInterval <= 0 {
		reconcileInterval = defaultDarwinReconcileInterval
	}
	if retryInterval <= 0 {
		retryInterval = defaultDarwinRetryInterval
	}
	if discoverDirs == nil {
		discoverDirs = func() []reportDirectory { return nil }
	}

	state, err := store.Load()
	unsaved := false
	if err != nil {
		if !errors.Is(err, errDarwinBookmarkCorrupt) {
			return nil, fmt.Errorf("load macOS notable events bookmark: %w", err)
		}
		log.Warn("Resetting corrupt macOS notable events bookmark and baselining current reports")
		state = newDarwinBookmarkState()
		unsaved = true
	}
	if state == nil {
		state = newDarwinBookmarkState()
		unsaved = true
	}
	if normalizeDarwinBookmarkState(state, time.Now()) {
		unsaved = true
	}

	collector := &Collector{
		discoverDirs:      discoverDirs,
		reconcileInterval: reconcileInterval,
		retryInterval:     retryInterval,
		store:             store,
		createWatcher:     createWatcher,
		state:             state,
		unsaved:           unsaved,
		knownDirs:         make(map[string]reportDirectory),
		retryDirs:         make(map[string]reportDirectory),
		now:               time.Now,
		identityRetention: defaultDarwinIdentityRetention,
		maxAcknowledged:   defaultDarwinMaxAcknowledged,
		closeDone:         make(chan struct{}),
	}
	collector.scanDirectory = collector.scanDirectoryInternal
	return collector, nil
}

// Start launches background reconciliation and FSEvents monitoring.
func (c *Collector) Start() error {
	c.stateMu.Lock()
	defer c.stateMu.Unlock()
	if c.closed {
		return errors.New("notable events collector is closed")
	}
	if c.started {
		return nil
	}

	runtimeCtx, cancel := context.WithCancel(context.Background())
	c.cancel = cancel
	c.started = true
	c.wg.Add(1)
	go c.run(runtimeCtx)
	return nil
}

// Close stops monitoring and waits for all collector work to finish. It is
// idempotent.
func (c *Collector) Close() error {
	// Cancellation must not wait for scanMu: a scan may be blocked in report
	// I/O and needs this signal in order to release scanMu.
	c.stateMu.Lock()
	if c.closed {
		closeDone := c.closeDone
		c.stateMu.Unlock()
		<-closeDone
		c.stateMu.Lock()
		defer c.stateMu.Unlock()
		return c.closeErr
	}
	c.closed = true
	cancel := c.cancel
	c.stateMu.Unlock()

	if cancel != nil {
		cancel()
	}
	c.wg.Wait()

	c.scanMu.Lock()
	err := c.closeWatcherLocked()
	c.scanMu.Unlock()

	c.stateMu.Lock()
	c.closeErr = err
	close(c.closeDone)
	c.stateMu.Unlock()
	return err
}

// Pending returns a deep-copied, stable snapshot without consuming events.
// Events remain pending until Ack successfully persists their IDs.
func (c *Collector) Pending() []Event {
	c.stateMu.Lock()
	defer c.stateMu.Unlock()

	ids := make([]string, 0, len(c.state.Pending))
	for id := range c.state.Pending {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	if len(ids) > maxDarwinPendingBatch {
		ids = ids[:maxDarwinPendingBatch]
	}
	events := make([]Event, 0, len(ids))
	for _, id := range ids {
		events = append(events, cloneEvent(c.state.Pending[id]))
	}
	return events
}

// Ack atomically moves pending IDs to retained acknowledgement state. Unknown
// and already acknowledged IDs are no-ops. If persistence fails, no in-memory
// event is removed and callers can safely retry.
func (c *Collector) Ack(ids []string) error {
	c.stateMu.Lock()
	if c.closed {
		c.stateMu.Unlock()
		return errors.New("acknowledge macOS notable events: collector is closed")
	}
	// Close sets closed while holding stateMu before calling Wait, so no Add can
	// race with a Wait that observed a zero counter.
	c.wg.Add(1)
	defer c.wg.Done()

	if c.stagedScan != nil || c.commitReservation != nil {
		for _, id := range ids {
			if _, pending := c.state.Pending[id]; pending {
				c.stateMu.Unlock()
				return errors.New("acknowledge macOS notable events: bookmark persistence is pending")
			}
		}
		c.stateMu.Unlock()
		return nil
	}

	next := cloneDarwinBookmarkStateForAck(c.state)
	changed := false
	nowUnix := c.currentTime().Unix()
	for _, id := range ids {
		if id == "" {
			continue
		}
		if _, pending := next.Pending[id]; pending {
			delete(next.Pending, id)
			next.Acknowledged[id] = nowUnix
			changed = true
		}
	}
	if !changed {
		c.stateMu.Unlock()
		return nil
	}
	pruneAcknowledgedToLimit(next, c.maxAcknowledged)
	if c.maxAcknowledged > 0 && len(next.Acknowledged) > c.maxAcknowledged {
		c.stateMu.Unlock()
		return errors.New("persist macOS notable event acknowledgements: acknowledgement limit exhausted")
	}
	reservation := c.reserveCommitLocked(darwinCommitAck, c.generation)
	c.stateMu.Unlock()

	if err := c.store.Save(next); err != nil {
		c.stateMu.Lock()
		c.releaseCommitLocked(reservation)
		c.stateMu.Unlock()
		return fmt.Errorf("persist macOS notable event acknowledgements: %w", err)
	}

	c.stateMu.Lock()
	c.state = next
	c.unsaved = false
	c.generation++
	c.releaseCommitLocked(reservation)
	c.stateMu.Unlock()
	return nil
}

// reserveCommitLocked grants exclusive ownership of one bookmark Save.
// stateMu must be held and callers must have already checked that no
// reservation exists.
func (c *Collector) reserveCommitLocked(kind darwinCommitKind, generation uint64) *darwinCommitReservation {
	c.nextCommitID++
	reservation := &darwinCommitReservation{
		id:         c.nextCommitID,
		kind:       kind,
		generation: generation,
	}
	c.commitReservation = reservation
	return reservation
}

// releaseCommitLocked clears only the reservation owned by the caller.
func (c *Collector) releaseCommitLocked(reservation *darwinCommitReservation) {
	if c.commitReservation == reservation {
		c.commitReservation = nil
	}
}

// run coordinates periodic scans, watcher notifications, retries, and shutdown.
func (c *Collector) run(ctx context.Context) {
	defer c.wg.Done()

	c.scanOnce(ctx)
	reconcileTicker := time.NewTicker(c.reconcileInterval)
	defer reconcileTicker.Stop()
	retryTicker := time.NewTicker(c.retryInterval)
	defer retryTicker.Stop()

	for {
		c.scanMu.Lock()
		watcherEvents, watcherErrors := c.watcherChannelsLocked()
		c.scanMu.Unlock()

		select {
		case <-ctx.Done():
			return
		case <-reconcileTicker.C:
			c.scanOnce(ctx)
		case <-retryTicker.C:
			c.retryMaintenance(ctx)
		case path, ok := <-watcherEvents:
			if !ok {
				c.scanMu.Lock()
				_ = c.closeWatcherLocked()
				c.restoreWatcherLocked()
				c.scanMu.Unlock()
				continue
			}
			c.processWatcherEvent(ctx, path)
		case err, ok := <-watcherErrors:
			if !ok {
				c.scanMu.Lock()
				_ = c.closeWatcherLocked()
				c.restoreWatcherLocked()
				c.scanMu.Unlock()
				continue
			}
			log.Warnf("macOS DiagnosticReports watcher error: %v", err)
		}
	}
}

// watcherChannelsLocked returns nil-safe watcher channels while scanMu is held.
func (c *Collector) watcherChannelsLocked() (<-chan string, <-chan error) {
	if c.watcher == nil {
		return nil, nil
	}
	return c.watcher.Events(), c.watcher.Errors()
}

// closeWatcherLocked detaches and closes the active directory watcher.
func (c *Collector) closeWatcherLocked() error {
	watcher := c.watcher
	c.watcher = nil
	if watcher == nil {
		return nil
	}
	if err := watcher.Close(); err != nil {
		log.Debugf("Failed to close macOS DiagnosticReports watcher: %v", err)
		return err
	}
	return nil
}

type directoryScanResult struct {
	StateChanged        bool
	ShouldRetry         bool
	CompleteBaseline    bool
	DirectoryKey        string
	BaselineIncidentIDs map[string]struct{}
	BaselineCompletions []darwinBaselineReportCompletion
	Deliverables        map[string]darwinScanDeliverable
}

type darwinBaselineReportCompletion struct {
	Name  string
	State reportFileState
}

type darwinScanDeliverable struct {
	Event               Event
	SourceKey           string
	ContributingDirKeys map[string]struct{}
	ContributingReports map[string]darwinReportSource
}

type darwinReportSource struct {
	DirectoryStateKey string
	Name              string
}

type darwinScanIncidents struct {
	baselineIDs  map[string]struct{}
	deliverables map[string]darwinScanDeliverable
}

// scanOnce refreshes report directories and reconciles their current contents.
func (c *Collector) scanOnce(ctx context.Context) {
	c.scanMu.Lock()
	defer c.scanMu.Unlock()

	c.ensureWatcherLocked()
	dirs := c.discoverDirs()
	c.refreshKnownDirectoriesLocked(dirs)
	hadWatcher := c.watcher != nil
	c.refreshWatchedDirectoriesLocked(dirs)
	if hadWatcher && c.watcher == nil {
		c.restoreWatcherLocked()
	}
	recoveredDirs, recovered := c.persistStagedScanLocked()
	if !recovered {
		return
	}
	dirs = mergeReportDirectories(dirs, recoveredDirs)
	c.scanDirectoriesLocked(ctx, dirs)
}

// ensureWatcherLocked lazily creates a watcher and attaches known directories.
func (c *Collector) ensureWatcherLocked() {
	if c.watcher != nil || c.createWatcher == nil {
		return
	}
	watcher, err := c.createWatcher()
	if err != nil {
		log.Warnf("Failed to initialize macOS DiagnosticReports watcher: %v", err)
		return
	}
	c.watcher = watcher
}

// processWatcherEvent coalesces queued changes and scans only affected known directories.
func (c *Collector) processWatcherEvent(ctx context.Context, firstPath string) {
	c.scanMu.Lock()
	defer c.scanMu.Unlock()

	dirsByKey := make(map[string]reportDirectory)
	c.addKnownDirectoryForPathLocked(firstPath, dirsByKey)
	watcherFailed := false
	if c.watcher != nil {
		for drainDone := false; !drainDone; {
			select {
			case path, ok := <-c.watcher.Events():
				if !ok {
					_ = c.closeWatcherLocked()
					watcherFailed = true
					drainDone = true
					break
				}
				c.addKnownDirectoryForPathLocked(path, dirsByKey)
			default:
				drainDone = true
			}
		}
	}
	recoveredDirs, recovered := c.persistStagedScanLocked()
	if !recovered {
		if watcherFailed {
			c.restoreWatcherLocked()
		}
		return
	}
	for _, dir := range recoveredDirs {
		dirsByKey[directoryRuntimeKey(dir.path)] = dir
	}
	dirs := make([]reportDirectory, 0, len(dirsByKey))
	for _, dir := range dirsByKey {
		dirs = append(dirs, dir)
	}
	c.scanDirectoriesLocked(ctx, dirs)
	if watcherFailed {
		c.restoreWatcherLocked()
	}
}

// addKnownDirectoryForPathLocked resolves a watcher path to its known report directory.
func (c *Collector) addKnownDirectoryForPathLocked(path string, dirsByKey map[string]reportDirectory) {
	dir, found := c.knownDirs[directoryRuntimeKey(path)]
	if found {
		dirsByKey[directoryRuntimeKey(dir.path)] = dir
	}
}

// retryMaintenance restores failed watcher state and retries pending directory scans.
func (c *Collector) retryMaintenance(ctx context.Context) {
	c.scanMu.Lock()
	defer c.scanMu.Unlock()
	if c.watcher == nil {
		c.restoreWatcherLocked()
	}
	dirsBeforeRecovery := c.retryDirectoriesLocked()
	recoveredDirs, recovered := c.persistStagedScanLocked()
	if !recovered {
		return
	}
	dirs := mergeReportDirectories(dirsBeforeRecovery, recoveredDirs)
	if len(dirs) == 0 {
		return
	}
	c.scanDirectoriesLocked(ctx, dirs)
}

// retryDirectoriesLocked returns a snapshot of directories awaiting a rescan.
func (c *Collector) retryDirectoriesLocked() []reportDirectory {
	dirs := make([]reportDirectory, 0, len(c.retryDirs))
	for _, dir := range c.retryDirs {
		dirs = append(dirs, dir)
	}
	return dirs
}

// restoreWatcherLocked recreates watcher coverage after an asynchronous watcher failure.
func (c *Collector) restoreWatcherLocked() {
	c.ensureWatcherLocked()
	if c.watcher != nil {
		c.refreshWatchedDirectoriesLocked(c.knownDirectoriesLocked())
	}
}

// knownDirectoriesLocked returns a stable snapshot of currently discovered report directories.
func (c *Collector) knownDirectoriesLocked() []reportDirectory {
	dirs := make([]reportDirectory, 0, len(c.knownDirs))
	for _, dir := range c.knownDirs {
		dirs = append(dirs, dir)
	}
	sort.Slice(dirs, func(i, j int) bool {
		return directoryRuntimeKey(dirs[i].path) < directoryRuntimeKey(dirs[j].path)
	})
	return dirs
}

// scanDirectoriesLocked reconciles directories while scanMu is held. Published
// state is cloned briefly under stateMu; report I/O never holds stateMu.
func (c *Collector) scanDirectoriesLocked(ctx context.Context, dirs []reportDirectory) {
	c.stateMu.Lock()
	if c.stagedScan != nil {
		c.stateMu.Unlock()
		c.markDirectoriesForRetryLocked(dirs)
		return
	}
	candidate := cloneDarwinBookmarkState(c.state)
	baseGeneration := c.generation
	dirty := c.unsaved
	c.stateMu.Unlock()

	orderedDirs := append([]reportDirectory(nil), dirs...)
	sort.Slice(orderedDirs, func(i, j int) bool {
		return directoryRuntimeKey(orderedDirs[i].path) < directoryRuntimeKey(orderedDirs[j].path)
	})
	retryResults := make(map[string]bool, len(orderedDirs))
	results := make([]directoryScanResult, 0, len(orderedDirs))
	incidents := newDarwinScanIncidents()
	for _, dir := range orderedDirs {
		select {
		case <-ctx.Done():
			markDirectoryRetries(orderedDirs, retryResults)
			c.persistCandidateLocked(candidate, dirty, orderedDirs, retryResults, baseGeneration)
			return
		default:
		}

		result, err := c.scanDirectory(ctx, dir, candidate)
		dirty = dirty || result.StateChanged
		retryResults[directoryRuntimeKey(dir.path)] = result.ShouldRetry
		results = append(results, result)
		incidents.addDirectoryResult(result)
		if err != nil && !errors.Is(err, context.Canceled) {
			log.Warnf("Failed to scan macOS DiagnosticReports directory scope=%s directory_id=%s: %v", sanitizeScope(dir.scope), directoryLogID(dir.path), err)
		}
		if errors.Is(err, context.Canceled) {
			markDirectoryRetries(orderedDirs, retryResults)
			c.persistCandidateLocked(candidate, dirty, orderedDirs, retryResults, baseGeneration)
			return
		}
	}
	if incidents.reconcile(candidate, retryResults, c.currentTime()) {
		dirty = true
	}
	for index := range results {
		result := &results[index]
		dirState := candidate.Directories[result.DirectoryKey]
		for _, completion := range result.BaselineCompletions {
			if dirState == nil {
				continue
			}
			if completion.State.EventID != "" && !eventIsAccountedFor(candidate, completion.State.EventID) {
				continue
			}
			previous, found := dirState.Files[completion.Name]
			if !found || previous != completion.State {
				dirState.Files[completion.Name] = completion.State
				dirty = true
			}
		}
		if !result.CompleteBaseline {
			continue
		}
		completeDiagnosticReportBaseline(dirState, result, true)
		dirty = dirty || result.StateChanged
	}
	if pruneAcknowledged(candidate, c.currentTime(), c.identityRetention, c.maxAcknowledged) {
		dirty = true
	}
	c.persistCandidateLocked(candidate, dirty, orderedDirs, retryResults, baseGeneration)
}

func newDarwinScanIncidents() *darwinScanIncidents {
	return &darwinScanIncidents{
		baselineIDs:  make(map[string]struct{}),
		deliverables: make(map[string]darwinScanDeliverable),
	}
}

func (i *darwinScanIncidents) addDirectoryResult(result directoryScanResult) {
	for id := range result.BaselineIncidentIDs {
		i.baselineIDs[id] = struct{}{}
	}
	for id, incoming := range result.Deliverables {
		current, found := i.deliverables[id]
		if !found {
			current = darwinScanDeliverable{
				Event:               incoming.Event,
				SourceKey:           incoming.SourceKey,
				ContributingDirKeys: make(map[string]struct{}),
				ContributingReports: make(map[string]darwinReportSource),
			}
		}
		if incoming.SourceKey < current.SourceKey {
			current.Event = incoming.Event
			current.SourceKey = incoming.SourceKey
		}
		for key := range incoming.ContributingDirKeys {
			current.ContributingDirKeys[key] = struct{}{}
		}
		for key, source := range incoming.ContributingReports {
			current.ContributingReports[key] = source
		}
		i.deliverables[id] = current
	}
}

func (i *darwinScanIncidents) reconcile(state *darwinBookmarkState, retryResults map[string]bool, now time.Time) bool {
	deliverables := make([]darwinScanDeliverable, 0, len(i.deliverables))
	for _, deliverable := range i.deliverables {
		deliverables = append(deliverables, deliverable)
	}
	sort.Slice(deliverables, func(left, right int) bool {
		if deliverables[left].SourceKey == deliverables[right].SourceKey {
			return deliverables[left].Event.ID < deliverables[right].Event.ID
		}
		return deliverables[left].SourceKey < deliverables[right].SourceKey
	})

	changed := false
	for _, deliverable := range deliverables {
		id := deliverable.Event.ID
		if _, acknowledged := state.Acknowledged[id]; acknowledged {
			continue
		}
		if _, pending := state.Pending[id]; pending {
			continue
		}
		if len(state.Pending) >= maxDarwinPendingEvents {
			for key := range deliverable.ContributingDirKeys {
				retryResults[key] = true
			}
			for _, source := range deliverable.ContributingReports {
				dirState := state.Directories[source.DirectoryStateKey]
				if dirState == nil {
					continue
				}
				if _, found := dirState.Files[source.Name]; found {
					delete(dirState.Files, source.Name)
					changed = true
				}
			}
			continue
		}
		state.Pending[id] = deliverable.Event
		changed = true
		log.Debugf("Collected macOS notable event: title=%s", deliverable.Event.Title)
	}

	baselineIDs := make([]string, 0, len(i.baselineIDs))
	for id := range i.baselineIDs {
		baselineIDs = append(baselineIDs, id)
	}
	sort.Strings(baselineIDs)
	for _, id := range baselineIDs {
		if _, deliverable := i.deliverables[id]; deliverable {
			continue
		}
		if rememberAcknowledged(state, id, now) {
			changed = true
		}
	}
	return changed
}

func markDirectoryRetries(dirs []reportDirectory, retryResults map[string]bool) {
	for _, dir := range dirs {
		retryResults[directoryRuntimeKey(dir.path)] = true
	}
}

// persistCandidateLocked reserves, saves, and publishes a complete candidate
// while scanMu is held. Save runs without stateMu, so Pending remains
// responsive. A failed save atomically becomes an immutable staged candidate.
func (c *Collector) persistCandidateLocked(
	candidate *darwinBookmarkState,
	dirty bool,
	dirs []reportDirectory,
	retryResults map[string]bool,
	baseGeneration uint64,
) {
	if !dirty {
		c.stateMu.Lock()
		staleOrBusy := c.generation != baseGeneration || c.stagedScan != nil || c.commitReservation != nil
		c.stateMu.Unlock()
		if staleOrBusy {
			c.markDirectoriesForRetryLocked(dirs)
			return
		}
		c.applyDirectoryRetryResultsLocked(dirs, retryResults)
		return
	}

	staged := newStagedDarwinScan(candidate)
	runtime := newStagedDarwinScanRuntime(dirs, retryResults)

	c.stateMu.Lock()
	if c.generation != baseGeneration || c.stagedScan != nil || c.commitReservation != nil {
		c.stateMu.Unlock()
		c.markDirectoriesForRetryLocked(dirs)
		return
	}
	reservation := c.reserveCommitLocked(darwinCommitScan, baseGeneration)
	c.stateMu.Unlock()

	if err := c.store.Save(cloneDarwinBookmarkState(staged.candidate)); err != nil {
		c.stagedRuntime = runtime
		c.stateMu.Lock()
		c.stagedScan = staged
		c.releaseCommitLocked(reservation)
		c.stateMu.Unlock()
		c.markStagedDirectoriesForRetryLocked(runtime)
		log.Warnf("Failed to save macOS notable events bookmark: %v", err)
		return
	}

	c.stateMu.Lock()
	c.state = staged.candidate
	c.unsaved = false
	c.generation++
	c.releaseCommitLocked(reservation)
	c.stateMu.Unlock()
	c.applyDirectoryRetryResultsLocked(runtime.dirs, runtime.retryResults)
}

// persistStagedScanLocked retries an unpublished candidate before any new scan.
// The staged record itself is never passed to storage or otherwise mutated.
func (c *Collector) persistStagedScanLocked() ([]reportDirectory, bool) {
	c.stateMu.Lock()
	staged := c.stagedScan
	if staged == nil {
		c.stateMu.Unlock()
		return nil, true
	}
	if c.commitReservation != nil {
		c.stateMu.Unlock()
		c.markStagedDirectoriesForRetryLocked(c.stagedRuntime)
		return nil, false
	}
	reservation := c.reserveCommitLocked(darwinCommitStagedRecovery, c.generation)
	c.stateMu.Unlock()

	if err := c.store.Save(cloneDarwinBookmarkState(staged.candidate)); err != nil {
		c.stateMu.Lock()
		c.releaseCommitLocked(reservation)
		c.stateMu.Unlock()
		c.markStagedDirectoriesForRetryLocked(c.stagedRuntime)
		log.Warnf("Failed to save staged macOS notable events bookmark: %v", err)
		return nil, false
	}

	c.stateMu.Lock()
	c.state = staged.candidate
	c.unsaved = false
	c.generation++
	c.stagedScan = nil
	c.releaseCommitLocked(reservation)
	c.stateMu.Unlock()

	runtime := c.stagedRuntime
	c.stagedRuntime = nil
	if runtime != nil {
		c.applyDirectoryRetryResultsLocked(runtime.dirs, runtime.retryResults)
	}
	return c.knownStagedDirectoriesLocked(runtime), true
}

func newStagedDarwinScan(candidate *darwinBookmarkState) *stagedDarwinScan {
	return &stagedDarwinScan{
		candidate: cloneDarwinBookmarkState(candidate),
	}
}

func newStagedDarwinScanRuntime(dirs []reportDirectory, retryResults map[string]bool) *stagedDarwinScanRuntime {
	runtime := &stagedDarwinScanRuntime{
		dirs:         append([]reportDirectory(nil), dirs...),
		retryResults: make(map[string]bool, len(retryResults)),
	}
	for key, shouldRetry := range retryResults {
		runtime.retryResults[key] = shouldRetry
	}
	return runtime
}

// markStagedDirectoriesForRetryLocked keeps failed work retryable without
// resurrecting directories removed by a later discovery refresh.
func (c *Collector) markStagedDirectoriesForRetryLocked(runtime *stagedDarwinScanRuntime) {
	if runtime == nil {
		return
	}
	for _, dir := range runtime.dirs {
		key := directoryRuntimeKey(dir.path)
		if known, found := c.knownDirs[key]; found {
			c.setDirectoryRetryLocked(known, true)
		}
	}
}

func (c *Collector) knownStagedDirectoriesLocked(runtime *stagedDarwinScanRuntime) []reportDirectory {
	if runtime == nil {
		return nil
	}
	dirs := make([]reportDirectory, 0, len(runtime.dirs))
	for _, dir := range runtime.dirs {
		if known, found := c.knownDirs[directoryRuntimeKey(dir.path)]; found {
			dirs = append(dirs, known)
		}
	}
	return dirs
}

func (c *Collector) markDirectoriesForRetryLocked(dirs []reportDirectory) {
	for _, dir := range dirs {
		if known, found := c.knownDirs[directoryRuntimeKey(dir.path)]; found {
			c.setDirectoryRetryLocked(known, true)
		}
	}
}

// applyDirectoryRetryResultsLocked publishes transient scan retry decisions.
func (c *Collector) applyDirectoryRetryResultsLocked(dirs []reportDirectory, retryResults map[string]bool) {
	for _, dir := range dirs {
		key := directoryRuntimeKey(dir.path)
		if known, found := c.knownDirs[key]; found {
			c.setDirectoryRetryLocked(known, retryResults[key])
		}
	}
}

func mergeReportDirectories(groups ...[]reportDirectory) []reportDirectory {
	byKey := make(map[string]reportDirectory)
	for _, dirs := range groups {
		for _, dir := range dirs {
			byKey[directoryRuntimeKey(dir.path)] = dir
		}
	}
	merged := make([]reportDirectory, 0, len(byKey))
	for _, dir := range byKey {
		merged = append(merged, dir)
	}
	sort.Slice(merged, func(i, j int) bool {
		return directoryRuntimeKey(merged[i].path) < directoryRuntimeKey(merged[j].path)
	})
	return merged
}

// refreshKnownDirectoriesLocked replaces runtime directory metadata with fresh discovery results.
func (c *Collector) refreshKnownDirectoriesLocked(dirs []reportDirectory) {
	nextKnown := make(map[string]reportDirectory, len(dirs))
	for _, dir := range dirs {
		nextKnown[directoryRuntimeKey(dir.path)] = dir
	}
	c.knownDirs = nextKnown
	for key := range c.retryDirs {
		if _, found := c.knownDirs[key]; !found {
			delete(c.retryDirs, key)
		}
	}
}

// refreshWatchedDirectoriesLocked synchronizes FSEvents coverage with watchable directories.
func (c *Collector) refreshWatchedDirectoriesLocked(dirs []reportDirectory) {
	if c.watcher == nil {
		return
	}
	watchPaths := make([]string, 0, len(dirs))
	for _, dir := range dirs {
		if isWatchableDiagnosticReportDirectory(dir.path) {
			watchPaths = append(watchPaths, filepath.Clean(dir.path))
		}
	}
	sort.Strings(watchPaths)
	if err := c.watcher.Update(watchPaths); err != nil {
		log.Warnf("Failed to update macOS DiagnosticReports watcher: %v", err)
		_ = c.closeWatcherLocked()
	}
}

// setDirectoryRetryLocked adds or removes a directory from transient retry tracking.
func (c *Collector) setDirectoryRetryLocked(dir reportDirectory, shouldRetry bool) {
	key := directoryRuntimeKey(dir.path)
	if shouldRetry {
		c.retryDirs[key] = dir
	} else {
		delete(c.retryDirs, key)
	}
}

// scanDirectoryInternal securely reads reports and updates only the private
// candidate passed by the serialized scanner.
func (c *Collector) scanDirectoryInternal(ctx context.Context, dir reportDirectory, state *darwinBookmarkState) (directoryScanResult, error) {
	directory, err := openDiagnosticReportDirectory(dir.path)
	if err != nil {
		if os.IsNotExist(err) || os.IsPermission(err) {
			log.Debugf("Skipping macOS DiagnosticReports directory scope=%s directory_id=%s: %v", sanitizeScope(dir.scope), directoryLogID(dir.path), err)
			return directoryScanResult{}, nil
		}
		return directoryScanResult{}, err
	}
	defer directory.Close()

	entries, directoryComplete, err := readBoundedDirectoryEntries(directory)
	if err != nil {
		return directoryScanResult{}, err
	}

	dirKey := hashString(filepath.Clean(dir.path))
	dirState, found := state.Directories[dirKey]
	nowUnix := c.currentTime().Unix()
	newDirectoryState := !found || dirState == nil
	if newDirectoryState {
		if len(state.Directories) >= maxDarwinDirectories {
			return directoryScanResult{}, nil
		}
		dirState = &directoryBookmarkState{
			Files:    make(map[string]reportFileState),
			LastSeen: nowUnix,
		}
		state.Directories[dirKey] = dirState
	}
	if dirState.Files == nil {
		dirState.Files = make(map[string]reportFileState)
	}

	result := directoryScanResult{
		StateChanged: newDirectoryState,
		DirectoryKey: dirKey,
	}
	if !newDirectoryState && nowUnix-dirState.LastSeen >= int64(darwinDirectoryHeartbeatInterval/time.Second) {
		dirState.LastSeen = nowUnix
		result.StateChanged = true
	}

	reportEntryCount := 0
	for _, entry := range entries {
		if !entry.IsDir() && entry.Type()&os.ModeSymlink == 0 && validateReportBasename(entry.Name()) == nil {
			reportEntryCount++
		}
	}
	availableFiles := maxDarwinTotalFiles - countDarwinFilesExcludingDirectory(state, dirKey)
	if availableFiles > maxDarwinFilesPerDirectory {
		availableFiles = maxDarwinFilesPerDirectory
	}
	if !directoryComplete || reportEntryCount > availableFiles {
		if !dirState.Saturated {
			dirState.Saturated = true
			result.StateChanged = true
		}
		return result, nil
	}

	baselineScan := !dirState.Initialized || dirState.Saturated
	presentFiles := make(map[string]struct{})

	for _, entry := range entries {
		select {
		case <-ctx.Done():
			return result, context.Canceled
		default:
		}

		name := entry.Name()
		if entry.IsDir() || entry.Type()&os.ModeSymlink != 0 || validateReportBasename(name) != nil {
			continue
		}
		presentFiles[name] = struct{}{}

		reportFile, err := openSafeReportFile(directory, name)
		if err != nil {
			recordDiagnosticReportOpenError(&result, err)
			log.Debugf("Skipping macOS DiagnosticReports file scope=%s directory_id=%s: %v", sanitizeScope(dir.scope), dirKey, err)
			continue
		}

		previous, previouslySeen := dirState.Files[name]
		if previouslySeen && previous.Fingerprint == reportFile.fingerprint &&
			(previous.EventID == "" || eventIsAccountedFor(state, previous.EventID)) {
			_ = reportFile.Close()
			continue
		}

		nextState := reportFileState{Fingerprint: reportFile.fingerprint}
		suppressOnSuccess := previouslySeen && previous.BaselineOnly
		report, isCrash, readErr := readMacOSCrashReportFile(reportFile)
		transientReadError := readErr != nil && isTransientDiagnosticReportReadError(readErr, reportFile)
		if transientReadError {
			result.ShouldRetry = true
		}
		_ = reportFile.Close()
		if readErr != nil {
			if !transientReadError {
				nextState.BaselineOnly = baselineScan || suppressOnSuccess
				if !previouslySeen || previous != nextState {
					dirState.Files[name] = nextState
					result.StateChanged = true
				}
			}
			log.Debugf("Skipping macOS DiagnosticReports file scope=%s directory_id=%s: %v", sanitizeScope(dir.scope), dirKey, readErr)
			continue
		}

		if !isCrash {
			if !previouslySeen || previous != nextState {
				dirState.Files[name] = nextState
				result.StateChanged = true
			}
			continue
		}

		identity := report.identity(name, reportFile.fingerprint)
		event := report.event(identity, dir.scope)
		nextState.EventID = event.ID
		if !eventFitsWireLimit(event) {
			result.ShouldRetry = true
			continue
		}

		if baselineScan || suppressOnSuccess {
			if suppressOnSuccess && !baselineScan {
				result.BaselineCompletions = append(result.BaselineCompletions, darwinBaselineReportCompletion{
					Name:  name,
					State: nextState,
				})
			} else if !previouslySeen || previous != nextState {
				dirState.Files[name] = nextState
				result.StateChanged = true
			}
			if result.BaselineIncidentIDs == nil {
				result.BaselineIncidentIDs = make(map[string]struct{})
			}
			result.BaselineIncidentIDs[event.ID] = struct{}{}
			continue
		}

		if _, acknowledged := state.Acknowledged[event.ID]; !acknowledged {
			if result.Deliverables == nil {
				result.Deliverables = make(map[string]darwinScanDeliverable)
			}
			sourceKey := directoryRuntimeKey(dir.path) + "\x00" + name
			current, found := result.Deliverables[event.ID]
			if !found {
				current = darwinScanDeliverable{
					Event:               event,
					SourceKey:           sourceKey,
					ContributingDirKeys: make(map[string]struct{}),
					ContributingReports: make(map[string]darwinReportSource),
				}
			} else if sourceKey < current.SourceKey {
				current.Event = event
				current.SourceKey = sourceKey
			}
			current.ContributingDirKeys[directoryRuntimeKey(dir.path)] = struct{}{}
			current.ContributingReports[sourceKey] = darwinReportSource{
				DirectoryStateKey: dirKey,
				Name:              name,
			}
			result.Deliverables[event.ID] = current
		}
		if !previouslySeen || previous != nextState {
			dirState.Files[name] = nextState
			result.StateChanged = true
		}
	}

	result.CompleteBaseline = baselineScan

	for name := range dirState.Files {
		if _, present := presentFiles[name]; !present {
			delete(dirState.Files, name)
			result.StateChanged = true
		}
	}

	return result, nil
}

// recordDiagnosticReportOpenError applies the retry policy without exposing a
// broad filesystem seam in production scanning.
func recordDiagnosticReportOpenError(result *directoryScanResult, err error) {
	if result != nil && isTransientDiagnosticReportOpenError(err) {
		result.ShouldRetry = true
	}
}

// completeDiagnosticReportBaseline completes a baseline only after all
// retryable open/read failures have cleared.
func completeDiagnosticReportBaseline(dirState *directoryBookmarkState, result *directoryScanResult, baselineScan bool) {
	if dirState == nil || result == nil || !baselineScan || result.ShouldRetry {
		return
	}
	if !dirState.Initialized {
		dirState.Initialized = true
		result.StateChanged = true
	}
	if dirState.Saturated {
		dirState.Saturated = false
		result.StateChanged = true
	}
}

// eventIsAccountedFor reports whether an event is already pending or acknowledged.
func eventIsAccountedFor(state *darwinBookmarkState, id string) bool {
	if _, found := state.Acknowledged[id]; found {
		return true
	}
	_, found := state.Pending[id]
	return found
}

// eventFitsWireLimit verifies that one pending event cannot exceed its durable
// or transport allocation.
func eventFitsWireLimit(event Event) bool {
	return notableeventtypes.ValidateEvent(event) == nil
}

// identity selects an incident-based identity or a file-derived fallback for deduplication.
func (r *macOSCrashReport) identity(name, fingerprint string) string {
	if incidentID := r.incidentID(); incidentID != "" {
		return "incident:" + incidentID
	}
	return fmt.Sprintf("file:%s:%s", name, fingerprint)
}

// eventID converts a private report identity into the stable public event identifier.
func eventID(identity string) string {
	return "macos-crash-v1:" + hashString(identity)
}

// newDarwinBookmarkState creates empty state at the current schema version.
func newDarwinBookmarkState() *darwinBookmarkState {
	return &darwinBookmarkState{
		Version:      darwinBookmarkSchemaVersion,
		Directories:  make(map[string]*directoryBookmarkState),
		Acknowledged: make(map[string]int64),
		Pending:      make(map[string]Event),
	}
}

// normalizeDarwinBookmarkState repairs missing state collections.
func normalizeDarwinBookmarkState(state *darwinBookmarkState, now time.Time) bool {
	changed := state.Version != darwinBookmarkSchemaVersion
	state.Version = darwinBookmarkSchemaVersion
	if state.Directories == nil {
		state.Directories = make(map[string]*directoryBookmarkState)
		changed = true
	}
	if state.Acknowledged == nil {
		state.Acknowledged = make(map[string]int64)
		changed = true
	}
	if state.Pending == nil {
		state.Pending = make(map[string]Event)
		changed = true
	}

	nowUnix := now.Unix()
	for id, lastSeen := range state.Acknowledged {
		if lastSeen <= 0 {
			state.Acknowledged[id] = nowUnix
			changed = true
		}
	}
	for key, event := range state.Pending {
		if event.ID == "" {
			event.ID = key
			state.Pending[key] = event
			changed = true
		}
		if event.ID != key {
			delete(state.Pending, key)
			state.Pending[event.ID] = event
			changed = true
		}
	}
	for _, dirState := range state.Directories {
		if dirState == nil {
			continue
		}
		if dirState.Files == nil {
			dirState.Files = make(map[string]reportFileState)
			changed = true
		}
		if dirState.LastSeen <= 0 {
			dirState.LastSeen = nowUnix
			changed = true
		}
	}
	return changed
}

// cloneDarwinBookmarkState explicitly deep-copies mutable reconciliation state.
func cloneDarwinBookmarkState(state *darwinBookmarkState) *darwinBookmarkState {
	cloned := &darwinBookmarkState{
		Version:      state.Version,
		Directories:  make(map[string]*directoryBookmarkState, len(state.Directories)),
		Acknowledged: cloneAcknowledgedMap(state.Acknowledged),
		Pending:      clonePendingMapDeep(state.Pending),
	}
	for key, directory := range state.Directories {
		if directory == nil {
			cloned.Directories[key] = nil
			continue
		}
		files := make(map[string]reportFileState, len(directory.Files))
		for name, fileState := range directory.Files {
			files[name] = fileState
		}
		cloned.Directories[key] = &directoryBookmarkState{
			Initialized: directory.Initialized,
			Saturated:   directory.Saturated,
			Files:       files,
			LastSeen:    directory.LastSeen,
		}
	}
	return cloned
}

// cloneDarwinBookmarkStateForAck copies only maps ACK mutates. Directory state
// is safely shared because acknowledgement never changes it.
func cloneDarwinBookmarkStateForAck(state *darwinBookmarkState) *darwinBookmarkState {
	return &darwinBookmarkState{
		Version:      state.Version,
		Directories:  state.Directories,
		Acknowledged: cloneAcknowledgedMap(state.Acknowledged),
		Pending:      clonePendingMap(state.Pending),
	}
}

func cloneAcknowledgedMap(source map[string]int64) map[string]int64 {
	cloned := make(map[string]int64, len(source))
	for id, timestamp := range source {
		cloned[id] = timestamp
	}
	return cloned
}

func clonePendingMap(source map[string]Event) map[string]Event {
	cloned := make(map[string]Event, len(source))
	for id, event := range source {
		// Event values are immutable while internal. ACK only deletes entries,
		// so their nested maps do not need copying on this hot path.
		cloned[id] = event
	}
	return cloned
}

func clonePendingMapDeep(source map[string]Event) map[string]Event {
	cloned := make(map[string]Event, len(source))
	for id, event := range source {
		cloned[id] = cloneEvent(event)
	}
	return cloned
}

// cloneEvent deep-copies an event before exposing or persisting it.
func cloneEvent(event Event) Event {
	if event.Custom != nil {
		event.Custom = cloneJSONValue(event.Custom).(map[string]interface{})
	}
	return event
}

// cloneJSONValue recursively copies every mutable shape accepted by
// validateDarwinCustomValue.
func cloneJSONValue(value interface{}) interface{} {
	switch typed := value.(type) {
	case map[string]interface{}:
		cloned := make(map[string]interface{}, len(typed))
		for key, child := range typed {
			cloned[key] = cloneJSONValue(child)
		}
		return cloned
	case []interface{}:
		cloned := make([]interface{}, len(typed))
		for index, child := range typed {
			cloned[index] = cloneJSONValue(child)
		}
		return cloned
	default:
		return value
	}
}

// currentTime returns the injectable collector clock in UTC.
func (c *Collector) currentTime() time.Time {
	if c.now == nil {
		return time.Now()
	}
	return c.now()
}

// rememberAcknowledged records an accounted-for event and reports whether state changed.
func rememberAcknowledged(state *darwinBookmarkState, id string, now time.Time) bool {
	if id == "" {
		return false
	}
	// A baseline in one directory can encounter the same incident that is
	// already pending from another directory. Pending delivery takes
	// precedence; recording the ID as acknowledged as well would create an
	// unloadable bookmark and could suppress the event after restart.
	if _, pending := state.Pending[id]; pending {
		return false
	}
	lastSeen := now.Unix()
	if previous, found := state.Acknowledged[id]; found && previous >= lastSeen {
		return false
	}
	state.Acknowledged[id] = lastSeen
	return true
}

// pruneAcknowledged removes expired or excess acknowledgements while retaining pending references.
func pruneAcknowledged(state *darwinBookmarkState, now time.Time, identityRetention time.Duration, maxAcknowledged int) bool {
	changed := false
	cutoff := now.Add(-identityRetention).Unix()
	if identityRetention > 0 {
		for directoryKey, dirState := range state.Directories {
			if dirState == nil || (dirState.LastSeen > 0 && dirState.LastSeen < cutoff) {
				delete(state.Directories, directoryKey)
				changed = true
			}
		}
	}

	referenced := make(map[string]struct{})
	for _, dirState := range state.Directories {
		if dirState == nil {
			continue
		}
		for _, fileState := range dirState.Files {
			if fileState.EventID != "" {
				referenced[fileState.EventID] = struct{}{}
			}
		}
	}

	if identityRetention > 0 {
		for id, lastSeen := range state.Acknowledged {
			if _, found := referenced[id]; found {
				continue
			}
			if lastSeen < cutoff {
				delete(state.Acknowledged, id)
				changed = true
			}
		}
	}

	return pruneAcknowledgedToLimit(state, maxAcknowledged) || changed
}

// pruneAcknowledgedToLimit removes the oldest unreferenced acknowledgements.
func pruneAcknowledgedToLimit(state *darwinBookmarkState, maxAcknowledged int) bool {
	if maxAcknowledged <= 0 || len(state.Acknowledged) <= maxAcknowledged {
		return false
	}

	referenced := make(map[string]struct{})
	for _, dirState := range state.Directories {
		if dirState == nil {
			continue
		}
		for _, fileState := range dirState.Files {
			if fileState.EventID != "" {
				referenced[fileState.EventID] = struct{}{}
			}
		}
	}

	type retainedAcknowledgement struct {
		id       string
		lastSeen int64
	}
	candidates := make([]retainedAcknowledgement, 0, len(state.Acknowledged))
	for id, lastSeen := range state.Acknowledged {
		if _, found := referenced[id]; !found {
			candidates = append(candidates, retainedAcknowledgement{id: id, lastSeen: lastSeen})
		}
	}
	sort.Slice(candidates, func(i, j int) bool {
		if candidates[i].lastSeen == candidates[j].lastSeen {
			return candidates[i].id < candidates[j].id
		}
		return candidates[i].lastSeen < candidates[j].lastSeen
	})
	changed := false
	for _, candidate := range candidates {
		if len(state.Acknowledged) <= maxAcknowledged {
			break
		}
		delete(state.Acknowledged, candidate.id)
		changed = true
	}
	return changed
}

// defaultDiagnosticReportDirs discovers system and current-user diagnostic report locations.
func defaultDiagnosticReportDirs() []reportDirectory {
	dirs := []reportDirectory{{path: systemDiagnosticReportsDir, scope: "system"}}
	userEntries, err := os.ReadDir(usersDir)
	if err != nil {
		log.Debugf("Failed to read %s while discovering macOS DiagnosticReports directories: %v", usersDir, err)
		return dirs
	}
	for _, entry := range userEntries {
		if !entry.IsDir() || strings.HasPrefix(entry.Name(), ".") || entry.Name() == "Shared" || entry.Name() == "Guest" {
			continue
		}
		dirs = append(dirs, reportDirectory{
			path:  filepath.Join(usersDir, entry.Name(), diagnosticReportsDirName),
			scope: "user",
		})
	}
	return dirs
}

// directoryRuntimeKey normalizes a directory path for in-memory lookup.
func directoryRuntimeKey(path string) string {
	return filepath.Clean(path)
}

// isWatchableDiagnosticReportDirectory verifies that a directory can be opened without symlink traversal.
func isWatchableDiagnosticReportDirectory(path string) bool {
	directory, err := openDiagnosticReportDirectory(path)
	if err != nil {
		return false
	}
	_ = directory.Close()
	return true
}

// isTransientDiagnosticReportOpenError identifies report-open failures worth
// retrying. EACCES and EPERM are intentionally permanent so one inaccessible
// historical report cannot prevent baseline completion indefinitely. If access
// is restored later, that historical report may consequently be emitted.
func isTransientDiagnosticReportOpenError(err error) bool {
	if err == nil {
		return false
	}
	var policyError *diagnosticReportPolicyError
	if errors.As(err, &policyError) {
		return false
	}
	for _, retryable := range []error{
		unix.ENOENT,
		unix.EINTR,
		unix.EIO,
		unix.EAGAIN,
		unix.EBUSY,
		unix.ESTALE,
		unix.ETIMEDOUT,
		unix.EMFILE,
		unix.ENFILE,
		unix.ENOMEM,
	} {
		if errors.Is(err, retryable) {
			return true
		}
	}
	return false
}

// isTransientDiagnosticReportReadError identifies reports that changed while
// being read. Stable malformed reports are fingerprinted so they cannot block a
// baseline forever and are reparsed if their contents later change.
func isTransientDiagnosticReportReadError(err error, reportFile *safeReportFile) bool {
	if err == nil || reportFile == nil {
		return false
	}
	return !reportFile.unchanged()
}

// hashString irreversibly hashes private identifiers before persistence or emission.
func hashString(value string) string {
	sum := sha256.Sum256([]byte(value))
	return hex.EncodeToString(sum[:])
}

// directoryLogID provides non-reversible context without exposing a user path.
func directoryLogID(path string) string {
	return hashString(filepath.Clean(path))
}

// readBoundedDirectoryEntries caps both directory enumeration work and retained entry memory.
func readBoundedDirectoryEntries(directory *os.File) ([]os.DirEntry, bool, error) {
	entries, err := directory.ReadDir(maxDarwinDirectoryEntries + 1)
	if err != nil && !errors.Is(err, io.EOF) {
		return nil, false, err
	}
	complete := true
	if len(entries) > maxDarwinDirectoryEntries {
		entries = entries[:maxDarwinDirectoryEntries]
		complete = false
	}
	sort.Slice(entries, func(i, j int) bool {
		return entries[i].Name() < entries[j].Name()
	})
	return entries, complete, nil
}

// countDarwinFilesExcludingDirectory returns the persisted file-record budget already in use elsewhere.
func countDarwinFilesExcludingDirectory(state *darwinBookmarkState, excludedKey string) int {
	total := 0
	for key, dirState := range state.Directories {
		if key == excludedKey || dirState == nil {
			continue
		}
		total += len(dirState.Files)
	}
	return total
}
