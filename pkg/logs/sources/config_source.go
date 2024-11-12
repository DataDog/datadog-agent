// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package sources

import (
	"fmt"
	"os"
	"sync"

	logsConfig "github.com/DataDog/datadog-agent/comp/logs/agent/config"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// ConfigSources serves as the interface between Schedulers and Launchers, distributing
// notifications of added/removed LogSources to subscribed Launchers.
//
// Each subscription receives its own unbuffered channel for sources, and should
// consume from the channel quickly to avoid blocking other goroutines.  There is
// no means to unsubscribe.
//
// If any sources have been added when GetAddedForType is called, then those sources
// are immediately sent to the channel.
//
// This type is threadsafe, and all of its methods can be called concurrently.
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

	// Step 1: Read the file content as bytes
	wd, err := os.Getwd()
	if err != nil {
		return err
	}

	absolutePath := wd + "/" + path
	data, err := os.ReadFile(absolutePath)
	if err != nil {
		return err
	}

	// Step 2: Parse the YAML data into LogsConfig structs
	logsConfig, err := logsConfig.ParseYAML(data)
	if err != nil {
		return err
	}
	for _, cfg := range logsConfig {
		source := NewLogSource(cfg.Name, cfg)
		// NOT SURE IF THIS IS NEEDED?
		// if source.Config.IntegrationName == "" {
		// 	// If the log integration comes from a config file, we try to match it with the config name
		// 	// that is most likely the integration name.
		// 	// If it comes from a container environment, the name was computed based on the `check_names`
		// 	// labels attached to the same container.
		// 	source.Config.IntegrationName = cfg.Name
		// }
		GetInstance().AddSource(source)
	}

	return nil
}

// AddSource adds a new source.
//
// All of the subscribers registered for this source's type (src.Config.Type) will be
// notified.
func (s *ConfigSources) AddSource(source *LogSource) {
	log.Tracef("Adding %s", source.Dump(false))
	GetInstance().mu.Lock()
	GetInstance().sources = append(GetInstance().sources, source)
	if source.Config == nil || source.Config.Validate() != nil {
		GetInstance().mu.Unlock()
		fmt.Println("andrewq config_source.go: source config isnt valid")
		return
	}
	streams := GetInstance().added
	streamsForType := GetInstance().addedByType[source.Config.Type]

	GetInstance().mu.Unlock()
	fmt.Println("channel 1 ", streams)
	fmt.Println("channel 2 ", streamsForType)
	fmt.Println("config_source.go source is : ", source)
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
	GetInstance().mu.Lock()
	var sourceFound bool
	for i, src := range s.sources {
		if src == source {
			GetInstance().sources = append(GetInstance().sources[:i], s.sources[i+1:]...)
			sourceFound = true
			break
		}
	}
	streams := GetInstance().removed
	streamsForType := GetInstance().removedByType[source.Config.Type]
	GetInstance().mu.Unlock()

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
	GetInstance().mu.Lock()
	defer GetInstance().mu.Unlock()

	added = make(chan *LogSource)
	removed = make(chan *LogSource)

	GetInstance().added = append(GetInstance().added, added)
	GetInstance().removed = append(GetInstance().removed, removed)

	existingSources := append([]*LogSource{}, GetInstance().sources...) // clone for goroutine
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
	GetInstance().mu.Lock()
	defer GetInstance().mu.Unlock()

	added = make(chan *LogSource)
	removed = make(chan *LogSource)

	if _, exists := GetInstance().addedByType[sourceType]; !exists {
		GetInstance().addedByType[sourceType] = []chan *LogSource{}
	}
	GetInstance().addedByType[sourceType] = append(GetInstance().addedByType[sourceType], added)

	if _, exists := GetInstance().removedByType[sourceType]; !exists {
		GetInstance().removedByType[sourceType] = []chan *LogSource{}
	}
	GetInstance().removedByType[sourceType] = append(GetInstance().removedByType[sourceType], removed)

	existingSources := append([]*LogSource{}, GetInstance().sources...) // clone for goroutine
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
	GetInstance().mu.Lock()
	defer GetInstance().mu.Unlock()

	_, exists := GetInstance().addedByType[sourceType]
	if !exists {
		GetInstance().addedByType[sourceType] = []chan *LogSource{}
	}

	stream := make(chan *LogSource)
	GetInstance().addedByType[sourceType] = append(GetInstance().addedByType[sourceType], stream)

	existingSources := append([]*LogSource{}, GetInstance().sources...) // clone for goroutine
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
	GetInstance().mu.Lock()
	defer GetInstance().mu.Unlock()

	clone := append([]*LogSource{}, GetInstance().sources...)
	return clone
}
