// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package file provides file-based log launchers
package file

import (
	"context"
	"regexp"
	"slices"
	"time"

	tagger "github.com/DataDog/datadog-agent/comp/core/tagger/def"
	"github.com/DataDog/datadog-agent/comp/logs/agent/config"
	flareController "github.com/DataDog/datadog-agent/comp/logs/agent/flare"
	auditor "github.com/DataDog/datadog-agent/comp/logs/auditor/def"
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
	"github.com/DataDog/datadog-agent/pkg/logs/types"
	"github.com/DataDog/datadog-agent/pkg/logs/util/opener"
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
	validatePodContainerID  bool
	scanPeriod              time.Duration
	flarecontroller         *flareController.FlareController
	tagger                  tagger.Component
	filesChan               chan []*tailer.File
	filesTailedBetweenScans []*tailer.File
	// Stores pertinent information about old tailer when rotation occurs and fingerprinting isn't possible
	oldInfoMap    map[string]*oldTailerInfo
	fileOpener    opener.FileOpener
	fingerprinter tailer.Fingerprinter
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
		fileOpener:             fileOpener,
		fingerprinter:          fingerprinter,
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
}

// run checks periodically if there are new files to tail and the state of its tailers until stop
func (s *Launcher) run() {
	scanTicker := time.NewTicker(s.scanPeriod)
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

			activeSourcesCopy := make([]*sources.LogSource, len(s.activeSources))
			copy(activeSourcesCopy, s.activeSources)

			// Clear files tailed between scans before starting new FilesToTail
			s.filesTailedBetweenScans = s.filesTailedBetweenScans[:0]

			scanTicker.Stop()
			go func() {
				s.filesChan <- s.fileProvider.FilesToTail(ctx, s.validatePodContainerID, activeSourcesCopy, s.registry)
			}()
		case files := <-s.filesChan:
			s.cleanUpRotatedTailers()

			s.resolveActiveTailers(files)
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

// cleanup all tailers
func (s *Launcher) cleanup() {
	stopper := startstop.NewParallelStopper()
	s.cleanUpRotatedTailers()
	for _, tailer := range s.rotatedTailers {
		stopper.Add(tailer)
	}
	s.rotatedTailers = []*tailer.Tailer{}

	for _, tailer := range s.tailers.All() {
		stopper.Add(tailer)
		s.tailers.Remove(tailer)
	}
	stopper.Stop()

	// Clean up old info map to prevent memory leaks
	s.oldInfoMap = make(map[string]*oldTailerInfo)
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
	files = append(files, s.filesTailedBetweenScans...)
	s.filesTailedBetweenScans = s.filesTailedBetweenScans[:0]
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

			if s.fingerprinter.ShouldFileFingerprint(file) {
				didRotate, err = tailered.DidRotateViaFingerprint(s.fingerprinter)
				if err != nil {
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
	// Pass 2 - Create new tailers for files that need to be tailed
	for _, file := range files {
		scanKey := file.GetScanKey()
		_, isTailed := s.tailers.Get(scanKey)
		if isTailed {
			filesTailed[scanKey] = true
			continue
		}

		// Check if we have stored info for this file from a previous rotation
		oldInfo, hasOldInfo := lastIterationOldInfo[scanKey]
		var fingerprint *types.Fingerprint
		var err error

		if s.fingerprinter.ShouldFileFingerprint(file) {
			// Check if this specific file should be fingerprinted
			fingerprint, err = s.fingerprinter.ComputeFingerprint(file)
			// Skip files with invalid fingerprints (Value == 0)
			if (fingerprint != nil && !fingerprint.ValidFingerprint()) || err != nil {
				// If fingerprint is invalid, persist the old info back into the map for future attempts
				if hasOldInfo {
					s.oldInfoMap[scanKey] = oldInfo
				}
				continue
			}
		} else {
			// File is not fingerprinted, but we still want to forward fingerprinting config for status display
			if fpConfig := s.fingerprinter.GetEffectiveConfigForFile(file); fpConfig != nil {
				fingerprint = &types.Fingerprint{
					Value:  types.InvalidFingerprintValue,
					Config: fpConfig,
				}
			}
		}

		if hasOldInfo {
			if s.startNewTailerWithStoredInfo(file, config.ForceBeginning, oldInfo, fingerprint) {
				// hasOldInfo is true when restarting a tailer after a file rotation, so start tailer from the beginning
				filesTailed[scanKey] = true
			}
		} else {
			// Normal case - no stored info
			if s.startNewTailer(file, config.Beginning, fingerprint) {
				filesTailed[scanKey] = true
			}
		}
	}
	log.Debugf("After starting new tailers, there are %d tailers running. Limit is %d.\n", s.tailers.Count(), s.tailingLimit)

	// Check how many file handles the Agent process has open and log a warning if the process is coming close to the OS file limit
	fileStats, err := procfilestats.GetProcessFileStats()
	if err == nil {
		CheckProcessTelemetry(fileStats)
	}
}

// cleanUpRotatedTailers removes any rotated tailers that have stopped from the list
func (s *Launcher) cleanUpRotatedTailers() {
	pendingTailers := []*tailer.Tailer{}
	for _, tailer := range s.rotatedTailers {
		if !tailer.IsFinished() {
			pendingTailers = append(pendingTailers, tailer)
		}
	}
	s.rotatedTailers = pendingTailers
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
		return
	}
	files, err := s.fileProvider.CollectFiles(source)
	if err != nil {
		source.Status.Error(err)
		log.Warnf("Could not collect files: %v", err)
		return
	}

	for _, file := range files {
		if s.tailers.Count() >= s.tailingLimit {
			return
		}

		if fileprovider.ShouldIgnore(s.validatePodContainerID, file) {
			continue
		}
		if tailer, isTailed := s.tailers.Get(file.GetScanKey()); isTailed {
			// new source inherits the old source's status
			source.Status = tailer.Source().Status
			// the file is already tailed, update the existing tailer's source so that the tailer
			// uses this new source going forward
			tailer.ReplaceSource(source)
			continue
		}

		var fingerprint *types.Fingerprint
		// Check if this specific file should be fingerprinted
		if s.fingerprinter.ShouldFileFingerprint(file) {
			fingerprint, err = s.fingerprinter.ComputeFingerprint(file)
			if err != nil || !fingerprint.ValidFingerprint() {
				continue
			}
		} else {
			// File is not fingerprinted, but we still want to forward fingerprinting config for status display
			if fpConfig := s.fingerprinter.GetEffectiveConfigForFile(file); fpConfig != nil {
				fingerprint = &types.Fingerprint{
					Value:  types.InvalidFingerprintValue,
					Config: fpConfig,
				}
			}
		}

		mode, isSet := config.TailingModeFromString(source.Config.TailingMode)
		if !isSet && source.Config.Identifier != "" {
			mode = config.Beginning
			source.Config.TailingMode = mode.String()
		}

		newTailerStarted := s.startNewTailer(file, mode, fingerprint)
		if newTailerStarted {
			s.filesTailedBetweenScans = append(s.filesTailedBetweenScans, file)
		}
	}
}

// startNewTailer creates a new tailer, making it tail from the last committed offset, the beginning or the end of the file,
// returns true if the operation succeeded, false otherwise.
func (s *Launcher) startNewTailer(file *tailer.File, m config.TailingMode, fingerprint *types.Fingerprint) bool {
	if file == nil {
		log.Debug("startNewTailer called with a nil file")
		return false
	}

	channel, monitor := s.pipelineProvider.NextPipelineChanWithMonitor()
	tailer := s.createTailer(file, channel, monitor, fingerprint)

	var offset int64
	var whence int
	mode := s.handleTailingModeChange(tailer.Identifier(), m)

	offset, whence, err := Position(s.registry, tailer.Identifier(), mode, s.fingerprinter)
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
	return true
}

// startNewTailerWithStoredInfo creates a new tailer using stored info from previous rotation
func (s *Launcher) startNewTailerWithStoredInfo(file *tailer.File, m config.TailingMode, oldInfo *oldTailerInfo, fingerprint *types.Fingerprint) bool {
	if file == nil {
		log.Debug("startNewTailerWithStoredInfo called with a nil file")
		return false
	}

	channel, monitor := s.pipelineProvider.NextPipelineChanWithMonitor()

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

	var offset int64
	var whence int
	mode := s.handleTailingModeChange(tailer.Identifier(), m)

	offset, whence, err := Position(s.registry, tailer.Identifier(), mode, s.fingerprinter)
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
	go tailer.Stop()
	s.tailers.Remove(tailer)
}

func (s *Launcher) rotateTailerWithoutRestart(oldTailer *tailer.Tailer, file *tailer.File) bool {
	log.Info("Log rotation happened to ", file.Path)
	oldTailer.StopAfterFileRotation()

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

// restartTailer safely stops tailer and starts a new one
// returns true if the new tailer is up and running, false if an error occurred
func (s *Launcher) restartTailerAfterFileRotation(oldTailer *tailer.Tailer, file *tailer.File) bool {
	log.Info("Log rotation happened to ", file.Path)
	oldTailer.StopAfterFileRotation()

	// Remove the rotated tailer from the active container so a fresh tailer can
	// be created for the new file while this one finishes draining the old file.
	s.tailers.Remove(oldTailer)

	oldRegexPattern := oldTailer.GetDetectedPattern()

	newTailer := s.createRotatedTailer(oldTailer, file, oldRegexPattern, nil)
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

func (s *Launcher) createRotatedTailer(t *tailer.Tailer, file *tailer.File, pattern *regexp.Regexp, fingerprint *types.Fingerprint) *tailer.Tailer {
	tailerInfo := t.GetInfo()
	channel, monitor := s.pipelineProvider.NextPipelineChanWithMonitor()
	if fingerprint != nil {
		log.Debugf("Creating new tailer for %s with fingerprint 0x%x (configuration: %v)", file.Path, fingerprint.Value, fingerprint.Config)
	} else {
		log.Debugf("Creating new tailer for %s with no fingerprint", file.Path)
	}
	newTailer := t.NewRotatedTailer(file, channel, monitor, decoder.NewDecoderFromSourceWithPattern(file.Source, pattern, tailerInfo), tailerInfo, s.tagger, fingerprint, s.fingerprinter, s.registry)
	addFingerprintConfigToTailerInfo(newTailer)

	return newTailer
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
