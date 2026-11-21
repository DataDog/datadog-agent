// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package file provides file-based log launchers
package file

import (
	"context"
	"errors"
	"os"
	"regexp"
	"slices"
	"sync"
	"sync/atomic"
	"time"

	tagger "github.com/DataDog/datadog-agent/comp/core/tagger/def"
	"github.com/DataDog/datadog-agent/comp/logs-library/metrics"
	"github.com/DataDog/datadog-agent/comp/logs-library/pipeline"
	"github.com/DataDog/datadog-agent/comp/logs-library/routing"
	"github.com/DataDog/datadog-agent/comp/logs/agent/config"
	flareController "github.com/DataDog/datadog-agent/comp/logs/agent/flare"
	auditor "github.com/DataDog/datadog-agent/comp/logs/auditor/def"
	pkgconfigsetup "github.com/DataDog/datadog-agent/pkg/config/setup"
	"github.com/DataDog/datadog-agent/pkg/logs/internal/decoder"
	"github.com/DataDog/datadog-agent/pkg/logs/launchers"
	fileprovider "github.com/DataDog/datadog-agent/pkg/logs/launchers/file/provider"
	"github.com/DataDog/datadog-agent/pkg/logs/message"
	"github.com/DataDog/datadog-agent/pkg/logs/sources"
	status "github.com/DataDog/datadog-agent/pkg/logs/status/utils"
	"github.com/DataDog/datadog-agent/pkg/logs/tailers"
	tailer "github.com/DataDog/datadog-agent/pkg/logs/tailers/file"
	"github.com/DataDog/datadog-agent/pkg/logs/types"
	"github.com/DataDog/datadog-agent/pkg/logs/util/opener"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/datadog-agent/pkg/util/procfilestats"
	"github.com/DataDog/datadog-agent/pkg/util/startstop"
)

// DefaultSleepDuration represents the amount of time the tailer waits before reading new data when no data is received
const DefaultSleepDuration = 1 * time.Second

// routedSourceProvider is the narrow optional interface implemented by
// *pipeline.RoutedProvider for per-source isolated channels, producer
// generation retirement, and direct checkpoint write guarding.
type routedSourceProvider interface {
	SourceInput(identifier string, generation uint64) (chan *message.Message, error)
	RetireSource(ctx context.Context, identifier string, generation uint64) error
	GuardRegistry(registry routing.Registry) routing.Registry
}

type sourceGeneration struct {
	identifier string
	generation uint64
}

// routedRotationAbsentScans is the number of consecutive under-limit scans
// where a pending rotated path must report os.ErrNotExist (with no draining
// old tailer remaining) before being pruned as permanently deleted.
const routedRotationAbsentScans = 3

type routedRotationEntry struct {
	info        *oldTailerInfo
	source      *sources.LogSource
	path        string
	missedScans int
}

// Launcher checks all files provided by fileProvider and create new tailers
// or update the old ones if needed
type Launcher struct {
	pipelineProvider    pipeline.Provider
	addedSources        chan *sources.LogSource
	removedSources      chan *sources.LogSource
	activeSources       []*sources.LogSource
	addedSourcesDone    chan struct{}
	removedSourcesDone  chan struct{}
	tailingLimit        int
	fileProvider        *fileprovider.FileProvider
	tailers             *tailers.TailerContainer[*tailer.Tailer]
	rotatedTailers      []*tailer.Tailer
	registry            auditor.Registry
	tailerSleepDuration time.Duration
	stop                chan struct{}
	done                chan struct{}
	// set to true if we want to use `ContainersLogsDir` to validate that a new
	// pod log file is being attached to the correct containerID.
	// Feature flag defaulting to false, use `logs_config.validate_pod_container_id`.
	validatePodContainerID  bool
	scanPeriod              time.Duration
	flarecontroller         *flareController.FlareController
	tagger                  tagger.Component
	filesChan               chan []*tailer.File
	filesTailedBetweenScans []*tailer.File
	// Scan keys of files that started being skipped for an unusable fingerprint while a scan was in
	// flight. Serves the same purpose as filesTailedBetweenScans, for files that got no tailer, and
	// is cleared on the same schedule. See resolveActiveTailers.
	filesSkippedBetweenScans []string
	// Stores pertinent information about old tailer when rotation occurs and fingerprinting isn't possible
	oldInfoMap map[string]*oldTailerInfo
	// Tracks the files we currently refuse to tail because their fingerprint is unusable, keyed by
	// scan key. Only used to log the transitions instead of one line per scan, see recordFingerprintSkip.
	fingerprintSkips map[string]*fingerprintSkip
	fileOpener       opener.FileOpener
	fingerprinter    tailer.Fingerprinter
	// Keep the exact message recipients: a tailer's ReplaceableSource can change before cleanup.
	fingerprintRotationErrors map[string]*config.Messages
	stopOnce                  sync.Once

	// Per-tailer generation ownership for routedSourceProvider.
	routedProvider        routedSourceProvider
	nextGeneration        atomic.Uint64
	routingMu             sync.Mutex
	tailerGenerations     map[*tailer.Tailer]sourceGeneration
	pendingRetirements    []sourceGeneration
	stoppingTailers       sync.WaitGroup
	routedRotationPending map[string]routedRotationEntry
}

const (
	// WildcardModeByName is the default mode and prioritizes files by name in reverse order
	WildcardModeByName string = "by_name"
	// WildcardModeByModificationTime prioritizes files by modification time
	WildcardModeByModificationTime string = "by_modification_time"
)

type oldTailerInfo struct {
	Pattern      *regexp.Regexp
	InfoRegistry *status.InfoRegistry
}

