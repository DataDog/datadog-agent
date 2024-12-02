// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package sources

import (
	"os"

	logsConfig "github.com/DataDog/datadog-agent/comp/logs/agent/config"
)

// ConfigSources receives file paths to log configs and creates sources. The sources are added to a channel and read by the launcher.
// This class implements the SourceProvider interface
type ConfigSources struct {
	addedByType map[string][]chan *LogSource
}

// NewConfigSources provides an instance of ConfigSources.
func NewConfigSources() *ConfigSources {
	return &ConfigSources{
		addedByType: make(map[string][]chan *LogSource),
	}
}

// AddFileSource gets a file from a file path and adds it as a source.
func (s *ConfigSources) AddFileSource(path string) error {
	wd, err := os.Getwd()
	if err != nil {
		return err
	}
	absolutePath := wd + "/" + path
	data, err := os.ReadFile(absolutePath)
	if err != nil {
		return err
	}
	logsConfig, err := logsConfig.ParseYAML(data)
	if err != nil {
		return err
	}
	for _, cfg := range logsConfig {
		if cfg.TailingMode == "" {
			cfg.TailingMode = "beginning"
		}
		source := NewLogSource(cfg.Name, cfg)
		s.AddSource(source)
	}

	return nil
}

// AddSource adds a new source.
// All of the subscribers registered for this source's type (src.Config.Type) will be
// notified.
func (s *ConfigSources) AddSource(source *LogSource) {
	if source.Config == nil || source.Config.Validate() != nil {
		return
	}
	streamsForType := s.addedByType[source.Config.Type]
	for _, stream := range streamsForType {
		stream <- source
	}
}

// SubscribeAll is required for the SourceProvider interface
func (s *ConfigSources) SubscribeAll() (added chan *LogSource, _ chan *LogSource) {
	return
}

// SubscribeForType returns two channels carrying notifications of added sources
// of a specified type
// Any sources added before this call are delivered from a new goroutine.
func (s *ConfigSources) SubscribeForType(sourceType string) (added chan *LogSource, _ chan *LogSource) {

	added = make(chan *LogSource)

	if _, exists := s.addedByType[sourceType]; !exists {
		s.addedByType[sourceType] = []chan *LogSource{}
	}
	s.addedByType[sourceType] = append(s.addedByType[sourceType], added)
	return
}

// GetAddedForType is required for the SourceProvider interface
func (s *ConfigSources) GetAddedForType(_ string) chan *LogSource {
	return nil
}
