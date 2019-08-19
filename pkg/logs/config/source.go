// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2019 Datadog, Inc.

package config

import (
	"sync"
)

// SourceType used for log line parsing logic.
// TODO: remove this logic.
type SourceType string

const (
	// DockerSourceType docker source type
	DockerSourceType SourceType = "docker"
	// KubernetesSourceType kubernetes source type
	KubernetesSourceType SourceType = "kubernetes"
)

// LogSource holds a reference to an integration name and a log configuration, and allows to track errors and
// successful operations on it. Both name and configuration are static for now and determined at creation time.
// Changing the status is designed to be thread safe.
type LogSource struct {
	Name     string
	Config   *LogsConfig
	Status   *LogStatus
	inputs   map[string]bool
	lock     *sync.Mutex
	Messages *Messages
	// sourceType is the type of the source that we are tailing whereas Config.Type is the type of the tailer
	// that reads log lines for this source. E.g, a sourceType == containerd and Config.Type == file means that
	// the agent is tailing a file to read logs of a containerd container
	sourceType SourceType
}

// NewLogSource creates a new log source.
func NewLogSource(name string, config *LogsConfig) *LogSource {
	return &LogSource{
		Name:     name,
		Config:   config,
		Status:   NewLogStatus(),
		inputs:   make(map[string]bool),
		lock:     &sync.Mutex{},
		Messages: NewMessages(),
	}
}

// AddInput registers an input as being handled by this source.
func (s *LogSource) AddInput(input string) {
	s.lock.Lock()
	s.inputs[input] = true
	s.lock.Unlock()
}

// RemoveInput removes an input from this source.
func (s *LogSource) RemoveInput(input string) {
	s.lock.Lock()
	delete(s.inputs, input)
	s.lock.Unlock()
}

// GetInputs returns the inputs handled by this source.
func (s *LogSource) GetInputs() []string {
	s.lock.Lock()
	defer s.lock.Unlock()
	inputs := make([]string, 0, len(s.inputs))
	for input := range s.inputs {
		inputs = append(inputs, input)
	}
	return inputs
}

// SetSourceType sets a format that give information on how the source lines should be parsed
func (s *LogSource) SetSourceType(sourceType SourceType) {
	s.lock.Lock()
	s.sourceType = sourceType
	s.lock.Unlock()
}

// GetSourceType returns the sourceType used by this source
func (s *LogSource) GetSourceType() SourceType {
	s.lock.Lock()
	defer s.lock.Unlock()
	return s.sourceType
}
