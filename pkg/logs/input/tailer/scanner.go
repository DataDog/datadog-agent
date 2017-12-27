// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018 Datadog, Inc.

// +build !windows

package tailer

import (
	"os"
	"syscall"
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
	pp           pipeline.Provider
	tailingLimit int
	fileProvider *FileProvider
	tailers      map[string]*Tailer
	auditor      *auditor.Auditor
}

// New returns an initialized Scanner
func New(sources []*config.IntegrationConfigLogSource, tailingLimit int, pp pipeline.Provider, auditor *auditor.Auditor) *Scanner {
	tailSources := []*config.IntegrationConfigLogSource{}
	for _, source := range sources {
		switch source.Type {
		case config.FileType:
			tailSources = append(tailSources, source)
		default:
		}
	}
	return &Scanner{
		pp:           pp,
		tailingLimit: tailingLimit,
		fileProvider: NewFileProvider(tailSources, tailingLimit),
		tailers:      make(map[string]*Tailer),
		auditor:      auditor,
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
	t := NewTailer(outputChan, file.Source, file.Path)
	var err error
	if tailFromBeginning {
		err = t.tailFromBeginning()
	} else {
		// resume tailing from last committed offset
		err = t.recoverTailing(s.auditor)
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

// run lets the Scanner tail its file
func (s *Scanner) run() {
	ticker := time.NewTicker(scanPeriod)
	for range ticker.C {
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
	f, err := os.Open(file.Path)
	if err != nil {
		return false, err
	}

	stat1, err := f.Stat()
	if err != nil {
		return false, err
	}

	stat2, err := tailer.file.Stat()
	if err != nil {
		return true, nil
	}

	return inode(stat1) != inode(stat2) || stat1.Size() < tailer.GetReadOffset(), nil
}

// onFileRotation safely stops tailer and setup a new one
func (s *Scanner) onFileRotation(tailer *Tailer, file *File) {
	log.Info("Log rotation happened to", tailer.path)
	shouldTrackOffset := false
	tailer.Stop(shouldTrackOffset)
	s.setupTailer(file, true, tailer.outputChan)
}

// stopTailer safely stops tailer and remove its reference from tailers
func (s *Scanner) stopTailer(tailer *Tailer) {
	shouldTrackOffset := false
	tailer.Stop(shouldTrackOffset)
	delete(s.tailers, tailer.path)
}

// Stop stops the Scanner and its tailers
func (s *Scanner) Stop() {
	shouldTrackOffset := true
	for _, t := range s.tailers {
		t.Stop(shouldTrackOffset)
	}
}

// inode uniquely identifies a file on a filesystem
func inode(f os.FileInfo) uint64 {
	s := f.Sys()
	if s == nil {
		return 0
	}
	switch s := s.(type) {
	case *syscall.Stat_t:
		return uint64(s.Ino)
	default:
		return 0
	}
}
