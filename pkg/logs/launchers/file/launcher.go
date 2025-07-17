// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//nolint:revive // TODO(AML) Fix revive linter
package file

import (
	"regexp"
	"slices"
	"sync"
	"sync/atomic"
	"time"

	tagger "github.com/DataDog/datadog-agent/comp/core/tagger/def"
	"github.com/DataDog/datadog-agent/comp/logs/agent/config"
	flareController "github.com/DataDog/datadog-agent/comp/logs/agent/flare"
	auditor "github.com/DataDog/datadog-agent/comp/logs/auditor/def"
	pkgconfigsetup "github.com/DataDog/datadog-agent/pkg/config/setup"
	"github.com/DataDog/datadog-agent/pkg/logs/internal/decoder"
	"github.com/DataDog/datadog-agent/pkg/logs/launchers"
	fileprovider "github.com/DataDog/datadog-agent/pkg/logs/launchers/file/provider"
	"github.com/DataDog/datadog-agent/pkg/logs/message"
	"github.com/DataDog/datadog-agent/pkg/logs/metrics"
	"github.com/DataDog/datadog-agent/pkg/logs/pipeline"
	"github.com/DataDog/datadog-agent/pkg/logs/sources"
	status "github.com/DataDog/datadog-agent/pkg/logs/status/utils"
	"github.com/DataDog/datadog-agent/pkg/logs/tailers"
	tailer "github.com/DataDog/datadog-agent/pkg/logs/tailers/file"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/datadog-agent/pkg/util/procfilestats"
	"github.com/DataDog/datadog-agent/pkg/util/startstop"
)

// DefaultSleepDuration represents the amount of time the tailer waits before reading new data when no data is received
const DefaultSleepDuration = 1 * time.Second

// Launcher checks all files provided by fileProvider and create new tailers
// or update the old ones if needed
type Launcher struct {
	pipelineProvider    pipeline.Provider
	addedSources        chan *sources.LogSource
	removedSources      chan *sources.LogSource
	activeSources       []*sources.LogSource
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
	validatePodContainerID bool
	scanPeriod             time.Duration
	flarecontroller        *flareController.FlareController
	tagger                 tagger.Component
	//Stores pertinent information about old tailer when rotation occurs and fingerprinting isn't possible
	oldInfoMap map[string]*oldTailerInfo
	scanCount  int64
	// Scan timing statistics
	scanDurations    []time.Duration
	lastScanDuration time.Duration
	mu               sync.Mutex
}

type oldTailerInfo struct {
	Pattern      *regexp.Regexp
	InfoRegistry *status.InfoRegistry
}

// NewLauncher returns a new launcher.
func NewLauncher(tailingLimit int, tailerSleepDuration time.Duration, validatePodContainerID bool, scanPeriod time.Duration, wildcardMode string, flarecontroller *flareController.FlareController, tagger tagger.Component) *Launcher {

	var wildcardStrategy fileprovider.WildcardSelectionStrategy
	switch wildcardMode {
	case "by_modification_time":
		wildcardStrategy = fileprovider.WildcardUseFileModTime
	case "by_name":
		wildcardStrategy = fileprovider.WildcardUseFileName
	default:
		log.Warnf("Unknown wildcard mode specified: %q, defaulting to 'by_name' strategy.", wildcardMode)
		wildcardStrategy = fileprovider.WildcardUseFileName
	}

	return &Launcher{
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
		oldInfoMap:             make(map[string]*oldTailerInfo),
		scanCount:              0,
	}
}

// Start starts the Launcher
func (s *Launcher) Start(sourceProvider launchers.SourceProvider, pipelineProvider pipeline.Provider, registry auditor.Registry, tracker *tailers.TailerTracker) {
	s.pipelineProvider = pipelineProvider
	s.addedSources, s.removedSources = sourceProvider.SubscribeForType(config.FileType)
	s.registry = registry
	tracker.Add(s.tailers)
	go s.run()
}

// Stop stops the Scanner and its tailers in parallel,
// this call returns only when all the tailers are stopped
func (s *Launcher) Stop() {
	s.stop <- struct{}{}
	<-s.done
	log.Infof("Total scan runs: %d", s.GetScanCount())
}

