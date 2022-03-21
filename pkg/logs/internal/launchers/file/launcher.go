// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package file

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/DataDog/datadog-agent/pkg/util/log"

	"github.com/DataDog/datadog-agent/pkg/logs/auditor"
	"github.com/DataDog/datadog-agent/pkg/logs/config"
	"github.com/DataDog/datadog-agent/pkg/logs/internal/decoder"
	tailer "github.com/DataDog/datadog-agent/pkg/logs/internal/tailers/file"
	"github.com/DataDog/datadog-agent/pkg/logs/message"
	"github.com/DataDog/datadog-agent/pkg/logs/pipeline"
	"github.com/DataDog/datadog-agent/pkg/util/startstop"
)

// rxContainerID is used in the shouldIgnore func to do a best-effort validation
// that the file currently scanned for a source is attached to the proper container.
// If the container ID we parse from the filename isn't matching this regexp, we *will*
// tail the file because we prefer a false-negative than a false-positive (best-effort).
var rxContainerID = regexp.MustCompile("^[a-fA-F0-9]{64}$")

// ContainersLogsDir is the directory in which we should find containers logsfile
// with the container ID in their filename.
// Public to be able to change it while running unit tests.
var ContainersLogsDir = "/var/log/containers"

// DefaultSleepDuration represents the amount of time the tailer waits before reading new data when no data is received
const DefaultSleepDuration = 1 * time.Second

// Launcher checks all files provided by fileProvider and create new tailers
// or update the old ones if needed
type Launcher struct {
	pipelineProvider    pipeline.Provider
	addedSources        chan *config.LogSource
	removedSources      chan *config.LogSource
	activeSources       []*config.LogSource
	tailingLimit        int
	fileProvider        *fileProvider
	tailers             map[string]*tailer.Tailer
	registry            auditor.Registry
	tailerSleepDuration time.Duration
	stop                chan struct{}
	// set to true if we want to use `ContainersLogsDir` to validate that a new
	// pod log file is being attached to the correct containerID.
	// Feature flag defaulting to false, use `logs_config.validate_pod_container_id`.
	validatePodContainerID bool
	scanPeriod             time.Duration
}

// NewLauncher returns a new launcher.
func NewLauncher(sources *config.LogSources, tailingLimit int, pipelineProvider pipeline.Provider, registry auditor.Registry,
	tailerSleepDuration time.Duration, validatePodContainerID bool, scanPeriod time.Duration) *Launcher {
	return &Launcher{
		pipelineProvider:       pipelineProvider,
		tailingLimit:           tailingLimit,
		addedSources:           sources.GetAddedForType(config.FileType),
		removedSources:         sources.GetRemovedForType(config.FileType),
		fileProvider:           newFileProvider(tailingLimit),
		tailers:                make(map[string]*tailer.Tailer),
		registry:               registry,
		tailerSleepDuration:    tailerSleepDuration,
		stop:                   make(chan struct{}),
		validatePodContainerID: validatePodContainerID,
		scanPeriod:             scanPeriod,
	}
}

// Start starts the Scanner
func (s *Launcher) Start() {
	go s.run()
}

// Stop stops the Scanner and its tailers in parallel,
// this call returns only when all the tailers are stopped
func (s *Launcher) Stop() {
	s.stop <- struct{}{}
	s.cleanup()
}

// run checks periodically if there are new files to tail and the state of its tailers until stop
func (s *Launcher) run() {
	scanTicker := time.NewTicker(s.scanPeriod)
	defer scanTicker.Stop()
	for {
		select {
		case source := <-s.addedSources:
			s.addSource(source)
		case source := <-s.removedSources:
			s.removeSource(source)
		case <-scanTicker.C:
			// check if there are new files to tail, tailers to stop and tailer to restart because of file rotation
			s.scan()
		case <-s.stop:
			// no more file should be tailed
			return
		}
	}
}

// cleanup all tailers
func (s *Launcher) cleanup() {
	stopper := startstop.NewParallelStopper()
	for scanKey, tailer := range s.tailers {
		stopper.Add(tailer)
		delete(s.tailers, scanKey)
	}
	stopper.Stop()
}

