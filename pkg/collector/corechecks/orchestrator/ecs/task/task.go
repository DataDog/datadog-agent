// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package task is used for the orchestrator ecs task check
package task

import (
	"github.com/DataDog/datadog-agent/pkg/aggregator/sender"
	"github.com/DataDog/datadog-agent/pkg/autodiscovery/integration"
	"github.com/DataDog/datadog-agent/pkg/collector/check"
	core "github.com/DataDog/datadog-agent/pkg/collector/corechecks"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/datadog-agent/pkg/workloadmeta"
)

const checkName = "orchestrator_ecs_task"

func init() {
	core.RegisterCheck(checkName, CheckFactory)
}

// Check doesn't need additional fields
type Check struct {
	core.CheckBase
	hostName          string
	workloadmetaStore workloadmeta.Store
}

// CheckFactory returns a new Pod.Check
func CheckFactory() check.Check {
	return &Check{
		CheckBase:         core.NewCheckBase(checkName),
		workloadmetaStore: workloadmeta.GetGlobalStore(),
	}
}

// Configure the task check
// nil check to allow for overrides
func (c *Check) Configure(
	senderManager sender.SenderManager,
	integrationConfigDigest uint64,
	data integration.Data,
	initConfig integration.Data,
	source string,
) error {

	return nil
}

// Run executes the check
func (c *Check) Run() error {
	log.Error("list-tasks start")
	tasks := c.workloadmetaStore.ListECSTasks()
	for _, t := range tasks {
		if t != nil {
			log.Error("list-tasks: ", *t)
		}
	}
	log.Error("list-tasks end")
	return nil
}