// run checks periodically if there are new files to tail and the state of its tailers until stop
func (s *Launcher) run() {
	scanTicker := time.NewTicker(s.scanPeriod)
	defer func() {
		scanTicker.Stop()
		close(s.done)
	}()

	log.Debugf("File launcher started with scan period: %v, tailing limit: %d", s.scanPeriod, s.tailingLimit)

	for {
		select {
		case source := <-s.addedSources:
			log.Debugf("Received new source to add: %s", source.Name)
			s.addSource(source)
		case source := <-s.removedSources:
			log.Debugf("Received source to remove: %s", source.Name)
			s.removeSource(source)
		case <-scanTicker.C:
			atomic.AddInt64(&s.scanCount, 1)
			log.Debugf("Starting scan iteration #%d", atomic.LoadInt64(&s.scanCount))
			s.cleanUpRotatedTailers()
			// check if there are new files to tail, tailers to stop and tailer to restart because of file rotation
			s.scan()
		case <-s.stop:
			log.Debugf("Received stop signal, cleaning up launcher")
			// no more file should be tailed
			s.cleanup()
			log.Infof("Total scan runs: %d", s.GetScanCount())
			return
		}
	}
}

// cleanup all tailers
func (s *Launcher) cleanup() {
	log.Debugf("Starting cleanup of all tailers")
	stopper := startstop.NewParallelStopper()
	s.cleanUpRotatedTailers()
	for _, tailer := range s.rotatedTailers {
		log.Debugf("Adding rotated tailer to cleanup: %s", tailer.GetId())
		stopper.Add(tailer)
	}
	s.rotatedTailers = []*tailer.Tailer{}

	for _, tailer := range s.tailers.All() {
		log.Debugf("Adding active tailer to cleanup: %s", tailer.GetId())
		stopper.Add(tailer)
		s.tailers.Remove(tailer)
	}
	stopper.Stop()
	log.Debugf("Cleanup completed")
}