// scan checks all the files we're expected to tail, compares them to the currently tailed files,
// and triggeres the required updates.
// For instance, when a file is logrotated, its tailer will keep tailing the rotated file.
// The Scanner needs to stop that previous tailer, and start a new one for the new file.
func (s *Launcher) scan() {
	files := s.fileProvider.filesToTail(s.activeSources)
	filesTailed := make(map[string]bool)
	tailersLen := len(s.tailers)

	for _, file := range files {
		// We're using generated key here: in case this file has been found while
		// scanning files for container, the key will use the format:
		//   <filepath>/<containerID>
		// If it has been found while scanning for a regular integration config,
		// its format will be:
		//   <filepath>
		// It is a hack to let two tailers tail the same file (it's happening
		// when a tailer for a dead container is still tailing the file, and another
		// tailer is tailing the file for the new container).
		tailerKey := file.GetScanKey()
		tailer, isTailed := s.tailers[tailerKey]
		if isTailed && tailer.IsFinished() {
			// skip this tailer as it must be stopped
			continue
		}
		if !isTailed && tailersLen >= s.tailingLimit {
			// can't create new tailer because tailingLimit is reached
			continue
		}

		if !isTailed && tailersLen < s.tailingLimit {
			// create a new tailer tailing from the beginning of the file if no offset has been recorded
			succeeded := s.startNewTailer(file, config.Beginning)
			if !succeeded {
				// the setup failed, let's try to tail this file in the next scan
				continue
			}
			tailersLen++
			filesTailed[tailerKey] = true
			continue
		}

		didRotate, err := tailer.DidRotate()
		if err != nil {
			continue
		}
		if didRotate {
			// restart tailer because of file-rotation on file
			succeeded := s.restartTailerAfterFileRotation(tailer, file)
			if !succeeded {
				// the setup failed, let's try to tail this file in the next scan
				continue
			}
		}

		filesTailed[tailerKey] = true
	}

	for scanKey, tailer := range s.tailers {
		// stop all tailers which have not been selected
		_, shouldTail := filesTailed[scanKey]
		if !shouldTail {
			s.stopTailer(scanKey, tailer)
		}
	}
}

// addSource keeps track of the new source and launch new tailers for this source.
func (s *Launcher) addSource(source *config.LogSource) {
	s.activeSources = append(s.activeSources, source)
	s.launchTailers(source)
}

// removeSource removes the source from cache.
func (s *Launcher) removeSource(source *config.LogSource) {
	for i, src := range s.activeSources {
		if src == source {
			// no need to stop the tailer here, it will be stopped in the next iteration of scan.
			s.activeSources = append(s.activeSources[:i], s.activeSources[i+1:]...)
			break
		}
	}
}

