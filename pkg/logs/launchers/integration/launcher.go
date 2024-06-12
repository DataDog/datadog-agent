// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package integration

import (
	"log"
	"os"
	"strings"
	"time"

	ddLog "github.com/DataDog/datadog-agent/pkg/util/log"

	"github.com/DataDog/datadog-agent/comp/logs/agent/config"
	"github.com/DataDog/datadog-agent/pkg/logs/auditor"
	"github.com/DataDog/datadog-agent/pkg/logs/internal/decoder"
	"github.com/DataDog/datadog-agent/pkg/logs/launchers"
	fileLauncher "github.com/DataDog/datadog-agent/pkg/logs/launchers/file"
	"github.com/DataDog/datadog-agent/pkg/logs/message"
	"github.com/DataDog/datadog-agent/pkg/logs/pipeline"
	"github.com/DataDog/datadog-agent/pkg/logs/sources"
	status "github.com/DataDog/datadog-agent/pkg/logs/status/utils"
	"github.com/DataDog/datadog-agent/pkg/logs/tailers"
	tailer "github.com/DataDog/datadog-agent/pkg/logs/tailers/file"
)

// DefaultSleepDuration represents the amount of time the tailer waits before reading new data when no data is received
const DefaultSleepDuration = 1 * time.Second

type Launcher struct {
	sources             chan *sources.LogSource
	piplineProvider     pipeline.Provider
	registry            auditor.Registry
	tailerSleepDuration time.Duration
	addedSources        chan *sources.LogSource
	removedSources      chan *sources.LogSource
	stop                chan struct{}
	runPath             string
	tailers             *tailers.TailerContainer[*tailer.Tailer]
	// tailers         []*tailer.Tailer
}

// NewLauncher returns a new launcher
func NewLauncher(runPath string, tailersSleepDuration time.Duration) *Launcher {
	return &Launcher{
		runPath:             runPath,
		tailers:             tailers.NewTailerContainer[*tailer.Tailer](),
		stop:                make(chan struct{}),
		tailerSleepDuration: tailersSleepDuration,
		// done:    make(chan struct{}),
	}
}

// Start starts the launcher
func (s *Launcher) Start(sourceProvider launchers.SourceProvider, pipelineProvider pipeline.Provider, registry auditor.Registry, tracker *tailers.TailerTracker) {
	s.piplineProvider = pipelineProvider
	s.addedSources, s.removedSources = sourceProvider.SubscribeForType(config.IntegrationType)
	s.registry = registry
	tracker.Add(s.tailers)

	go s.run()
}

// Stop stops the scanner tailers
func (s *Launcher) Stop() {
	s.stop <- struct{}{}
}

// run checks if there are new files to tail and tails them
func (s *Launcher) run() {
	scanTicker := time.NewTicker(time.Second * 1)
	// Add some functionality in here to detect when the agent is sent a log??
	// Maybe call createFile() whenever log is sent?
	for {
		select {
		case source := <-s.addedSources:
			filePath := s.createFile(source)
			s.addSource(source, filePath)
		case <-s.stop:
			return
		case <-scanTicker.C:
			return
		}
	}
}

// addSource adds the sources to active sources and launches tailers for the source
func (s *Launcher) addSource(source *sources.LogSource, filePath string) {
	// check if source is already being tailed here instead of startNewTailer
	s.startNewTailer(source, filePath)
}

// startNewTailer launches the tailer for a new source
func (s *Launcher) startNewTailer(source *sources.LogSource, filePath string) {
	file := tailer.NewFile(filePath, source, false)

	tailer := s.createTailer(file, s.piplineProvider.NextPipelineChan())

	// Not sure if these are necessary yet, should the feature overwrite any file
	// that's there? Or continue adding to it?
	var offset int64
	var whence int

	mode, _ := config.TailingModeFromString(source.Config.TailingMode)

	offset, whence, err := fileLauncher.Position(s.registry, tailer.GetId(), mode)
	if err != nil {
		ddLog.Warnf("Could not recover offset for file with path %v: %v", file.Path, err)
	}

	err = tailer.Start(offset, whence)
	if err != nil {
		ddLog.Warn(err)
	}

	s.tailers.Add(tailer)
}

// createTailer returns a new initialized tailer
func (s *Launcher) createTailer(file *tailer.File, outputChan chan *message.Message) *tailer.Tailer {
	tailerInfo := status.NewInfoRegistry()

	tailerOptions := &tailer.TailerOptions{
		OutputChan:    outputChan,
		File:          file,
		Info:          tailerInfo,
		SleepDuration: DefaultSleepDuration,
		Decoder:       decoder.NewDecoderFromSource(file.Source, tailerInfo),
	}

	return tailer.NewTailer(tailerOptions)
}

// createFile creates a file for the logsource
func (s *Launcher) createFile(source *sources.LogSource) string {
	fileName := source.Config.Source + ".log"
	pathSlice := []string{s.runPath, "integrations", source.Config.Service}
	path := strings.Join(pathSlice, "/")

	err := os.MkdirAll(path, 0755)
	if err != nil {
		log.Fatal(err)
	}

	filePath := strings.Join([]string{path, fileName}, "/")
	file, err := os.Create(filePath)
	defer file.Close()
	if err != nil {
		log.Fatal(err)
	}

	return filePath
}