// NewLauncher returns a new launcher.
func NewLauncher(
	tailingLimit int,
	tailerSleepDuration time.Duration,
	validatePodContainerID bool,
	scanPeriod time.Duration,
	wildcardMode string,
	flarecontroller *flareController.FlareController,
	tagger tagger.Component,
	fileOpener opener.FileOpener,
	fingerprinter tailer.Fingerprinter,
) *Launcher {

	var wildcardStrategy fileprovider.WildcardSelectionStrategy
	switch wildcardMode {
	case WildcardModeByModificationTime:
		wildcardStrategy = fileprovider.WildcardUseFileModTime
	case WildcardModeByName:
		wildcardStrategy = fileprovider.WildcardUseFileName
	default:
		log.Warnf("Unknown wildcard mode specified: %q, defaulting to 'by_name' strategy.", wildcardMode)
		wildcardStrategy = fileprovider.WildcardUseFileName
	}

	return &Launcher{
		addedSourcesDone:       make(chan struct{}),
		removedSourcesDone:     make(chan struct{}),
		tailingLimit:           tailingLimit,
		fileProvider:           fileprovider.NewFileProvider(tailingLimit, wildcardStrategy),
		tailers:                tailers.NewTailerContainer[*tailer.Tailer](),
		rotatedTailers:         []*tailer.Tailer{},
		tailerSleepDuration:    tailerSleepDuration,
		stop:                   make(chan struct{}),
		done:                   make(chan struct{}),
		validatePodContainerID: validatePodContainerID,
		scanPeriod:             scanPeriod,
		flarecontroller:        flarecontroller,
		tagger:                 tagger,
		filesChan:              make(chan []*tailer.File, 1),
		oldInfoMap:             make(map[string]*oldTailerInfo),
		fingerprintSkips:       make(map[string]*fingerprintSkip),
		fileOpener:             fileOpener,
		fingerprinter:          fingerprinter,
		tailerGenerations:      make(map[*tailer.Tailer]sourceGeneration),
	}
}

// Start starts the Launcher
func (s *Launcher) Start(sourceProvider launchers.SourceProvider, pipelineProvider pipeline.Provider, registry auditor.Registry, tracker *tailers.TailerTracker) {
	s.pipelineProvider = pipelineProvider
	s.addedSources, s.removedSources = sourceProvider.SubscribeForType(config.FileType, s.addedSourcesDone, s.removedSourcesDone)
	s.registry = registry
	if routed, ok := pipelineProvider.(routedSourceProvider); ok {
		s.routedProvider = routed
		if registry != nil {
			s.registry = routed.GuardRegistry(registry)
		}
	}
	tracker.Add(s.tailers)
	go s.run()
}

// Stop stops the Scanner and its tailers in parallel,
// this call returns only when all the tailers are stopped
func (s *Launcher) Stop() {
	s.stopOnce.Do(func() {
		// stop the launcher source subscriptions from blocking
		close(s.addedSourcesDone)
		close(s.removedSourcesDone)

		s.stop <- struct{}{}
		<-s.done
	})
}

// run checks periodically if there are new files to tail and the state of its tailers until stop
func (s *Launcher) run() {
	scanTicker := time.NewTicker(s.scanPeriod)
	var scannedSources []*sources.LogSource
	defer func() {
		scanTicker.Stop()
		close(s.done)
	}()

	ctx, cancel := context.WithCancel(context.Background())
	for {
		select {
		case source := <-s.addedSources:
			s.addSource(source)
		case source := <-s.removedSources:
			s.removeSource(source)
		case <-scanTicker.C:

			scannedSources = slices.Clone(s.activeSources)

			// Clear the between-scans state before starting a new FilesToTail: the copy of
			// activeSources it receives already covers the sources these files came from, so its
			// result will mention them on its own.
			s.filesTailedBetweenScans = s.filesTailedBetweenScans[:0]
			s.filesSkippedBetweenScans = s.filesSkippedBetweenScans[:0]

			scanTicker.Stop()
			go func(inputSources []*sources.LogSource) {
				s.filesChan <- s.fileProvider.FilesToTail(ctx, s.validatePodContainerID, inputSources, s.registry)
			}(scannedSources)
		case files := <-s.filesChan:
			s.resolveScan(files)
			scanTicker.Reset(s.scanPeriod)
		case <-s.stop:
			// Cancel the context passed to fileProvider.FilesToTail
			cancel()
			// no more file should be tailed
			s.cleanup()
			return
		}
	}
}

func (s *Launcher) resolveScan(files []*tailer.File) {
	s.cleanUpRotatedTailers()
	s.clearFingerprintRotationErrors()
	s.resolveActiveTailers(files)
}

// cleanup all tailers
func (s *Launcher) cleanup() {
	s.clearFingerprintRotationErrors()
	stopper := startstop.NewParallelStopper()
	s.cleanUpRotatedTailers()
	var stoppedTailers []*tailer.Tailer
	for _, t := range s.rotatedTailers {
		stoppedTailers = append(stoppedTailers, t)
		stopper.Add(t)
	}
	s.rotatedTailers = []*tailer.Tailer{}
	for _, t := range s.tailers.All() {
		stoppedTailers = append(stoppedTailers, t)
		stopper.Add(t)
		s.tailers.Remove(t)
	}
	stopper.Stop()
	s.stoppingTailers.Wait()
	for _, t := range stoppedTailers {
		s.retireTailer(t)
	}
	s.flushPendingRetirements(time.Second)

	// Clean up old info map to prevent memory leaks
	s.oldInfoMap = make(map[string]*oldTailerInfo)
	s.routedRotationPending = nil
	if stillSkipped := len(s.fingerprintSkips); stillSkipped > 0 {
		log.Warnf("Stopping with %d file(s) still not tailed because their fingerprint was unusable.", stillSkipped)
		for _, skip := range s.fingerprintSkips {
			removeFingerprintSkipMessage(skip.file)
		}
		s.fingerprintSkips = make(map[string]*fingerprintSkip)
	}
}

