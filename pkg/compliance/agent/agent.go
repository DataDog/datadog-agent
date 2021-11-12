// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package agent implements the Compliance Agent entrypoint
package agent

import (
	"context"
	"expvar"
	"path"
	"path/filepath"

	"github.com/DataDog/datadog-agent/pkg/collector/check"
	"github.com/DataDog/datadog-agent/pkg/compliance"
	"github.com/DataDog/datadog-agent/pkg/compliance/checks"
	"github.com/DataDog/datadog-agent/pkg/compliance/event"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

var status = expvar.NewMap("compliance")

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
	telemetry *telemetry
	configDir string
	cancel    context.CancelFunc
}

// New creates a new instance of Agent
func New(reporter event.Reporter, scheduler Scheduler, configDir string, options ...checks.BuilderOption) (*Agent, error) {
	builder, err := checks.NewBuilder(
		reporter,
		options...,
	)
	if err != nil {
		return nil, err
	}

	telemetry, err := newTelemetry()
	if err != nil {
		return nil, err
	}

	return &Agent{
		builder:   builder,
		scheduler: scheduler,
		configDir: configDir,
		telemetry: telemetry,
	}, nil
}

// RunChecks runs checks right away without scheduling
func RunChecks(reporter event.Reporter, configDir string, options ...checks.BuilderOption) error {
	builder, err := checks.NewBuilder(
		reporter,
		options...,
	)
	if err != nil {
		return err
	}

	defer builder.Close()

	agent := &Agent{
		builder:   builder,
		configDir: configDir,
	}

	return agent.RunChecks()
}

// RunChecksFromFile runs checks from the specified file with no scheduling
func RunChecksFromFile(reporter event.Reporter, file string, options ...checks.BuilderOption) error {
	builder, err := checks.NewBuilder(
		reporter,
		options...,
	)
	if err != nil {
		return err
	}

	defer builder.Close()

	agent := &Agent{
		builder: builder,
	}

	return agent.RunChecksFromFile(file)
}

// Run starts the Compliance Agent
func (a *Agent) Run() error {
	ctx, cancel := context.WithCancel(context.Background())
	a.cancel = cancel

	go a.telemetry.run(ctx)

	a.scheduler.Run()

	defer status.Set(
		"Checks",
		expvar.Func(func() interface{} {
			return a.builder.GetCheckStatus()
		}),
	)

	onCheck := func(rule *compliance.RuleCommon, check compliance.Check, err error) bool {
		if err != nil {
			log.Errorf("%s: check not scheduled: %v", rule.ID, err)
			return true
		}

		err = a.scheduler.Enter(check)
		if err != nil {
			log.Errorf("%s: failed to schedule check: %v", rule.ID, err)
			return false
		}

		return true
	}
	return a.buildChecks(onCheck)
}

func runCheck(rule *compliance.RuleCommon, check compliance.Check, err error) bool {
	if err != nil {
		log.Infof("%s: Not running check: %v", rule.ID, err)
		return true
	}

	log.Infof("%s: Running check: %s [version=%s]", rule.ID, check.String(), check.Version())
	err = check.Run()
	if err != nil {
		log.Errorf("%s: Check failed: %v", check.ID(), err)
	}
	return true
}

// RunChecks runs checks with no scheduling
func (a *Agent) RunChecks() error {
	return a.buildChecks(runCheck)
}

// RunChecksFromFile runs checks from the specified file with no scheduling
func (a *Agent) RunChecksFromFile(file string) error {
	log.Infof("Loading compliance rules from %s", file)
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

	a.cancel()
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
