// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//nolint:revive // TODO(AML) Fix revive linter
package cca

import (
	"time"

	"github.com/DataDog/datadog-agent/comp/core/autodiscovery"
	logsConfig "github.com/DataDog/datadog-agent/comp/logs/agent/config"
	coreConfig "github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/logs/schedulers"
	"github.com/DataDog/datadog-agent/pkg/logs/sources"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// Scheduler creates a single source to represent all containers collected due to
// the `logs_config.container_collect_all` configuration.
type Scheduler struct {
	ac autodiscovery.Component
	// added is closed when the source is added (for testing)
	added chan struct{}
}

var _ schedulers.Scheduler = &Scheduler{}

// New creates a new scheduler.
func New(ac autodiscovery.Component) schedulers.Scheduler {
	return &Scheduler{
		ac:    ac,
		added: make(chan struct{}),
	}
}

// Start implements schedulers.Scheduler#Start.
func (s *Scheduler) Start(sourceMgr schedulers.SourceManager) {
	if !coreConfig.Datadog().GetBool("logs_config.container_collect_all") {
		return
	}
	// source to collect all logs from all containers
	source := sources.NewLogSource(logsConfig.ContainerCollectAll, &logsConfig.LogsConfig{
		Type:    logsConfig.DockerType,
		Service: "docker",
		Source:  "docker",
	})

	// We must ensure that this source is enabled *after* the AutoConfig initialization, so
	// that any containers that do have specific configuration get handled first.  This is
	// a hack!
	go func() {
		s.blockUntilAutoConfigRanOnce(
			time.Millisecond * time.Duration(coreConfig.Datadog().GetInt("ac_load_timeout")))
		log.Debug("Adding ContainerCollectAll source to the Logs Agent")
		sourceMgr.AddSource(source)
		close(s.added)
	}()
}

// blockUntilAutoConfigRanOnce blocks until the AutoConfig has been run once.
// It also returns after the given timeout.
func (s *Scheduler) blockUntilAutoConfigRanOnce(timeout time.Duration) {
	now := time.Now()
	for {
		time.Sleep(100 * time.Millisecond) // don't hog the CPU
		if s.ac.HasRunOnce() {
			return
		}
		if time.Since(now) > timeout {
			log.Error("BlockUntilAutoConfigRanOnce timeout after", timeout)
			return
		}
	}
}

// Stop implements schedulers.Scheduler#Stop.
func (s *Scheduler) Stop() {}
