// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018 Datadog, Inc.

package tailer

import (
	"sync"
	"time"

	log "github.com/cihub/seelog"

	"github.com/DataDog/datadog-agent/pkg/logs/auditor"
	"github.com/DataDog/datadog-agent/pkg/logs/config"
	"github.com/DataDog/datadog-agent/pkg/logs/message"
	"github.com/DataDog/datadog-agent/pkg/logs/pipeline"
)

// scanPeriod represents the period of scanning
const scanPeriod = 10 * time.Second

// Scanner checks all files provided by fileProvider and create new tailers
// or update the old ones if needed
type Scanner struct {
	pp                  pipeline.Provider
	tailingLimit        int
	fileProvider        *FileProvider
	tailers             map[string]*Tailer
	auditor             *auditor.Auditor
	tailerSleepDuration time.Duration
	ticker              *time.Ticker
	mu                  *sync.Mutex
	shouldStop          bool
}

// New returns an initialized Scanner
func New(sources []*config.LogSource, tailingLimit int, pp pipeline.Provider, auditor *auditor.Auditor, tailerSleepDuration time.Duration) *Scanner {
	tailSources := []*config.LogSource{}
	for _, source := range sources {
		switch source.Config.Type {
		case config.FileType:
			tailSources = append(tailSources, source)
		default:
		}
	}
	return &Scanner{
		pp:                  pp,
		tailingLimit:        tailingLimit,
		fileProvider:        NewFileProvider(tailSources, tailingLimit),
		tailers:             make(map[string]*Tailer),
		auditor:             auditor,
		tailerSleepDuration: tailerSleepDuration,
		ticker:              time.NewTicker(scanPeriod),
		mu:                  &sync.Mutex{},
	}
}

// setup sets all tailers
func (s *Scanner) setup() {
	files := s.fileProvider.FilesToTail()
	for _, file := range files {
		if len(s.tailers) == s.tailingLimit {
			return
		}
		if _, ok := s.tailers[file.Path]; ok {
			log.Warn("Can't tail file twice: ", file.Path)
		} else {
			s.setupTailer(file, false, s.pp.NextPipelineChan())
		}
	}
}

// setupTailer sets one tailer, making it tail from the beginning or the end
func (s *Scanner) setupTailer(file *File, tailFromBeginning bool, outputChan chan message.Message) {
	t := NewTailer(outputChan, file.Source, file.Path, s.tailerSleepDuration)
	var err error
	if tailFromBeginning {
		err = t.tailFromBeginning()
	} else {
		// resume tailing from last committed offset
		err = t.recoverTailing(s.auditor.GetLastCommittedOffset(t.Identifier()))
	}
	if err != nil {
		log.Warn(err)
	}
	s.tailers[file.Path] = t
}

// Start starts the Scanner
func (s *Scanner) Start() {
	s.setup()
	go s.run()
}

// Stop stops the Scanner and its tailers
func (s *Scanner) Stop() {
	s.mu.Lock()
	s.shouldStop = true
	s.ticker.Stop()
	wg := &sync.WaitGroup{}
	for _, tailer := range s.tailers {
		// stop the tailers in parallel
		wg.Add(1)
		go func(t *Tailer) {
			t.Stop(false, true)
			wg.Done()
		}(tailer)
		delete(s.tailers, tailer.path)
	}
	wg.Wait()
	s.mu.Unlock()
}

// run lets the Scanner tail its file
func (s *Scanner) run() {
	for range s.ticker.C {
		s.scan()
	}
}

// scan checks all the files we're expected to tail,
// compares them to the currently tailed files,
// and triggeres the required updates.
// For instance, when a file is logrotated,
// its tailer will keep tailing the rotated file.
// The Scanner needs to stop that previous tailer,
// and start a new one for the new file.
func (s *Scanner) scan() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.shouldStop {
		// prevent the scanner to create new tailers if stopped
		return
	}

	files := s.fileProvider.FilesToTail()
	filesTailed := make(map[string]bool)
	tailersLen := len(s.tailers)

	for _, file := range files {
		tailer, exists := s.tailers[file.Path]
		if !exists && tailersLen >= s.tailingLimit {
			// can't create new tailer because tailingLimit is reached
			continue
		}

		if !exists && tailersLen < s.tailingLimit {
			// create new tailer for file
			s.setupTailer(file, false, s.pp.NextPipelineChan())
			tailersLen++
			filesTailed[file.Path] = true
			continue
		}

		didRotate, err := s.didFileRotate(file, tailer)
		if err != nil {
			continue
		}

		if didRotate {
			// update tailer because of file-rotation on file
			s.onFileRotation(tailer, file)
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

// didFileRotate returns true if a file-rotation happened to file
// since tailer has been set up, otherwise returns false
func (s *Scanner) didFileRotate(file *File, tailer *Tailer) (bool, error) {
	return tailer.checkForRotation()
}

// onFileRotation safely stops tailer and setup a new one
func (s *Scanner) onFileRotation(tailer *Tailer, file *File) {
	log.Info("Log rotation happened to ", tailer.path)
	tailer.Stop(true, false)
	s.setupTailer(file, true, tailer.outputChan)
}

// stopTailer stops the tailer
func (s *Scanner) stopTailer(tailer *Tailer) {
	tailer.Stop(false, false)
	delete(s.tailers, tailer.path)
}
