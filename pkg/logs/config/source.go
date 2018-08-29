// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018 Datadog, Inc.

package config

import "sync"

// SourceOrigin represents the origin of the source.
type SourceOrigin int

const (
	// SourceOriginService indicates the source was created from a service listener.
	SourceOriginService SourceOrigin = iota
	// SourceOriginConfig indicates the source was created from a config provider.
	SourceOriginConfig
)

// LogSource holds a reference to and integration name and a log configuration, and allows to track errors and
// successful operations on it. Both name and configuration are static for now and determined at creation time.
// Changing the status is designed to be thread safe.
type LogSource struct {
	Name   string
	Config *LogsConfig
	Status *LogStatus
	Origin SourceOrigin
	inputs map[string]bool
	lock   *sync.Mutex
}

// NewLogSource creates a new log source.
func NewLogSource(name string, config *LogsConfig, origin SourceOrigin) *LogSource {
	return &LogSource{
		Name:   name,
		Config: config,
		Status: NewLogStatus(),
		Origin: origin,
		inputs: make(map[string]bool),
		lock:   &sync.Mutex{},
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
