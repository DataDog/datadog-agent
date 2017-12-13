// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2017 Datadog, Inc.

// +build !windows

package tailer

import (
	"log"
	"os"
	"syscall"
	"time"

	"github.com/DataDog/datadog-agent/pkg/logs/auditor"
	"github.com/DataDog/datadog-agent/pkg/logs/config"
	"github.com/DataDog/datadog-agent/pkg/logs/message"
	"github.com/DataDog/datadog-agent/pkg/logs/pipeline"
)

const scanPeriod = 10 * time.Second

// Scanner checks all files metadata and updates its tailers if needed
type Scanner struct {
	sources []*config.IntegrationConfigLogSource
	pp      pipeline.Provider
	tailers map[string]*Tailer
	auditor *auditor.Auditor
}

// New returns an initialized Scanner
func New(sources []*config.IntegrationConfigLogSource, pp pipeline.Provider, auditor *auditor.Auditor) *Scanner {
	tailSources := []*config.IntegrationConfigLogSource{}
	for _, source := range sources {
		switch source.Type {
		case config.FileType:
			tailSources = append(tailSources, source)
		default:
		}
	}
	return &Scanner{
		sources: tailSources,
		pp:      pp,
		tailers: make(map[string]*Tailer),
		auditor: auditor,
	}
}

// setup sets all tailers
func (s *Scanner) setup() {
	for _, source := range s.sources {
		if _, ok := s.tailers[source.Path]; ok {
			log.Println("Can't tail file twice:", source.Path)
		} else {
			s.setupTailer(source, false, s.pp.NextPipelineChan())
		}
	}
}

// setupTailer sets one tailer, making it tail from the beginning or the end
func (s *Scanner) setupTailer(source *config.IntegrationConfigLogSource, tailFromBeginning bool, outputChan chan message.Message) {
	t := NewTailer(outputChan, source)
	var err error
	if tailFromBeginning {
		err = t.tailFromBeginning()
	} else {
		// resume tailing from last committed offset
		err = t.recoverTailing(s.auditor)
	}
	if err != nil {
		log.Println(err)
	}
	s.tailers[source.Path] = t
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
	for _, source := range s.sources {
		tailer := s.tailers[source.Path]
		f, err := os.Open(source.Path)
		if err != nil {
			continue
		}
		stat1, err := f.Stat()
		if err != nil {
			continue
		}
		stat2, err := tailer.file.Stat()
		if err != nil {
			s.onFileRotation(tailer, source)
			continue
		}
		if inode(stat1) != inode(stat2) {
			s.onFileRotation(tailer, source)
			continue
		}

		if stat1.Size() < tailer.GetReadOffset() {
			s.onFileRotation(tailer, source)
		}
	}
}

func (s *Scanner) onFileRotation(tailer *Tailer, source *config.IntegrationConfigLogSource) {
	shouldTrackOffset := false
	tailer.Stop(shouldTrackOffset)
	s.setupTailer(source, true, tailer.outputChan)
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
