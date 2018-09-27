// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018 Datadog, Inc.

package config

import (
	"sync"
)

const (
	ContainerdFormat = "containerd"
)

// LogSource holds a reference to an integration name and a log configuration, and allows to track errors and
// successful operations on it. Both name and configuration are static for now and determined at creation time.
// Changing the status is designed to be thread safe.
type LogSource struct {
	Name         string
	Config       *LogsConfig
	Status       *LogStatus
	inputs       map[string]bool
	lock         *sync.Mutex
	Messages     *Messages
	parserFormat string
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

// SetParserFormat sets a format that give information on how the source lines should be parsed
func (s *LogSource) SetParserFormat(parserFormat string) {
	s.lock.Lock()
	s.parserFormat = parserFormat
	s.lock.Unlock()
}

// GetParserFormat returns the parserFormat used by this source
func (s *LogSource) GetParserFormat() string {
	s.lock.Lock()
	defer s.lock.Unlock()
	return s.parserFormat
}
