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
func New(reporter compliance.Reporter, scheduler Scheduler, configDir string, options ...checks.BuilderOption) *Agent {
	builder := checks.NewBuilder(
		reporter,
		options...,
	)

	return &Agent{
		builder:   builder,
		scheduler: scheduler,
		configDir: configDir,
	}
}

// Run starts the Compliance Agent
func (a *Agent) Run() error {
	a.scheduler.Run()

	log.Infof("Loading compliance rules from %s", a.configDir)
	pattern := path.Join(a.configDir, "*.yaml")
	matches, err := filepath.Glob(pattern)
	if err != nil {
		return err
	}

	for _, config := range matches {
		suite, err := compliance.ParseSuite(config)
		if err != nil {
			return err
		}

		log.Infof("%s/%s: loading suite from %s", suite.Meta.Name, suite.Meta.Version, config)
		for _, r := range suite.Rules {
			log.Debugf("%s/%s: loading rule %s", suite.Meta.Name, suite.Meta.Version, r.ID)
			checks, err := a.builder.ChecksFromRule(&suite.Meta, &r)
			if err != nil {
				return err
			}
			for _, check := range checks {
				log.Debugf("%s/%s: scheduling check %s", suite.Meta.Name, suite.Meta.Version, check.ID())
				err = a.scheduler.Enter(check)
				if err != nil {
					return err
				}
			}
		}
	}

	return nil
}

// Stop stops the Compliance Agent
func (a *Agent) Stop() {
	a.scheduler.Stop()
}
