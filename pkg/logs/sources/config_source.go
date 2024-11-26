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
)

// ConfigSources receives file paths to log configs and creates sources. The sources are added to a channel and read by the launcher.
// This class implements the SourceProvider interface
type ConfigSources struct {
	mu          sync.Mutex
	sources     []*LogSource
	added       []chan *LogSource
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
	fmt.Println("HEHEXD1")
	if err != nil {
		return err
	}
	fmt.Println("HEHEXD2")
	absolutePath := wd + "/" + path
	data, err := os.ReadFile(absolutePath)
	fmt.Println("HEHEXD3")
	if err != nil {
		return err
	}
	fmt.Println("HEHEXD4")
	logsConfig, err := logsConfig.ParseYAML(data)
	fmt.Println("HEHEXD5")
	if err != nil {
		return err
	}
	configSource := GetInstance()
	fmt.Println("HEHEXD6")
	for _, cfg := range logsConfig {
		if cfg.TailingMode == "" {
			cfg.TailingMode = "beginning"
		}
		fmt.Println("HEHEXD7")
		source := NewLogSource(cfg.Name, cfg)
		fmt.Println("HEHEXD8")
		configSource.AddSource(source)
		fmt.Println("HEHEXD9")
	}
	fmt.Println("HEHEXD10")

	return nil
}

// AddSource adds a new source.
// All of the subscribers registered for this source's type (src.Config.Type) will be
// notified.
func (s *ConfigSources) AddSource(source *LogSource) {
	fmt.Println("UNGA BUNGA 1")
	configSource := GetInstance()
	configSource.mu.Lock()
	fmt.Println("UNGA BUNGA 2")
	configSource.sources = append(configSource.sources, source)
	fmt.Println("UNGA BUNGA 3")
	if source.Config == nil || source.Config.Validate() != nil {
		configSource.mu.Unlock()
		return
	}
	fmt.Println("UNGA BUNGA 4")
	streams := configSource.added
	streamsForType := configSource.addedByType[source.Config.Type]
	configSource.mu.Unlock()
	fmt.Println("UNGA BUNGA 5")
	for _, stream := range streams {
		stream <- source
	}
	fmt.Println("UNGA BUNGA 6")
	fmt.Println("UNGA BUNGA streamsForType", streamsForType)
	for _, stream := range streamsForType {
		fmt.Println("steam is ", stream)
		fmt.Println("source is ", source)
		stream <- source
	}
	fmt.Println("UNGA BUNGA 7")
}

// SubscribeAll returns a channel carrying notifications of all added sources.
// Any sources added before this call are delivered from a new goroutine.
func (s *ConfigSources) SubscribeAll() (added chan *LogSource, _ chan *LogSource) {
	configSource := GetInstance()
	configSource.mu.Lock()
	defer configSource.mu.Unlock()

	added = make(chan *LogSource)

	configSource.added = append(configSource.added, added)

	existingSources := append([]*LogSource{}, configSource.sources...) // clone for goroutine
	go func() {
		for _, source := range existingSources {
			added <- source
		}
	}()

	return
}

// SubscribeForType returns two channels carrying notifications of added sources
// of a specified type
// Any sources added before this call are delivered from a new goroutine.
func (s *ConfigSources) SubscribeForType(sourceType string) (added chan *LogSource, _ chan *LogSource) {
	configSource := GetInstance()
	configSource.mu.Lock()
	defer configSource.mu.Unlock()

	added = make(chan *LogSource)

	if _, exists := configSource.addedByType[sourceType]; !exists {
		configSource.addedByType[sourceType] = []chan *LogSource{}
	}
	configSource.addedByType[sourceType] = append(configSource.addedByType[sourceType], added)

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
