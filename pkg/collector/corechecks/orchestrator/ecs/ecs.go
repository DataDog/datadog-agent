// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build orchestrator

// Package ecs is used for the orchestrator ECS check
package ecs

import (
	"math/rand"
	"time"

	"go.uber.org/atomic"

	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/integration"
	"github.com/DataDog/datadog-agent/comp/core/workloadmeta"
	"github.com/DataDog/datadog-agent/pkg/aggregator/sender"
	"github.com/DataDog/datadog-agent/pkg/collector/check"
	core "github.com/DataDog/datadog-agent/pkg/collector/corechecks"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/cluster/orchestrator/collectors"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/cluster/orchestrator/collectors/ecs"
	oconfig "github.com/DataDog/datadog-agent/pkg/orchestrator/config"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/datadog-agent/pkg/util/optional"
)

// CheckName is the name of the check
const CheckName = "orchestrator_ecs"

// Check doesn't need additional fields
type Check struct {
	core.CheckBase
	WorkloadmetaStore workloadmeta.Component
	sender            sender.Sender
	config            *oconfig.OrchestratorConfig
	collectors        []collectors.Collector
	groupID           *atomic.Int32

	// isECSCollectionEnabledFunc is used for testing
	isECSCollectionEnabledFunc func() bool
}

// Factory creates a new check factory
func Factory() optional.Option[func() check.Check] {
	return optional.NewOption(newCheck)
}

func newCheck() check.Check {
	return &Check{
		CheckBase:         core.NewCheckBase(CheckName),
		WorkloadmetaStore: workloadmeta.GetGlobalStore(),
		config:            oconfig.NewDefaultOrchestratorConfig(),
		groupID:           atomic.NewInt32(rand.Int31()),
	}
}

// Configure the CPU check
// nil check to allow for overrides
func (c *Check) Configure(
	senderManager sender.SenderManager,
	integrationConfigDigest uint64,
	data integration.Data,
	initConfig integration.Data,
	source string,
) error {
	c.BuildID(integrationConfigDigest, data, initConfig)

	err := c.CommonConfigure(senderManager, integrationConfigDigest, initConfig, data, source)
	if err != nil {
		return err
	}

	err = c.config.Load()
	if err != nil {
		return err
	}

	if !c.isECSCollectionEnabled() {
		log.Debug("Orchestrator ECS Collection is disabled")
		return nil
	}

	if c.sender == nil {
		sender, err := c.GetSender()
		if err != nil {
			return err
		}
		c.sender = sender
	}
	return nil
}

// Run executes the check
func (c *Check) Run() error {
	if !c.isECSCollectionEnabled() {
		return nil
	}

	c.initCollectors()

	for _, collector := range c.collectors {
		if collector.Metadata().IsSkipped {
			c.Warnf("collector %s is skipped: %s", collector.Metadata().Name, collector.Metadata().SkippedReason)
			continue
		}

		runStartTime := time.Now()
		runConfig := &collectors.CollectorRunConfig{
			ECSCollectorRunConfig: collectors.ECSCollectorRunConfig{
				WorkloadmetaStore: c.WorkloadmetaStore,
			},
			Config:      c.config,
			MsgGroupRef: c.groupID,
		}
		result, err := collector.Run(runConfig)
		if err != nil {
			_ = c.Warnf("K8sCollector %s failed to run: %s", collector.Metadata().FullName(), err.Error())
			continue
		}
		runDuration := time.Since(runStartTime)
		log.Debugf("ECSCollector %s run stats: listed=%d processed=%d messages=%d duration=%s", collector.Metadata().FullName(), result.ResourcesListed, result.ResourcesProcessed, len(result.Result.MetadataMessages), runDuration)

		c.sender.OrchestratorMetadata(result.Result.MetadataMessages, runConfig.ClusterID, int(collector.Metadata().NodeType))
	}
	return nil
}

func (c *Check) isECSCollectionEnabled() bool {
	if c.isECSCollectionEnabledFunc != nil {
		return c.isECSCollectionEnabledFunc()
	}

	return oconfig.IsOrchestratorECSExplorerEnabled()
}

func (c *Check) initCollectors() {
	c.collectors = []collectors.Collector{ecs.NewTaskCollector()}
}