// scan checks all the files we're expected to tail, compares them to the currently tailed files,
// and triggers the required updates.
// For instance, when a file is logrotated, its tailer will keep tailing the rotated file.
// The Scanner needs to stop that previous tailer, and start a new one for the new file.
func (s *Launcher) scan() {
	startTime := time.Now()
	defer func() {
		duration := time.Since(startTime)
		s.mu.Lock()
		s.lastScanDuration = duration

		// Keep track of last 10 scan durations for averaging
		if len(s.scanDurations) >= 10 {
			s.scanDurations = s.scanDurations[1:]
		}
		s.scanDurations = append(s.scanDurations, duration)
		s.mu.Unlock()

		fingerprintStrategy := pkgconfigsetup.Datadog().GetString("logs_config.fingerprint_strategy")

		// Calculate average duration if we have enough samples
		var avgDuration time.Duration
		s.mu.Lock()
		if len(s.scanDurations) > 0 {
			total := time.Duration(0)
			for _, d := range s.scanDurations {
				total += d
			}
			avgDuration = total / time.Duration(len(s.scanDurations))
		}
		s.mu.Unlock()

		log.Infof("Scan completed in %v (avg: %v) using fingerprint strategy: %s",
			duration, avgDuration, fingerprintStrategy)
	}()

	files := s.fileProvider.FilesToTail(s.validatePodContainerID, s.activeSources, s.registry)
	filesTailed := make(map[string]bool)
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
		tailered, isTailed := s.tailers.Get(scanKey)
		if isTailed && tailered.IsFinished() {
			// skip this tailer as it must be stopped
			continue
		}

		// If the file is currently being tailed, check for rotation and handle it appropriately.
		if isTailed {
			var didRotate bool
			var err error
			fingerprintStrategy := pkgconfigsetup.Datadog().GetString("logs_config.fingerprint_strategy")
			if fingerprintStrategy == "checksum" {
				didRotate, err = tailered.DidRotateViaFingerprint()
				if err != nil {
					didRotate = false
				}
			} else {
				didRotate, err = tailered.DidRotate()
			}
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

				// For checksum mode, if new file is undersized, don't mark as "should tail"
				// so the new tailer gets cleaned up, but old tailer continues with 60s grace period
				checkSumEnabled := pkgconfigsetup.Datadog().GetString("logs_config.fingerprint_strategy")
				if checkSumEnabled == "checksum" {
					if tailer.ComputeFingerprint(file.Path, tailer.ReturnFingerprintConfig()) == 0 {
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
		_, shouldTail := filesTailed[tailer.GetId()]
		if !shouldTail {
			s.stopTailer(tailer)
		}
	}

	tailersLen := s.tailers.Count()
	log.Debugf("After stopping tailers, there are %d tailers running.\n", tailersLen)

	for _, file := range files {
		scanKey := file.GetScanKey()
		isTailed := s.tailers.Contains(scanKey)
		if !isTailed && tailersLen < s.tailingLimit {
			checkSumEnabled := pkgconfigsetup.Datadog().GetString("logs_config.fingerprint_strategy")
			log.Debugf("Processing file %s (scanKey: %s, isTailed: %v, tailersLen: %d, limit: %d, checksum: %s)",
				file.Path, scanKey, isTailed, tailersLen, s.tailingLimit, checkSumEnabled)

			if checkSumEnabled == "checksum" {
				log.Debugf("Checksum fingerprinting enabled for file %s", file.Path)

				fingerprint := tailer.ComputeFingerprint(file.Path, tailer.ReturnFingerprintConfig())
				log.Debugf("Computed fingerprint for file %s: 0x%x", file.Path, fingerprint)

				if fingerprint == 0 {
					log.Debugf("Skipping file %s - fingerprint returned 0 (insufficient data)", file.Path)
					continue
				} else {
					log.Debugf("File %s has valid fingerprint: 0x%x, proceeding with tailer creation", file.Path, fingerprint)
				}

				// Check if we have stored info from previous rotation and use it
				var succeeded bool
				if oldInfo, exists := s.oldInfoMap[file.Path]; exists {
					log.Debugf("Using stored info for file %s (pattern: %v)", file.Path, oldInfo.Pattern != nil)
					succeeded = s.startNewTailerWithStoredInfo(file, config.Beginning, oldInfo)
					// Remove from map after use to prevent stale data
					delete(s.oldInfoMap, file.Path)
				} else {
					log.Debugf("No stored info found for file %s, creating new tailer", file.Path)
					succeeded = s.startNewTailer(file, config.Beginning)
				}
				if !succeeded {
					log.Debugf("Failed to start tailer for file %s, will retry in next scan", file.Path)
					// the setup failed, let's try to tail this file in the next scan
					continue
				}
				log.Debugf("Successfully created tailer for file %s", file.Path)
				tailersLen++
				filesTailed[scanKey] = true
				continue
			} else {
				log.Debugf("Checksum fingerprinting disabled for file %s, creating tailer without fingerprint check", file.Path)
				// create a new tailer tailing from the beginning of the file if no offset has been recorded
				succeeded := s.startNewTailer(file, config.Beginning)
				if !succeeded {
					log.Debugf("Failed to start tailer for file %s (non-checksum mode), will retry in next scan", file.Path)
					// the setup failed, let's try to tail this file in the next scan
					continue
				}
				log.Debugf("Successfully created tailer for file %s (non-checksum mode)", file.Path)
				tailersLen++
				filesTailed[scanKey] = true
				continue
			}
		} else {
			if isTailed {
				log.Debugf("File %s is already being tailed, skipping", file.Path)
			} else {
				log.Debugf("File %s skipped - tailersLen (%d) >= limit (%d)", file.Path, tailersLen, s.tailingLimit)
			}
		}
	}
	log.Debugf("After starting new tailers, there are %d tailers running. Limit is %d.\n", tailersLen, s.tailingLimit)

	// Check how many file handles the Agent process has open and log a warning if the process is coming close to the OS file limit
	fileStats, err := procfilestats.GetProcessFileStats()
	if err == nil {
		CheckProcessTelemetry(fileStats)
	}
}

// cleanUpRotatedTailers removes any rotated tailers that have stopped from the list
func (s *Launcher) cleanUpRotatedTailers() {
	log.Debugf("Cleaning up rotated tailers, current count: %d", len(s.rotatedTailers))
	pendingTailers := []*tailer.Tailer{}
	for _, tailer := range s.rotatedTailers {
		if !tailer.IsFinished() {
			log.Debugf("Keeping rotated tailer: %s (not finished)", tailer.GetId())
			pendingTailers = append(pendingTailers, tailer)
		} else {
			log.Debugf("Removing finished rotated tailer: %s", tailer.GetId())
		}
	}
	s.rotatedTailers = pendingTailers
	log.Debugf("Rotated tailers cleanup completed, remaining: %d", len(s.rotatedTailers))
}

// addSource keeps track of the new source and launch new tailers for this source.
func (s *Launcher) addSource(source *sources.LogSource) {
	log.Debugf("Adding source: %s (path: %s)", source.Name, source.Config.Path)
	s.activeSources = append(s.activeSources, source)
	s.launchTailers(source)
	log.Debugf("Active sources count after adding: %d", len(s.activeSources))
}

// removeSource removes the source from cache.
func (s *Launcher) removeSource(source *sources.LogSource) {
	log.Debugf("Removing source: %s", source.Name)
	for i, src := range s.activeSources {
		if src == source {
			log.Debugf("Found source at index %d, removing from active sources", i)
			// no need to stop the tailer here, it will be stopped in the next iteration of scan.
			s.activeSources = slices.Delete(s.activeSources, i, i+1)
			break
		}
	}
	log.Debugf("Active sources count after removing: %d", len(s.activeSources))
}

// launch launches new tailers for a new source.
func (s *Launcher) launchTailers(source *sources.LogSource) {
	log.Debugf("Launching tailers for source: %s", source.Name)
	// If we're at the limit already, no need to do a 'CollectFiles', just wait for the next 'scan'
	if s.tailers.Count() >= s.tailingLimit {
		log.Debugf("Skipping tailer launch - at limit (%d/%d)", s.tailers.Count(), s.tailingLimit)
		return
	}
	files, err := s.fileProvider.CollectFiles(source)
	if err != nil {
		source.Status.Error(err)
		log.Warnf("Could not collect files: %v", err)
		return
	}
	log.Debugf("Collected %d files for source: %s", len(files), source.Name)
	for _, file := range files {
		if s.tailers.Count() >= s.tailingLimit {
			log.Debugf("Reached tailing limit (%d), stopping file collection", s.tailingLimit)
			return
		}

		if fileprovider.ShouldIgnore(s.validatePodContainerID, file) {
			log.Debugf("Skipping ignored file: %s", file.Path)
			continue
		}
		if tailer, isTailed := s.tailers.Get(file.GetScanKey()); isTailed {
			log.Debugf("File already being tailed, updating source: %s", file.Path)
			// new source inherits the old source's status
			source.Status = tailer.Source().Status
			// the file is already tailed, update the existing tailer's source so that the tailer
			// uses this new source going forward
			tailer.ReplaceSource(source)
			continue
		}

		mode, isSet := config.TailingModeFromString(source.Config.TailingMode)
		if !isSet && source.Config.Identifier != "" {
			mode = config.Beginning
			source.Config.TailingMode = mode.String()
			log.Debugf("Set default tailing mode to Beginning for source with identifier: %s", source.Config.Identifier)
		}

		log.Debugf("Starting new tailer for file: %s with mode: %v", file.Path, mode)
		s.startNewTailer(file, mode)
	}
}

// startNewTailer creates a new tailer, making it tail from the last committed offset, the beginning or the end of the file,
// returns true if the operation succeeded, false otherwise.
func (s *Launcher) startNewTailer(file *tailer.File, m config.TailingMode) bool {
	if file == nil {
		log.Debug("startNewTailer called with a nil file")
		return false
	}

	log.Debugf("Creating new tailer for file: %s with mode: %v", file.Path, m)
	channel, monitor := s.pipelineProvider.NextPipelineChanWithMonitor()
	tailer := s.createTailer(file, channel, monitor)

	var offset int64
	var whence int
	mode := s.handleTailingModeChange(tailer.Identifier(), m)
	offset, whence, err := Position(s.registry, tailer.Identifier(), mode)
	if err != nil {
		log.Warnf("Could not recover offset for file with path %v: %v", file.Path, err)
	}

	log.Infof("Starting a new tailer for: %s (offset: %d, whence: %d) for tailer key %s", file.Path, offset, whence, file.GetScanKey())
	err = tailer.Start(offset, whence)
	if err != nil {
		log.Warn(err)
		return false
	}

	s.tailers.Add(tailer)
	log.Debugf("Successfully added tailer for file: %s, total tailers: %d", file.Path, s.tailers.Count())
	return true
}

// startNewTailerWithStoredInfo creates a new tailer using stored info from previous rotation
func (s *Launcher) startNewTailerWithStoredInfo(file *tailer.File, m config.TailingMode, oldInfo *oldTailerInfo) bool {
	if file == nil {
		log.Debug("startNewTailerWithStoredInfo called with a nil file")
		return false
	}

	log.Debugf("Creating new tailer with stored info for file: %s", file.Path)
	channel, monitor := s.pipelineProvider.NextPipelineChanWithMonitor()

	// Use stored InfoRegistry if available, otherwise create new one
	var tailerInfo *status.InfoRegistry
	if oldInfo.InfoRegistry != nil {
		log.Debugf("Using stored InfoRegistry for file: %s", file.Path)
		tailerInfo = oldInfo.InfoRegistry
	} else {
		log.Debugf("Creating new InfoRegistry for file: %s", file.Path)
		tailerInfo = status.NewInfoRegistry()
	}

	// Create decoder with stored pattern if available
	var decoderInstance *decoder.Decoder
	if oldInfo.Pattern != nil {
		log.Debugf("Using stored pattern for decoder: %s", file.Path)
		decoderInstance = decoder.NewDecoderFromSourceWithPattern(file.Source, oldInfo.Pattern, tailerInfo)
	} else {
		log.Debugf("Creating decoder without stored pattern for file: %s", file.Path)
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
	}

	tailer := tailer.NewTailer(tailerOptions)

	var offset int64
	var whence int
	mode := s.handleTailingModeChange(tailer.Identifier(), m)
	offset, whence, err := Position(s.registry, tailer.Identifier(), mode)
	if err != nil {
		log.Warnf("Could not recover offset for file with path %v: %v", file.Path, err)
	}

	log.Infof("Starting new tailer with stored info (pattern: %v) for: %s (offset: %d, whence: %d)",
		oldInfo.Pattern != nil, file.Path, offset, whence)
	err = tailer.Start(offset, whence)
	if err != nil {
		log.Warn(err)
		return false
	}

	s.tailers.Add(tailer)
	log.Debugf("Successfully added tailer with stored info for file: %s", file.Path)
	return true
}

// handleTailingModeChange determines the tailing behaviour when the tailing mode for a given file has its
// configuration change. Two case may happen we can switch from "end" to "beginning" (1) and from "beginning" to
// "end" (2). If the tailing mode is set to forceEnd or forceBeginning it will remain unchanged.
// If (1) then the resulting tailing mode if "beginning" in order to honor existing offset to avoid duplicated lines to be sent.
// If (2) then the resulting tailing mode is "forceEnd" to drop any saved offset and tail from the end of the file.
func (s *Launcher) handleTailingModeChange(tailerID string, currentTailingMode config.TailingMode) config.TailingMode {
	log.Debugf("Handling tailing mode change for tailer: %s, current mode: %v", tailerID, currentTailingMode)
	if currentTailingMode == config.ForceBeginning || currentTailingMode == config.ForceEnd {
		log.Debugf("Using forced mode: %v", currentTailingMode)
		return currentTailingMode
	}
	previousMode, _ := config.TailingModeFromString(s.registry.GetTailingMode(tailerID))
	if previousMode != currentTailingMode {
		log.Infof("Tailing mode changed for %v. Was: %v: Now: %v", tailerID, previousMode, currentTailingMode)
		if currentTailingMode == config.Beginning {
			// end -> beginning, the offset will be honored if it exists
			log.Debugf("Mode changed from end to beginning, honoring existing offset")
			return config.Beginning
		}
		// beginning -> end, the offset will be ignored
		log.Debugf("Mode changed from beginning to end, ignoring existing offset")
		return config.ForceEnd
	}
	log.Debugf("No mode change detected, using current mode: %v", currentTailingMode)
	return currentTailingMode
}

// stopTailer stops the tailer
func (s *Launcher) stopTailer(tailer *tailer.Tailer) {
	log.Debugf("Stopping tailer: %s", tailer.GetId())
	go tailer.Stop()
	s.tailers.Remove(tailer)
	log.Debugf("Removed tailer from container: %s", tailer.GetId())
}

// restartTailer safely stops tailer and starts a new one
// returns true if the new tailer is up and running, false if an error occurred
func (s *Launcher) restartTailerAfterFileRotation(oldTailer *tailer.Tailer, file *tailer.File) bool {
	log.Info("Log rotation happened to ", file.Path)
	log.Debugf("Restarting tailer after rotation for file: %s", file.Path)
	oldTailer.StopAfterFileRotation()

	oldRegexPattern := oldTailer.GetDetectedPattern()
	oldInfoRegistry := oldTailer.GetInfo()

	log.Debugf("Stored pattern from old tailer: %v", oldRegexPattern != nil)
	log.Debugf("Stored info registry from old tailer: %v", oldInfoRegistry != nil)

	// Only store info if we're using checksum fingerprinting (where it will be retrieved)
	checkSumEnabled := pkgconfigsetup.Datadog().GetString("logs_config.fingerprint_strategy")
	if checkSumEnabled == "checksum" && (oldRegexPattern != nil || oldInfoRegistry != nil) {
		log.Debugf("Storing old tailer info for checksum fingerprinting")
		regexAndRegistry := &oldTailerInfo{
			InfoRegistry: oldInfoRegistry,
			Pattern:      oldRegexPattern,
		}
		s.oldInfoMap[file.Path] = regexAndRegistry
	}

	newTailer := s.createRotatedTailer(oldTailer, file, oldRegexPattern)
	// force reading file from beginning since it has been log-rotated
	err := newTailer.StartFromBeginning()
	if err != nil {
		log.Warn(err)
		return false
	}

	// Since newTailer and oldTailer share the same ID, tailers.Add will replace the old tailer.
	// We will keep track of the rotated tailer until it is finished.
	s.rotatedTailers = append(s.rotatedTailers, oldTailer)
	s.tailers.Add(newTailer)
	log.Debugf("Successfully restarted tailer after rotation for file: %s", file.Path)
	return true
}

// createTailer returns a new initialized tailer
func (s *Launcher) createTailer(file *tailer.File, outputChan chan *message.Message, capacityMonitor *metrics.CapacityMonitor) *tailer.Tailer {
	log.Debugf("Creating tailer for file: %s", file.Path)
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
	}

	tailer := tailer.NewTailer(tailerOptions)
	log.Debugf("Successfully created tailer for file: %s", file.Path)
	return tailer
}

func (s *Launcher) createRotatedTailer(t *tailer.Tailer, file *tailer.File, pattern *regexp.Regexp) *tailer.Tailer {
	log.Debugf("Creating rotated tailer for file: %s", file.Path)
	tailerInfo := t.GetInfo()
	channel, monitor := s.pipelineProvider.NextPipelineChanWithMonitor()
	newTailer := t.NewRotatedTailer(file, channel, monitor, decoder.NewDecoderFromSourceWithPattern(file.Source, pattern, tailerInfo), tailerInfo, s.tagger, s.registry)
	log.Debugf("Successfully created rotated tailer for file: %s", file.Path)
	return newTailer
}

//nolint:revive // TODO(AML) Fix revive linter
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

func (s *Launcher) GetScanCount() int64 {
	return atomic.LoadInt64(&s.scanCount)
}

// GetScanTimingStats returns scan timing statistics
func (s *Launcher) GetScanTimingStats() (lastDuration, avgDuration time.Duration, sampleCount int) {
	s.mu.Lock()
	defer s.mu.Unlock()

	lastDuration = s.lastScanDuration

	if len(s.scanDurations) > 0 {
		total := time.Duration(0)
		for _, d := range s.scanDurations {
			total += d
		}
		avgDuration = total / time.Duration(len(s.scanDurations))
		sampleCount = len(s.scanDurations)
	}

	return lastDuration, avgDuration, sampleCount
}

// GetFingerprintStrategy returns the current fingerprint strategy being used
func (s *Launcher) GetFingerprintStrategy() string {
	return pkgconfigsetup.Datadog().GetString("logs_config.fingerprint_strategy")
}