// resolveActiveTailers checks all the files we're expected to tail, compares them to the
// currently tailed files, and triggers the required updates.  For instance,
// when a file is logrotated, its tailer will keep tailing the rotated file.
// The Scanner needs to stop that previous tailer, and start a new one for the
// new file.
func (s *Launcher) resolveActiveTailers(files []*tailer.File) {
	// resolveActiveTailers() receives the files parameter from FilesToTail(),
	// which is called in the main run loop of launcher. FilesToTail() is always
	// executed concurrently.  It is therefore possible that addSource() can be
	// called while FilesToTail() is still running. Since FilesToTail() is only
	// passed a copy of activeSources it is possible that it would miss new
	// sources added by addsource() therefore scan() would unschedule a tailer
	// added during a concurrent scan.  In order to mitigate that possibility, any
	// tailers started while FilesToTail() is running need to be merged with the
	// result of FilesToTail() to prevent scan() from unscheudling them.
	// FilesToTail returns at most tailingLimit files, so a result that reached the limit may have
	// left out files that are still matched. Read before the merge below, which can push the count
	// past the limit on its own.
	hitFileLimit := len(files) >= s.tailingLimit

	files = append(files, s.filesTailedBetweenScans...)
	s.filesTailedBetweenScans = s.filesTailedBetweenScans[:0]
	filesTailed := make(map[string]bool)

	// Every file we are expected to tail during this scan, used to expire skip state for files that
	// have gone away. Only worth building when there is skip state to expire, which is the
	// exception: otherwise it costs a map insert per file on every scan and is never read. A file
	// that starts being skipped later in this call is simply expired by the next scan instead.
	expireSkips := len(s.fingerprintSkips) > 0
	var filesExpected map[string]bool
	if expireSkips {
		filesExpected = make(map[string]bool, len(files)+len(s.filesSkippedBetweenScans))
		// A scan already in flight cannot mention files whose source was added after it took its
		// copy of activeSources, so they would look like they had vanished. This is the same
		// mitigation the files slice gets above, for files that never got a tailer.
		for _, scanKey := range s.filesSkippedBetweenScans {
			filesExpected[scanKey] = true
		}
	}
	s.filesSkippedBetweenScans = s.filesSkippedBetweenScans[:0]
	var allFiles []string

	log.Debugf("Scan - got %d files from FilesToTail and currently tailing %d files\n", len(files), s.tailers.Count())

	// Pass 1 - Compare 'files' to our current set of tailed files. If any no longer need to be tailed,
	// stop the tailers.
	// Defer creation of new tailers until second pass.
	for _, file := range files {
		allFiles = append(allFiles, file.Path)
		// We're using generated key here: in case this file has been found while
		// scanning files for container, the key will use the format:
		//   <filepath>/<containerID>
		// If it has been found while scanning for a regular integration config,
		// its format will be:
		//   <filepath>
		// It is a hack to let two tailers tail the same file (it's happening
		// when a tailer for a dead container is still tailing the file, and another
		// tailer is tailing the file for the new container).
		scanKey := file.GetScanKey()
		if expireSkips {
			filesExpected[scanKey] = true
		}
		tailered, isTailed := s.tailers.Get(scanKey)
		if isTailed && tailered.IsFinished() {
			// skip this tailer as it must be stopped
			log.Debugf("Tailer for %s has finished, it will be stopped and a new one started for this file", file.Path)
			continue
		}

		// If the file is currently being tailed, check for rotation and handle it appropriately.
		if isTailed {
			var didRotate bool
			var err error

			// The rotation check reads through the tailer's own file, so whether
			// to fingerprint at all and which source to report a failure on are
			// both decided from that file rather than the freshly scanned one.
			tailedFile := tailered.File()
			if s.fingerprinter.ShouldFileFingerprint(tailedFile) {
				didRotate, err = tailered.DidRotateViaFingerprint(s.fingerprinter)
				if err != nil {
					s.recordFingerprintRotationError(tailedFile, err)
					didRotate = false
				}
				if didRotate {
					s.rotateTailerWithoutRestart(tailered, file)
					continue
				}
			} else {
				didRotate, err = tailered.DidRotate()

				if err != nil {
					log.Debugf("failed to detect log rotation: %v", err)
					continue
				}
				if didRotate {
					// restart tailer because of file-rotation on file
					succeeded := s.restartTailerAfterFileRotation(tailered, file)
					if !succeeded {
						// the setup failed, let's try to tail this file in the next scan
						continue
					}
				}
			}
		} else {
			// Defer any files that are not tailed for the 2nd pass
			continue
		}

		filesTailed[scanKey] = true
	}

	s.flarecontroller.SetAllFiles(allFiles)

	for _, tailer := range s.tailers.All() {
		// stop all tailers which have not been selected
		_, shouldTail := filesTailed[tailer.GetID()]
		if !shouldTail {
			s.stopTailer(tailer)
		}
	}

	tailersLen := s.tailers.Count()
	log.Debugf("After stopping tailers, there are %d tailers running.\n", tailersLen)

	lastIterationOldInfo := s.oldInfoMap
	s.oldInfoMap = make(map[string]*oldTailerInfo)
	unconsumedOldInfo := len(lastIterationOldInfo)
	// Pass 2 - Create new tailers for files that need to be tailed
	for _, file := range files {
		scanKey := file.GetScanKey()
		_, isTailed := s.tailers.Get(scanKey)
		if isTailed {
			if _, ok := lastIterationOldInfo[scanKey]; ok {
				unconsumedOldInfo--
			}
			filesTailed[scanKey] = true
			continue
		}

		// Check if we have stored info for this file from a previous rotation
		oldInfo, hasOldInfo := lastIterationOldInfo[scanKey]
		if hasOldInfo {
			unconsumedOldInfo--
		}
		fromRoutedPending := false
		if !hasOldInfo && s.routedProvider != nil {
			if pendingEntry, ok := s.routedRotationPending[scanKey]; ok {
				oldInfo = pendingEntry.info
				hasOldInfo = true
				fromRoutedPending = true
			}
		}

		fingerprint, ok := s.resolveFingerprint(file)
		if !ok {
			// Persist the old info back into the map for future attempts
			if hasOldInfo && !fromRoutedPending {
				s.oldInfoMap[scanKey] = oldInfo
			}
			continue
		}

		if hasOldInfo {
			if s.startNewTailerWithStoredInfo(file, config.ForceBeginning, oldInfo, fingerprint) {
				// hasOldInfo is true when restarting a tailer after a file rotation, so start tailer from the beginning
				filesTailed[scanKey] = true
				s.clearRoutedRotationPending(scanKey)
			} else if !fromRoutedPending {
				// Bug fix: the tailer failed to start, so re-persist the stored info
				// for the next scan instead of dropping it. resolveFingerprint already
				// records/clears the fingerprint-skip note, so no resolveFingerprintSkip here.
				s.oldInfoMap[scanKey] = oldInfo
			}
		} else {
			if s.routedProvider != nil && !s.canAdmitNewRoutedTailer(unconsumedOldInfo) {
				continue
			}
			// Normal case - no stored info
			if s.startNewTailer(file, config.Beginning, fingerprint) {
				filesTailed[scanKey] = true
				s.resolveFingerprintSkip(file)
			}
		}
	}
	if s.routedProvider != nil && len(s.routedRotationPending) > 0 {
		s.reconcileRoutedRotationPending(filesExpected, hitFileLimit)
	}
	log.Debugf("After starting new tailers, there are %d tailers running. Limit is %d.\n", s.tailers.Count(), s.tailingLimit)

	// Files that are no longer expected to be tailed can't recover, so drop their skip state.
	if expireSkips {
		s.forgetVanishedFiles(filesExpected, hitFileLimit)
	}

	// Check how many file handles the Agent process has open and log a warning if the process is coming close to the OS file limit
	fileStats, err := procfilestats.GetProcessFileStats()
	if err == nil {
		CheckProcessTelemetry(fileStats)
	} else {
		log.Debugf("Could not read the Agent process file statistics, skipping the open file limit check: %v", err)
	}
}