// launch launches new tailers for a new source.
func (s *Launcher) launchTailers(source *config.LogSource) {
	files, err := s.fileProvider.collectFiles(source)
	if err != nil {
		source.Status.Error(err)
		log.Warnf("Could not collect files: %v", err)
		return
	}
	for _, file := range files {
		if len(s.tailers) >= s.tailingLimit {
			return
		}
		if _, isTailed := s.tailers[file.GetScanKey()]; isTailed {
			continue
		}

		mode, _ := config.TailingModeFromString(source.Config.TailingMode)

		if source.Config.Identifier != "" {
			// only sources generated from a service discovery will contain a config identifier,
			// in which case we want to collect all logs.
			// FIXME: better detect a source that has been generated from a service discovery.
			mode = config.Beginning
		}

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

	// We also use the file launcher to look for containers and pods logs file, because of that
	// we have to make sure that the file we just detected is tagged with the correct
	// container ID. Enabled through `logs_config.validate_pod_container_id`.
	// The way k8s is storing files in /var/log/pods doesn't let us do that properly
	// (the filename doesn't contain the container ID).
	// However, the symlinks present in /var/log/containers are pointing to /var/log/pods files,
	// meaning that we can use them to validate that the file we have found is concerning us.
	// That's what the function shouldIgnore is trying to do when the directory exists and is readable.
	// See these links for more info:
	//   - https://github.com/kubernetes/kubernetes/issues/58638
	//   - https://github.com/fabric8io/fluent-plugin-kubernetes_metadata_filter/issues/105
	if s.validatePodContainerID && file.Source != nil &&
		(file.Source.GetSourceType() == config.KubernetesSourceType || file.Source.GetSourceType() == config.DockerSourceType) &&
		s.shouldIgnore(file) {
		return false
	}

	tailer := s.createTailer(file, s.pipelineProvider.NextPipelineChan())

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

	s.tailers[file.GetScanKey()] = tailer
	return true
}

// shouldIgnore resolves symlinks in /var/log/containers in order to use that redirection
// to validate that we will be reading a file for the correct container.
func (s *Launcher) shouldIgnore(file *tailer.File) bool {
	// this method needs a source config to detect whether we should ignore that file or not
	if file == nil || file.Source == nil || file.Source.Config == nil {
		return false
	}

	infos := make(map[string]string)
	err := filepath.Walk(ContainersLogsDir, func(containerLogFilename string, info os.FileInfo, err error) error {
		// we only wants to follow symlinks
		if info == nil || info.Mode()&os.ModeSymlink != os.ModeSymlink || info.IsDir() {
			// not a symlink, we are not interested in this file
			return nil
		}

		// resolve the symlink
		podLogFilename, err2 := os.Readlink(containerLogFilename)
		if err2 != nil {
			log.Debug("Error while resolving symlink of", containerLogFilename, ":", err)
			return nil
		}

		infos[podLogFilename] = containerLogFilename
		return nil
	})

	// this is not an error if we we are not currently looking for container logs files,
	// so not problem and just return false.
	// Still, we write a debug message to be able to troubleshoot that
	// in cases we're legitimately looking for containers logs.
	if err != nil {
		log.Debug("Can't look for symlinks in /var/log/containers:", err)
		return false
	}

	// container id extracted from the file found in /var/log/containers
	base := filepath.Base(infos[file.Path]) // only the file
	ext := filepath.Ext(base)               // file extension
	parts := strings.Split(base, "-")       // get only the container ID part from the file
	var containerIDFromFilename string
	if len(parts) > 1 {
		containerIDFromFilename = strings.TrimSuffix(parts[len(parts)-1], ext)
	}

	// basic validation of the ID that has been parsed, if it doesn't look like
	// an ID we don't want to compare another ID to it
	if containerIDFromFilename == "" || !rxContainerID.Match([]byte(containerIDFromFilename)) {
		return false
	}

	if file.Source.Config.Identifier != "" && containerIDFromFilename != "" {
		if strings.TrimSpace(strings.ToLower(containerIDFromFilename)) != strings.TrimSpace(strings.ToLower(file.Source.Config.Identifier)) {
			log.Debugf("We were about to tail a file attached to the wrong container (%s != %s), probably due to short-lived containers.",
				containerIDFromFilename, file.Source.Config.Identifier)
			// ignore this file, it is not concerning the container stored in file.Source
			return true
		}
	}

	return false
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
func (s *Launcher) stopTailer(scanKey string, tailer *tailer.Tailer) {
	go tailer.Stop()
	delete(s.tailers, scanKey)
}

// restartTailer safely stops tailer and starts a new one
// returns true if the new tailer is up and running, false if an error occurred
func (s *Launcher) restartTailerAfterFileRotation(tailer *tailer.Tailer, file *tailer.File) bool {
	log.Info("Log rotation happened to ", file.Path)
	tailer.StopAfterFileRotation()
	tailer = s.createRotatedTailer(tailer, file, tailer.GetDetectedPattern())
	// force reading file from beginning since it has been log-rotated
	err := tailer.StartFromBeginning()
	if err != nil {
		log.Warn(err)
		return false
	}
	s.tailers[file.GetScanKey()] = tailer
	return true
}

// createTailer returns a new initialized tailer
func (s *Launcher) createTailer(file *tailer.File, outputChan chan *message.Message) *tailer.Tailer {
	return tailer.NewTailer(outputChan, file, s.tailerSleepDuration, decoder.NewDecoderFromSource(file.Source))
}

func (s *Launcher) createRotatedTailer(t *tailer.Tailer, file *tailer.File, pattern *regexp.Regexp) *tailer.Tailer {
	return t.NewRotatedTailer(file, decoder.NewDecoderFromSourceWithPattern(file.Source, pattern))
}
