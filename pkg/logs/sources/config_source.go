// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package sources

import (
	"os"
	"sync"

	logsConfig "github.com/DataDog/datadog-agent/comp/logs/agent/config"
)

// ConfigSources receives file paths to log configs and creates sources. The sources are added to a channel and read by the launcher.
// This class implements the SourceProvider interface
type ConfigSources struct {
	addedByType map[string][]chan *LogSource
}

var (
	instance *ConfigSources
	once     sync.Once
)

// GetInstance provides a singleton instance of ConfigSources.
func GetInstance() *ConfigSources {
	once.Do(func() {
		instance = &ConfigSources{
			addedByType: make(map[string][]chan *LogSource),
		}
	})
	return instance
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
	configSource := GetInstance()
	for _, cfg := range logsConfig {
		if cfg.TailingMode == "" {
			cfg.TailingMode = "beginning"
		}
		source := NewLogSource(cfg.Name, cfg)
		configSource.AddSource(source)
	}

	return nil
}

// AddSource adds a new source.
// All of the subscribers registered for this source's type (src.Config.Type) will be
// notified.
func (s *ConfigSources) AddSource(source *LogSource) {
	configSource := GetInstance()
	if source.Config == nil || source.Config.Validate() != nil {
		return
	}
	streamsForType := configSource.addedByType[source.Config.Type]
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
	configSource := GetInstance()

	added = make(chan *LogSource)

	if _, exists := configSource.addedByType[sourceType]; !exists {
		configSource.addedByType[sourceType] = []chan *LogSource{}
	}
	configSource.addedByType[sourceType] = append(configSource.addedByType[sourceType], added)
	return
}

// GetAddedForType is required for the SourceProvider interface
func (s *ConfigSources) GetAddedForType(_ string) chan *LogSource {
	return nil
}