// resolveFingerprint returns the fingerprint to hand to a new tailer for file,
// and false when the file cannot be scheduled and must be retried on a later
// scan. A file that is not fingerprinted still gets its effective config so the
// status page can display it.
func (s *Launcher) resolveFingerprint(file *tailer.File) (*types.Fingerprint, bool) {
	// Computing a fingerprint is itself an open on the path, so the sequential
	// handoff barrier has to come before it.
	if s.drainingOnPath(file) {
		log.Debugf("Waiting for the rotated tailer on %s to finish before reopening it", file.Path)
		return nil, false
	}

	if !s.fingerprinter.ShouldFileFingerprint(file) {
		fpConfig := s.fingerprinter.GetEffectiveConfigForFile(file)
		if fpConfig == nil {
			return nil, true
		}
		return &types.Fingerprint{Value: types.InvalidFingerprintValue, Config: fpConfig}, true
	}

	fingerprint, err := s.fingerprinter.ComputeFingerprint(file)
	if err != nil || fingerprint == nil || !fingerprint.ValidFingerprint() {
		s.recordFingerprintSkip(file, fingerprint, err)
		return nil, false
	}
	// Fingerprint is usable again: clear any skip note opened on a previous scan.
	s.resolveFingerprintSkip(file)
	return fingerprint, true
}

// sequentialHandoffActive reports whether a fingerprint-detected rotation hands the
// path over (StopAfterFileRotationForHandoff) rather than draining alongside the
// replacement. The barrier itself keys on Tailer.IsHandoffDrain, not on this. The
// unreliable-mount profile is the configuration for mounts that reuse handles
// across a rotation.
//
// This deliberately asks whether the profile is enabled, not whether direct reads
// are honoured on this platform: O_DIRECT is Linux-only, but gating the handoff on
// that would give the same configuration different rotation semantics per platform.
func (s *Launcher) sequentialHandoffActive() bool {
	return config.UnreliableMountEnabled(pkgconfigsetup.Datadog())
}

// drainingOnPath reports whether a rotated tailer on this file's path is still
// draining for a sequential handoff. Tailer.Identifier() is "file:"+path, so two
// tailers on one path -- for example a dead and a fresh container -- compare equal
// here, which is what the barrier needs. Tailers rotated through the parallel
// restart path do not block.
func (s *Launcher) drainingOnPath(file *tailer.File) bool {
	id := file.Identifier()
	return slices.ContainsFunc(s.rotatedTailers, func(t *tailer.Tailer) bool {
		return t.IsHandoffDrain() && t.Identifier() == id && !t.IsFinished()
	})
}

// cleanUpRotatedTailers removes any rotated tailers that have stopped from the list
func (s *Launcher) cleanUpRotatedTailers() {
	pendingTailers := []*tailer.Tailer{}
	for _, tailer := range s.rotatedTailers {
		if !tailer.IsFinished() {
			pendingTailers = append(pendingTailers, tailer)
		} else {
			s.retireTailer(tailer)
		}
	}
	s.rotatedTailers = pendingTailers
	s.flushPendingRetirements(50 * time.Millisecond)
}

func (s *Launcher) acquireSourceInput(file *tailer.File) (chan *message.Message, *metrics.CapacityMonitor, uint64, error) {
	if s.routedProvider != nil {
		gen := s.nextGeneration.Add(1)
		ch, err := s.routedProvider.SourceInput(file.Identifier(), gen)
		if err != nil {
			return nil, nil, 0, err
		}
		return ch, nil, gen, nil
	}
	ch, monitor := s.pipelineProvider.NextPipelineChanWithMonitor()
	return ch, monitor, 0, nil
}

func (s *Launcher) bindTailerGeneration(t *tailer.Tailer, identifier string, gen uint64) {
	if s.routedProvider == nil || gen == 0 {
		return
	}
	s.routingMu.Lock()
	if s.tailerGenerations == nil {
		s.tailerGenerations = make(map[*tailer.Tailer]sourceGeneration)
	}
	s.tailerGenerations[t] = sourceGeneration{identifier: identifier, generation: gen}
	s.routingMu.Unlock()
}

