// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

package file

import (
	"sync/atomic"
	"time"

	"github.com/DataDog/datadog-agent/pkg/util/log"

	"github.com/DataDog/datadog-agent/pkg/logs/auditor"
	"github.com/DataDog/datadog-agent/pkg/logs/config"
	"github.com/DataDog/datadog-agent/pkg/logs/message"
	"github.com/DataDog/datadog-agent/pkg/logs/pipeline"
	"github.com/DataDog/datadog-agent/pkg/logs/restart"
)

// scanPeriod represents the period of time between two scans.
const scanPeriod = 10 * time.Second

// Scanner checks all files provided by fileProvider and create new tailers
// or update the old ones if needed
type Scanner struct {
	pipelineProvider    pipeline.Provider
	addedSources        chan *config.LogSource
	removedSources      chan *config.LogSource
	activeSources       []*config.LogSource
	tailingLimit        int
	fileProvider        *Provider
	tailers             map[string]*Tailer
	registry            auditor.Registry
	tailerSleepDuration time.Duration
	stop                chan struct{}
}

// NewScanner returns a new scanner.
func NewScanner(sources *config.LogSources, tailingLimit int, pipelineProvider pipeline.Provider, registry auditor.Registry, tailerSleepDuration time.Duration) *Scanner {
	return &Scanner{
		pipelineProvider:    pipelineProvider,
		tailingLimit:        tailingLimit,
		addedSources:        sources.GetAddedForType(config.FileType),
		removedSources:      sources.GetRemovedForType(config.FileType),
		fileProvider:        NewProvider(tailingLimit),
		tailers:             make(map[string]*Tailer),
		registry:            registry,
		tailerSleepDuration: tailerSleepDuration,
		stop:                make(chan struct{}),
	}
}

// Start starts the Scanner
func (s *Scanner) Start() {
	go s.run()
}

// Stop stops the Scanner and its tailers in parallel,
// this call returns only when all the tailers are stopped
func (s *Scanner) Stop() {
	s.stop <- struct{}{}
	s.cleanup()
}

// run checks periodically if there are new files to tail and the state of its tailers until stop
func (s *Scanner) run() {
	scanTicker := time.NewTicker(scanPeriod)
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
func (s *Scanner) cleanup() {
	stopper := restart.NewParallelStopper()
	for _, tailer := range s.tailers {
		stopper.Add(tailer)
		delete(s.tailers, tailer.path)
	}
	stopper.Stop()
}

// scan checks all the files we're expected to tail,
// compares them to the currently tailed files,
// and triggeres the required updates.
// For instance, when a file is logrotated,
// its tailer will keep tailing the rotated file.
// The Scanner needs to stop that previous tailer,
// and start a new one for the new file.
func (s *Scanner) scan() {
	files := s.fileProvider.FilesToTail(s.activeSources)
	filesTailed := make(map[string]bool)
	tailersLen := len(s.tailers)

	for _, file := range files {
		tailer, isTailed := s.tailers[file.Path]
		if isTailed && atomic.LoadInt32(&tailer.shouldStop) != 0 {
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
			filesTailed[file.Path] = true
			continue
		}

		didRotate, err := DidRotate(tailer.file, tailer.GetReadOffset())
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

		filesTailed[file.Path] = true
	}

	for path, tailer := range s.tailers {
		// stop all tailers which have not been selected
		_, shouldTail := filesTailed[path]
		if !shouldTail {
			s.stopTailer(tailer)
		}
	}
}

// addSource keeps track of the new source and launch new tailers for this source.
func (s *Scanner) addSource(source *config.LogSource) {
	s.activeSources = append(s.activeSources, source)
	s.launchTailers(source)
}

// removeSource removes the source from cache.
func (s *Scanner) removeSource(source *config.LogSource) {
	for i, src := range s.activeSources {
		if src == source {
			// no need to stop the tailer here, it will be stopped in the next iteration of scan.
			s.activeSources = append(s.activeSources[:i], s.activeSources[i+1:]...)
			break
		}
	}
}

// launch launches new tailers for a new source.
func (s *Scanner) launchTailers(source *config.LogSource) {
	files, err := s.fileProvider.CollectFiles(source)
	if err != nil {
		source.Status.Error(err)
		log.Warnf("Could not collect files: %v", err)
		return
	}
	for _, file := range files {
		if len(s.tailers) >= s.tailingLimit {
			return
		}
		if _, isTailed := s.tailers[file.Path]; isTailed {
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
// returns true if the operation succeeded, false otherwise
func (s *Scanner) startNewTailer(file *File, m config.TailingMode) bool {
	tailer := s.createTailer(file, s.pipelineProvider.NextPipelineChan())

	var offset int64
	var whence int
	mode := s.handleTailingModeChange(tailer.Identifier(), m)

	offset, whence, err := Position(s.registry, tailer.Identifier(), mode)
	if err != nil {
		log.Warnf("Could not recover offset for file with path %v: %v", file.Path, err)
	}

	err = tailer.Start(offset, whence)
	if err != nil {
		log.Warn(err)
		return false
	}

	s.tailers[file.Path] = tailer
	return true
}

// handleTailingModeChange determines the tailing behaviour when the tailing mode for a given file has its
// configuration change. Two case may happen we can switch from "end" to "beginning" (1) and from "beginning" to
// "end" (2). If the tailing mode is set to forceEnd or forceBeginning it will remain unchanged.
// If (1) then the resulting tailing mode if "beginning" in order to honor existing offset to avoid duplicated lines to be sent.
// If (2) then the resulting tailing mode is "forceEnd" to drop any saved offset and tail from the end of the file.
func (s *Scanner) handleTailingModeChange(tailerID string, currentTailingMode config.TailingMode) config.TailingMode {
	if currentTailingMode == config.ForceBeginning || currentTailingMode == config.ForceEnd {
		return currentTailingMode
	}
	previousMode, _ := config.TailingModeFromString(s.registry.GetTailingMode(tailerID))
	if previousMode != currentTailingMode {
		log.Infof("Tailing mode changed for %v from %v to %v", tailerID, previousMode, currentTailingMode)
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
func (s *Scanner) stopTailer(tailer *Tailer) {
	go tailer.Stop()
	delete(s.tailers, tailer.path)
}

// restartTailer safely stops tailer and starts a new one
// returns true if the new tailer is up and running, false if an error occurred
func (s *Scanner) restartTailerAfterFileRotation(tailer *Tailer, file *File) bool {
	log.Info("Log rotation happened to ", tailer.path)
	tailer.StopAfterFileRotation()
	tailer = s.createTailer(file, tailer.outputChan)
	// force reading file from beginning since it has been log-rotated
	err := tailer.StartFromBeginning()
	if err != nil {
		log.Warn(err)
		return false
	}
	s.tailers[file.Path] = tailer
	return true
}

// createTailer returns a new initialized tailer
func (s *Scanner) createTailer(file *File, outputChan chan *message.Message) *Tailer {
	return NewTailer(outputChan, file.Source, file.Path, s.tailerSleepDuration, file.IsWildcardPath)
}
