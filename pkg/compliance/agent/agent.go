// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

// Package agent implements the Compliance Agent entrypoint
package agent

import (
	"path"
	"path/filepath"

	"github.com/DataDog/datadog-agent/pkg/collector/check"
	"github.com/DataDog/datadog-agent/pkg/compliance"
	"github.com/DataDog/datadog-agent/pkg/compliance/checks"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// Scheduler abstracts the collector.Scheduler interface
type Scheduler interface {
	Enter(check check.Check) error
	Cancel(id check.ID) error
	Run()
	Stop() error
	IsCheckScheduled(id check.ID) bool
}

// Agent defines Compliance Agent
type Agent struct {
	builder   checks.Builder
	scheduler Scheduler
	configDir string
}

// New creates a new instance of Agent
func New(reporter compliance.Reporter, scheduler Scheduler, configDir string, options ...checks.BuilderOption) (*Agent, error) {
	builder, err := checks.NewBuilder(
		reporter,
		options...,
	)
	if err != nil {
		return nil, err
	}

	return &Agent{
		builder:   builder,
		scheduler: scheduler,
		configDir: configDir,
	}, nil
}

// RunChecks runs checks right away without scheduling
func RunChecks(reporter compliance.Reporter, configDir string, options ...checks.BuilderOption) error {
	builder, err := checks.NewBuilder(
		reporter,
		options...,
	)
	if err != nil {
		return err
	}

	agent := &Agent{
		builder:   builder,
		configDir: configDir,
	}

	return agent.RunChecks()
}

// RunChecksFromFile runs checks from the specified file with no scheduling
func RunChecksFromFile(reporter compliance.Reporter, file string, options ...checks.BuilderOption) error {
	builder, err := checks.NewBuilder(
		reporter,
		options...,
	)
	if err != nil {
		return err
	}

	agent := &Agent{
		builder: builder,
	}

	return agent.RunChecksFromFile(file)
}

// Run starts the Compliance Agent
func (a *Agent) Run() error {
	a.scheduler.Run()
	onCheck := func(check check.Check) error {
		return a.scheduler.Enter(check)
	}
	return a.buildChecks(onCheck)
}

func runCheck(check check.Check) error {
	log.Infof("%s: Running check %s [%s]", check.ID(), check.String(), check.Version())
	return check.Run()
}

// RunChecks runs checks with no scheduling
func (a *Agent) RunChecks() error {
	return a.buildChecks(runCheck)

}

// RunChecksFromFile runs checks from the specified file with no scheduling
func (a *Agent) RunChecksFromFile(file string) error {
	return a.builder.ChecksFromFile(file, runCheck)
}

// Stop stops the Compliance Agent
func (a *Agent) Stop() {
	if err := a.scheduler.Stop(); err != nil {
		log.Errorf("Scheduler failed to stop: %v", err)
	}

	if err := a.builder.Close(); err != nil {
		log.Errorf("Builder failed to close: %v", err)
	}
}

func (a *Agent) buildChecks(onCheck compliance.CheckVisitor) error {
	log.Infof("Loading compliance rules from %s", a.configDir)
	pattern := path.Join(a.configDir, "*.yaml")
	files, err := filepath.Glob(pattern)

	if err != nil {
		return err
	}

	for _, file := range files {
		err := a.builder.ChecksFromFile(file, onCheck)
		if err != nil {
			log.Errorf("Failed to load rules from %s: %v", file, err)
			continue
		}
	}
	return nil
}