func (s *Launcher) retireTailer(t *tailer.Tailer) {
	if s.routedProvider == nil {
		return
	}
	s.routingMu.Lock()
	sg, ok := s.tailerGenerations[t]
	if ok {
		delete(s.tailerGenerations, t)
	}
	s.routingMu.Unlock()
	if ok {
		s.retireGeneration(sg.identifier, sg.generation)
	}
}

func (s *Launcher) retireGeneration(identifier string, gen uint64) {
	if s.routedProvider == nil || gen == 0 {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	err := s.routedProvider.RetireSource(ctx, identifier, gen)
	cancel()
	if err == nil || err == routing.ErrStopped || err == routing.ErrUnknownGeneration {
		return
	}
	s.routingMu.Lock()
	s.pendingRetirements = append(s.pendingRetirements, sourceGeneration{identifier: identifier, generation: gen})
	s.routingMu.Unlock()
}

func (s *Launcher) flushPendingRetirements(timeout time.Duration) {
	if s.routedProvider == nil {
		return
	}
	s.routingMu.Lock()
	if len(s.pendingRetirements) == 0 {
		s.routingMu.Unlock()
		return
	}
	pending := s.pendingRetirements
	s.pendingRetirements = nil
	s.routingMu.Unlock()

	var remaining []sourceGeneration
	for _, sg := range pending {
		ctx, cancel := context.WithTimeout(context.Background(), timeout)
		err := s.routedProvider.RetireSource(ctx, sg.identifier, sg.generation)
		cancel()
		if err != nil && err != routing.ErrStopped && err != routing.ErrUnknownGeneration {
			remaining = append(remaining, sg)
		}
	}
	if len(remaining) > 0 {
		s.routingMu.Lock()
		s.pendingRetirements = append(s.pendingRetirements, remaining...)
		s.routingMu.Unlock()
	}
}

// addSource keeps track of the new source and launch new tailers for this source.
func (s *Launcher) addSource(source *sources.LogSource) {
	s.activeSources = append(s.activeSources, source)
	s.launchTailers(source)
}

// removeSource removes the source from cache.
func (s *Launcher) removeSource(source *sources.LogSource) {
	for i, src := range s.activeSources {
		if src == source {
			// no need to stop the tailer here, it will be stopped in the next iteration of scan.
			s.activeSources = slices.Delete(s.activeSources, i, i+1)
			break
		}
	}
}

// launch launches new tailers for a new source.
func (s *Launcher) launchTailers(source *sources.LogSource) {
	// If we're at the limit already, no need to do a 'CollectFiles', just wait for the next 'scan'
	if s.tailers.Count() >= s.tailingLimit {
		log.Debugf("Not launching tailers for %s: already tailing %d files, which is the limit (logs_config.open_files_limit). Files for this source are picked up by the next scan.",
			source.Config.Path, s.tailingLimit)
		return
	}
	files, err := s.fileProvider.CollectFiles(source)
	if err != nil {
		source.Status().Error(err)
		log.Warnf("Could not collect files: %v", err)
		return
	}

	// Reuse a single resolver so the /var/log/containers scan happens at most once
	// across all files for this source rather than once per file.
	symlinkResolver := fileprovider.NewContainerLogSymlinkResolver()
	for _, file := range files {
		if s.tailers.Count() >= s.tailingLimit {
			log.Debugf("Reached the limit of %d tailed files (logs_config.open_files_limit) while launching tailers for %s, the remaining files are picked up by the next scan.",
				s.tailingLimit, source.Config.Path)
			return
		}

		if fileprovider.ShouldIgnore(s.validatePodContainerID, file, symlinkResolver) {
			continue
		}
		if tailer, isTailed := s.tailers.Get(file.GetScanKey()); isTailed {
			// new source inherits the old source's status
			source.SetStatus(tailer.Source().Status())
			// the file is already tailed, update the existing tailer's source so that the tailer
			// uses this new source going forward
			s.clearFingerprintRotationError(file.GetScanKey())
			tailer.ReplaceSource(source)
			continue
		}

		fingerprint, ok := s.resolveFingerprint(file)
		if !ok {
			// resolveFingerprint already recorded the skip in the agent log and status, or is
			// waiting for a rotated tailer on the same path to finish. A scan
			// running right now was handed a copy of activeSources that predates this source, so its result will
			// not mention this file; without this, that result expires the skip we just
			// opened and we report giving up on a file we are still retrying, then open a
			// second skip on the next scan.
			s.filesSkippedBetweenScans = append(s.filesSkippedBetweenScans, file.GetScanKey())
			continue
		}

		mode, isSet := config.TailingModeFromString(source.GetTailingMode())
		if !isSet && source.Config.Identifier != "" {
			mode = config.Beginning
			source.SetTailingMode(mode.String())
		}

		var newTailerStarted bool
		if s.routedProvider != nil {
			if pendingEntry, hasPending := s.routedRotationPending[file.GetScanKey()]; hasPending {
				newTailerStarted = s.startNewTailerWithStoredInfo(file, config.ForceBeginning, pendingEntry.info, fingerprint)
				if newTailerStarted {
					s.clearRoutedRotationPending(file.GetScanKey())
				}
			} else if s.canAdmitNewRoutedTailer(0) {
				newTailerStarted = s.startNewTailer(file, mode, fingerprint)
			}
		} else {
			newTailerStarted = s.startNewTailer(file, mode, fingerprint)
		}
		if newTailerStarted {
			s.resolveFingerprintSkip(file)
			s.filesTailedBetweenScans = append(s.filesTailedBetweenScans, file)
		}
	}
}

func (s *Launcher) positionFileOpener(file *tailer.File) opener.FileOpener {
	if file == nil {
		return s.fileOpener
	}
	return opener.ForSource(s.fileOpener, file.Source)
}

// tailerPosition uses the fallback position when the saved offset cannot be recovered.
func (s *Launcher) tailerPosition(file *tailer.File, t *tailer.Tailer, m config.TailingMode, fingerprint *types.Fingerprint) (int64, int) {
	mode := s.handleTailingModeChange(t.Identifier(), m)

	offset, whence, err := Position(s.registry, t.Identifier(), mode, s.fingerprinter, s.positionFileOpener(file), fingerprint)
	if err != nil {
		log.Warnf("Could not recover offset for file with path %v: %v", file.Path, err)
	}
	return offset, whence
}

// startNewTailer creates a new tailer, making it tail from the last committed offset, the beginning or the end of the file,
// returns true if the operation succeeded, false otherwise.
func (s *Launcher) startNewTailer(file *tailer.File, m config.TailingMode, fingerprint *types.Fingerprint) bool {
	if file == nil {
		log.Debug("startNewTailer called with a nil file")
		return false
	}

	channel, monitor, gen, err := s.acquireSourceInput(file)
	if err != nil {
		log.Warnf("Could not acquire routed pipeline input for %s, will retry on next scan: %v", file.Path, err)
		return false
	}
	tailer := s.createTailer(file, channel, monitor, fingerprint)

	offset, whence := s.tailerPosition(file, tailer, m, fingerprint)

	log.Infof("Starting a new tailer for: %s (offset: %d, whence: %d) for tailer key %s", file.Path, offset, whence, file.GetScanKey())
	if err := tailer.Start(offset, whence); err != nil {
		s.retireGeneration(file.Identifier(), gen)
		log.Warnf("Could not start the tailer for %s, this file is not being tailed and will be retried on the next scan: %v", file.Path, err)
		return false
	}

	s.bindTailerGeneration(tailer, file.Identifier(), gen)
	s.tailers.Add(tailer)
	return true
}

// startNewTailerWithStoredInfo creates a new tailer using stored info from previous rotation
func (s *Launcher) startNewTailerWithStoredInfo(file *tailer.File, m config.TailingMode, oldInfo *oldTailerInfo, fingerprint *types.Fingerprint) bool {
	if file == nil {
		log.Debug("startNewTailerWithStoredInfo called with a nil file")
		return false
	}

	channel, monitor, gen, err := s.acquireSourceInput(file)
	if err != nil {
		log.Warnf("Could not acquire routed pipeline input for %s after rotation, will retry on next scan: %v", file.Path, err)
		return false
	}

	// Use stored InfoRegistry if available, otherwise create new one
	var tailerInfo *status.InfoRegistry
	if oldInfo.InfoRegistry != nil {
		tailerInfo = oldInfo.InfoRegistry
	} else {
		tailerInfo = status.NewInfoRegistry()
	}

	// Create decoder with stored pattern if available
	var decoderInstance decoder.Decoder
	if oldInfo.Pattern != nil {
		decoderInstance = decoder.NewDecoderFromSourceWithPattern(file.Source, oldInfo.Pattern, tailerInfo)
	} else {
		decoderInstance = decoder.NewDecoderFromSource(file.Source, tailerInfo)
	}

	tailerOptions := &tailer.TailerOptions{
		OutputChan:      channel,
		File:            file,
		SleepDuration:   s.tailerSleepDuration,
		Decoder:         decoderInstance,
		Info:            tailerInfo,
		TagAdder:        s.tagger,
		CapacityMonitor: monitor,
		Registry:        s.registry,
		Fingerprint:     fingerprint,
		Fingerprinter:   s.fingerprinter,
		Rotated:         true,
		FileOpener:      s.fileOpener,
	}

	if fingerprint != nil {
		log.Debugf("Creating new tailer for %s with fingerprint 0x%x", file.Path, fingerprint.Value)
	} else {
		log.Debugf("Creating new tailer for %s with no fingerprint", file.Path)
	}

	tailer := tailer.NewTailer(tailerOptions)
	addFingerprintConfigToTailerInfo(tailer)

	offset, whence := s.tailerPosition(file, tailer, m, fingerprint)

	log.Infof("Starting new tailer with stored info (pattern: %v) for: %s (offset: %d, whence: %d)",
		oldInfo.Pattern != nil, file.Path, offset, whence)
	if err := tailer.Start(offset, whence); err != nil {
		s.retireGeneration(file.Identifier(), gen)
		log.Warnf("Could not start the tailer for %s after a log rotation, this file is not being tailed and will be retried on the next scan: %v", file.Path, err)
		return false
	}

	s.bindTailerGeneration(tailer, file.Identifier(), gen)
	s.tailers.Add(tailer)
	return true
}

// handleTailingModeChange determines the tailing behaviour when the tailing mode for a given file has its
// configuration change. Two case may happen we can switch from "end" to "beginning" (1) and from "beginning" to
// "end" (2). If the tailing mode is set to forceEnd or forceBeginning it will remain unchanged.
// If (1) then the resulting tailing mode if "beginning" in order to honor existing offset to avoid duplicated lines to be sent.
// If (2) then the resulting tailing mode is "forceEnd" to drop any saved offset and tail from the end of the file.
func (s *Launcher) handleTailingModeChange(tailerID string, currentTailingMode config.TailingMode) config.TailingMode {
	if currentTailingMode == config.ForceBeginning || currentTailingMode == config.ForceEnd {
		return currentTailingMode
	}
	previousMode, _ := config.TailingModeFromString(s.registry.GetTailingMode(tailerID))
	if previousMode != currentTailingMode {
		log.Infof("Tailing mode changed for %v. Was: %v: Now: %v", tailerID, previousMode, currentTailingMode)
		if currentTailingMode == config.Beginning {
			// end -> beginning, the offset will be honored if it exists
			return config.Beginning
		}
		// beginning -> end, the offset will be ignored
		return config.ForceEnd
	}
	return currentTailingMode
}

// stopTailer stops the tailer
func (s *Launcher) stopTailer(tailer *tailer.Tailer) {
	s.clearFingerprintRotationError(tailer.GetID())
	s.tailers.Remove(tailer)
	s.stoppingTailers.Add(1)
	go func() {
		defer s.stoppingTailers.Done()
		tailer.Stop()
		s.retireTailer(tailer)
	}()
}

func (s *Launcher) rotateTailerWithoutRestart(oldTailer *tailer.Tailer, file *tailer.File) bool {
	log.Info("Log rotation happened to ", file.Path)
	if s.sequentialHandoffActive() {
		oldTailer.StopAfterFileRotationForHandoff()
	} else {
		oldTailer.StopAfterFileRotation()
	}

	// Remove the draining tailer from the active map; it will keep draining via rotatedTailers.
	s.tailers.Remove(oldTailer)

	oldRegexPattern := oldTailer.GetDetectedPattern()
	oldInfoRegistry := oldTailer.GetInfo()

	// Only store info if we're using checksum fingerprinting (where it will be retrieved)
	if oldRegexPattern != nil || oldInfoRegistry != nil {
		regexAndRegistry := &oldTailerInfo{
			InfoRegistry: oldInfoRegistry,
			Pattern:      oldRegexPattern,
		}
		s.oldInfoMap[file.GetScanKey()] = regexAndRegistry
	}

	s.rotatedTailers = append(s.rotatedTailers, oldTailer)

	return false // Will return false regardless and we will let scan() handle it
}

func (s *Launcher) maxRoutedRotationPending() int {
	return max(s.tailingLimit*2, 256)
}

func (s *Launcher) pruneRoutedRotationPending() {
	if len(s.routedRotationPending) == 0 {
		return
	}
	for k, entry := range s.routedRotationPending {
		if _, tailed := s.tailers.Get(k); tailed {
			delete(s.routedRotationPending, k)
			continue
		}
		if entry.source != nil && !s.isSourceActive(entry.source) {
			delete(s.routedRotationPending, k)
		}
	}
}

// canAdmitNewRoutedTailer enforces the conservation invariant across both
// non-fingerprint (routedRotationPending) and fingerprint (oldInfoMap) rotations:
// len(routedRotationPending) + len(oldInfoMap) + unconsumedOldInfo + s.tailers.Count() <= maxRoutedRotationPending().
func (s *Launcher) canAdmitNewRoutedTailer(unconsumedOldInfo int) bool {
	if s.routedProvider == nil {
		return true
	}
	maxTotal := s.maxRoutedRotationPending()
	if len(s.routedRotationPending)+len(s.oldInfoMap)+unconsumedOldInfo+s.tailers.Count() < maxTotal {
		return true
	}
	s.pruneRoutedRotationPending()
	return len(s.routedRotationPending)+len(s.oldInfoMap)+unconsumedOldInfo+s.tailers.Count() < maxTotal
}

// reconcileRoutedRotationPending clears actively tailed entries and prunes
// entries whose path has reported os.ErrNotExist across routedRotationAbsentScans
// consecutive under-limit scans (with no draining rotated tailer remaining),
// allowing permanently deleted wildcard paths to free capacity while preserving
// entries across transient rename/create gaps, stat errors, and hitFileLimit scans.
func (s *Launcher) reconcileRoutedRotationPending(filesExpected map[string]bool, hitFileLimit bool) {
	for scanKey, entry := range s.routedRotationPending {
		if _, tailed := s.tailers.Get(scanKey); tailed {
			delete(s.routedRotationPending, scanKey)
			continue
		}
		if entry.source != nil && !s.isSourceActive(entry.source) {
			delete(s.routedRotationPending, scanKey)
			continue
		}
		if filesExpected[scanKey] {
			if entry.missedScans != 0 {
				entry.missedScans = 0
				s.routedRotationPending[scanKey] = entry
			}
			continue
		}
		if hitFileLimit || entry.path == "" {
			continue
		}
		if s.hasRotatedTailer(scanKey) {
			continue
		}
		_, err := os.Stat(entry.path)
		if err == nil {
			if entry.missedScans != 0 {
				entry.missedScans = 0
				s.routedRotationPending[scanKey] = entry
			}
			continue
		}
		if errors.Is(err, os.ErrNotExist) {
			entry.missedScans++
			if entry.missedScans >= routedRotationAbsentScans {
				delete(s.routedRotationPending, scanKey)
			} else {
				s.routedRotationPending[scanKey] = entry
			}
		}
	}
}

func (s *Launcher) hasRotatedTailer(scanKey string) bool {
	return slices.ContainsFunc(s.rotatedTailers, func(t *tailer.Tailer) bool {
		return t.GetID() == scanKey && !t.IsFinished()
	})
}

func (s *Launcher) recordRoutedRotationPending(scanKey string, info *oldTailerInfo, src *sources.LogSource, path string) {
	if s.routedProvider == nil {
		return
	}
	if s.routedRotationPending == nil {
		s.routedRotationPending = make(map[string]routedRotationEntry)
	}
	s.routedRotationPending[scanKey] = routedRotationEntry{info: info, source: src, path: path}
}

func (s *Launcher) clearRoutedRotationPending(scanKey string) {
	if len(s.routedRotationPending) == 0 {
		return
	}
	delete(s.routedRotationPending, scanKey)
}

func (s *Launcher) isSourceActive(src *sources.LogSource) bool {
	if src == nil {
		return false
	}
	return slices.Contains(s.activeSources, src)
}

// restartTailer safely stops tailer and starts a new one
// returns true if the new tailer is up and running, false if an error occurred
func (s *Launcher) restartTailerAfterFileRotation(oldTailer *tailer.Tailer, file *tailer.File) bool {
	log.Info("Log rotation happened to ", file.Path)
	oldTailer.StopAfterFileRotation()

	// Remove the rotated tailer from the active container immediately so:
	// - copytruncate cannot continue reading/checkpointing a regrown inode,
	// - oldTailer drains and retires its generation to release capacity,
	// - open_files_limit is never exceeded, and
	// - the conservation bound guarantees room in routedRotationPending on failure.
	s.tailers.Remove(oldTailer)

	oldRegexPattern := oldTailer.GetDetectedPattern()

	newTailer, gen, err := s.createRotatedTailer(oldTailer, file, oldRegexPattern, nil)
	if err != nil {
		log.Warnf("Could not acquire routed pipeline input for replacement tailer %s after log rotation: %v", file.Path, err)
		s.recordRoutedRotationPending(file.GetScanKey(), &oldTailerInfo{
			InfoRegistry: oldTailer.GetInfo(),
			Pattern:      oldRegexPattern,
		}, file.Source.UnderlyingSource(), file.Path)
		s.rotatedTailers = append(s.rotatedTailers, oldTailer)
		return false
	}
	// force reading file from beginning since it has been log-rotated
	err = newTailer.StartFromBeginning()
	if err != nil {
		s.retireGeneration(file.Identifier(), gen)
		s.recordRoutedRotationPending(file.GetScanKey(), &oldTailerInfo{
			InfoRegistry: oldTailer.GetInfo(),
			Pattern:      oldRegexPattern,
		}, file.Source.UnderlyingSource(), file.Path)
		s.rotatedTailers = append(s.rotatedTailers, oldTailer)
		log.Warnf("Could not start the replacement tailer for %s after a log rotation, this file is not being tailed and will be retried on the next scan: %v", file.Path, err)
		return false
	}

	// Since newTailer and oldTailer share the same ID, tailers.Add will replace the old tailer.
	// We will keep track of the rotated tailer until it is finished.
	s.clearRoutedRotationPending(file.GetScanKey())
	s.bindTailerGeneration(newTailer, file.Identifier(), gen)
	s.rotatedTailers = append(s.rotatedTailers, oldTailer)
	s.tailers.Add(newTailer)

	return true
}

// createTailer returns a new initialized tailer
func (s *Launcher) createTailer(file *tailer.File, outputChan chan *message.Message, capacityMonitor *metrics.CapacityMonitor, fingerprint *types.Fingerprint) *tailer.Tailer {
	tailerInfo := status.NewInfoRegistry()

	tailerOptions := &tailer.TailerOptions{
		OutputChan:      outputChan,
		File:            file,
		SleepDuration:   s.tailerSleepDuration,
		Decoder:         decoder.NewDecoderFromSource(file.Source, tailerInfo),
		Info:            tailerInfo,
		TagAdder:        s.tagger,
		CapacityMonitor: capacityMonitor,
		Registry:        s.registry,
		Fingerprint:     fingerprint,
		Fingerprinter:   s.fingerprinter,
		FileOpener:      s.fileOpener,
	}

	if fingerprint != nil {
		log.Debugf("Creating new tailer for %s with fingerprint 0x%x", file.Path, fingerprint.Value)
	} else {
		log.Debugf("Creating new tailer for %s with no fingerprint", file.Path)
	}

	t := tailer.NewTailer(tailerOptions)
	addFingerprintConfigToTailerInfo(t)
	return t
}

func (s *Launcher) createRotatedTailer(t *tailer.Tailer, file *tailer.File, pattern *regexp.Regexp, fingerprint *types.Fingerprint) (*tailer.Tailer, uint64, error) {
	tailerInfo := t.GetInfo()
	channel, monitor, gen, err := s.acquireSourceInput(file)
	if err != nil {
		return nil, 0, err
	}
	if fingerprint != nil {
		log.Debugf("Creating new tailer for %s with fingerprint 0x%x (configuration: %v)", file.Path, fingerprint.Value, fingerprint.Config)
	} else {
		log.Debugf("Creating new tailer for %s with no fingerprint", file.Path)
	}
	newTailer := t.NewRotatedTailer(file, channel, monitor, decoder.NewDecoderFromSourceWithPattern(file.Source, pattern, tailerInfo), tailerInfo, s.tagger, fingerprint, s.fingerprinter, s.registry)
	addFingerprintConfigToTailerInfo(newTailer)

	return newTailer, gen, nil
}

// addFingerprintConfigToTailerInfo adds fingerprint configuration info to the tailer's status display
// using the configuration actually supplied to the tailer (from its fingerprint)
func addFingerprintConfigToTailerInfo(t *tailer.Tailer) {
	// Pull the actual config from the tailer's fingerprint
	var config *types.FingerprintConfig
	fingerprint := t.GetFingerprint()
	if fingerprint != nil && fingerprint.Config != nil {
		config = fingerprint.Config
	}

	// Always register config info, even if nil - the Info() method handles nil gracefully
	configInfo := tailer.NewFingerprintConfigInfo(config)
	t.GetInfo().Register(configInfo)
}

// CheckProcessTelemetry checks process file statistics and logs warnings about file handle usage
func CheckProcessTelemetry(stats *procfilestats.ProcessFileStats) {
	ratio := float64(stats.AgentOpenFiles) / float64(stats.OsFileLimit)
	if ratio > 0.9 {
		log.Errorf("Agent process has %v files open which is %0.f%% of the OS open file limit (%v). This is over 90%% utilization. This may be preventing log files from being tailed by the Agent and could interfere with the basic functionality of the Agent. OS file limit must be increased.",
			stats.AgentOpenFiles,
			ratio*100,
			stats.OsFileLimit)
	} else if ratio > 0.7 {
		log.Warnf("Agent process has %v files open which is %0.f%% of the OS open file limit (%v). This is over 70%% utilization; consider increasing the OS open file limit.",
			stats.AgentOpenFiles,
			ratio*100,
			stats.OsFileLimit)
	}
	log.Debugf("Agent process has %v files open which is %0.f%% of the OS open file limit (%v).",
		stats.AgentOpenFiles,
		ratio*100,
		stats.OsFileLimit)
}
