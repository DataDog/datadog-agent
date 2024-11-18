// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package sources

import (
	"os"
	"sync"

	logsConfig "github.com/DataDog/datadog-agent/comp/logs/agent/config"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// ConfigSources receives file paths to log configs and creates sources. The sources are added to a channel and read by the launcher.
type ConfigSources struct {
	mu            sync.Mutex
	sources       []*LogSource
	added         []chan *LogSource
	addedByType   map[string][]chan *LogSource
	removed       []chan *LogSource
	removedByType map[string][]chan *LogSource
}

var (
	instance *ConfigSources
	once     sync.Once
)

// GetInstance provides a singleton instance of ConfigSources.
func GetInstance() *ConfigSources {
	once.Do(func() {
		instance = &ConfigSources{
			addedByType:   make(map[string][]chan *LogSource),
			removedByType: make(map[string][]chan *LogSource),
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
		cfg.Path = absolutePath
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
	log.Tracef("Adding %s", source.Dump(false))
	configSource := GetInstance()
	configSource.mu.Lock()
	configSource.sources = append(configSource.sources, source)
	if source.Config == nil || source.Config.Validate() != nil {
		configSource.mu.Unlock()
		return
	}
	streams := configSource.added
	streamsForType := configSource.addedByType[source.Config.Type]
	configSource.mu.Unlock()
	for _, stream := range streams {
		stream <- source
	}

	for _, stream := range streamsForType {

		stream <- source
	}
}

// RemoveSource removes a source.
//
// All of the subscribers registered for this source's type (src.Config.Type) will be
// notified of its removal.
func (s *ConfigSources) RemoveSource(source *LogSource) {
	log.Tracef("Removing %s", source.Dump(false))
	configSource := GetInstance()
	configSource.mu.Lock()
	var sourceFound bool
	for i, src := range s.sources {
		if src == source {
			configSource.sources = append(configSource.sources[:i], s.sources[i+1:]...)
			sourceFound = true
			break
		}
	}
	streams := configSource.removed
	streamsForType := configSource.removedByType[source.Config.Type]
	configSource.mu.Unlock()

	if sourceFound {
		for _, stream := range streams {
			stream <- source
		}
		for _, stream := range streamsForType {
			stream <- source
		}
	}
}

// SubscribeAll returns two channels carrying notifications of all added and
// removed sources, respectively.  This guarantees consistency if sources are
// added or removed concurrently.
//
// Any sources added before this call are delivered from a new goroutine.
func (s *ConfigSources) SubscribeAll() (added chan *LogSource, removed chan *LogSource) {
	configSource := GetInstance()
	configSource.mu.Lock()
	defer configSource.mu.Unlock()

	added = make(chan *LogSource)
	removed = make(chan *LogSource)

	configSource.added = append(configSource.added, added)
	configSource.removed = append(configSource.removed, removed)

	existingSources := append([]*LogSource{}, configSource.sources...) // clone for goroutine
	go func() {
		for _, source := range existingSources {
			added <- source
		}
	}()

	return
}

// SubscribeForType returns two channels carrying notifications of added and
// removed sources with the given type, respectively.  This guarantees
// consistency if sources are added or removed concurrently.
//
// Any sources added before this call are delivered from a new goroutine.
func (s *ConfigSources) SubscribeForType(sourceType string) (added chan *LogSource, removed chan *LogSource) {
	configSource := GetInstance()
	configSource.mu.Lock()
	defer configSource.mu.Unlock()

	added = make(chan *LogSource)
	removed = make(chan *LogSource)

	if _, exists := configSource.addedByType[sourceType]; !exists {
		configSource.addedByType[sourceType] = []chan *LogSource{}
	}
	configSource.addedByType[sourceType] = append(configSource.addedByType[sourceType], added)

	if _, exists := configSource.removedByType[sourceType]; !exists {
		configSource.removedByType[sourceType] = []chan *LogSource{}
	}
	configSource.removedByType[sourceType] = append(configSource.removedByType[sourceType], removed)

	existingSources := append([]*LogSource{}, configSource.sources...) // clone for goroutine
	go func() {
		for _, source := range existingSources {
			if source.Config.Type == sourceType {
				added <- source
			}
		}
	}()

	return
}

// GetAddedForType returns a channel carrying notifications of new sources
// with the given type.
//
// Any sources added before this call are delivered from a new goroutine.
func (s *ConfigSources) GetAddedForType(sourceType string) chan *LogSource {
	configSource := GetInstance()
	configSource.mu.Lock()
	defer configSource.mu.Unlock()

	_, exists := configSource.addedByType[sourceType]
	if !exists {
		configSource.addedByType[sourceType] = []chan *LogSource{}
	}

	stream := make(chan *LogSource)
	configSource.addedByType[sourceType] = append(configSource.addedByType[sourceType], stream)

	existingSources := append([]*LogSource{}, configSource.sources...) // clone for goroutine
	go func() {
		for _, source := range existingSources {
			if source.Config.Type == sourceType {
				stream <- source
			}
		}
	}()

	return stream
}

// GetSources returns all the sources currently held.  The result is copied and
// will not be modified after it is returned.  However, the copy in the LogSources
// instance may change in that time (changing indexes or adding/removing entries).
func (s *ConfigSources) GetSources() []*LogSource {
	configSource := GetInstance()
	configSource.mu.Lock()
	defer configSource.mu.Unlock()

	clone := append([]*LogSource{}, configSource.sources...)
	return clone
}
