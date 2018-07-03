// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018 Datadog, Inc.

package logs

import (
	"encoding/json"
	"fmt"

	"github.com/DataDog/datadog-agent/pkg/autodiscovery/integration"
	"github.com/DataDog/datadog-agent/pkg/util/log"

	"github.com/DataDog/datadog-agent/pkg/logs/config"
	"github.com/DataDog/datadog-agent/pkg/logs/input"
	"github.com/DataDog/datadog-agent/pkg/logs/restart"
)

// Scheduler registers to autodiscovery to schedule/unschedule log-collection.
type Scheduler struct {
	inputs map[string]input.Input
}

// NewScheduler returns a new scheduler.
func NewScheduler(inputs map[string]input.Input) *Scheduler {
	return &Scheduler{
		inputs: inputs,
	}
}

// Start starts all inputs
func (s *Scheduler) Start() {
	restart.Start(s.inputs...)
}

// Stop stops all inputs
func (s *Scheduler) Stop() {
	stopper := restart.NewParallelStopper()
	for _, input := range s.inputs {
		stopper.Add(input)
	}
	stopper.Stop()
}

// Schedule handles new configurations, transforms them to log-sources and pass them
// along to the right inputs to start collecting logs.
func (s *Scheduler) Schedule(configs []integration.Config) {
	for _, cfg := range configs {
		if !s.isLogConfig(cfg) {
			continue
		}
		input, err := s.getInput(config)
		if err != nil {
			log.Debugf("Invalid input: %v", err)
			continue
		}
		source, err := s.toSource(cfg)
		if err != nil {
			log.Warnf("Invalid configuration: %v", err)
			continue
		}
		input.Collect(source)
	}
}

// Unschedule retrieve the log-sources for the configurations and pass them along to the right inputs
// to stop collecting logs.
func (s *Scheduler) Unschedule(configs []integration.Config) {
	for _, cfg := range configs {
		if !s.isLogConfig(cfg) {
			continue
		}
		input, err := s.getInput(config)
		if err != nil {
			// config not supported by the logs-agent yet.
			continue
		}
		source, err := s.getSource(cfg)
		if err != nil {
			// parsing failed.
			continue
		}
		input.Purge(source)
	}
}

// isLogConfig returns true if config contains a logs config
func (s *Scheduler) isLogConfig(config []integration.Config) {
	return config.LogsConfig != nil
}

// getInput returns the right input to collect logs for the config.
func (s *Scheduler) getInput(config []integration.Config) (input.Input, error) {
	inputName, err := s.toInputName(cfg.Provider)
	if err != nil {
		return nil, fmt.Errorf("provider not supported yet: %v", cfg.Provider)
	}
	input, exists := s.inputs[inputName]
	if !exists {
		return nil, fmt.Errorf("input not plugged to autodiscovery yet: %v", inputName)
	}
	return input, nil
}

// toInputName returns the name of the input able to collect logs for the config,
// if the config is not supported by the logs-agent yet, returns an error.
func (s *Scheduler) toInputName(config integration.Config) (string, error) {
	// not implemented
	return config.Provider, nil
}

// toSource creates a new logs-source from the config,
// if the parsing failed, returns an error.
func (s *Scheduler) toSource(config integration.Config) (*config.LogSource, error) {
	// not implemented
	return nil, nil
}

// getSource returns the source matching the config,
// if none is found, returns an error.
func (s *Scheduler) getSource(config integration.Config) (*config.LogSource, error) {
	// not implemented
	return nil, nil
}
