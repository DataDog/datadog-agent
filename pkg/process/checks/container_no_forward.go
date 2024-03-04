// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package checks

import (
	"context"
	"sync"
	"time"

	model "github.com/DataDog/agent-payload/v5/process"

	sysconfig "github.com/DataDog/datadog-agent/cmd/system-probe/config"
	sysconfigtypes "github.com/DataDog/datadog-agent/cmd/system-probe/config/types"
	ddconfig "github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/process/util"
	proccontainers "github.com/DataDog/datadog-agent/pkg/process/util/containers"
	"github.com/DataDog/datadog-agent/pkg/util/cloudproviders"
	"github.com/DataDog/datadog-agent/pkg/util/flavor"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// NewContainerNoForwardCheck returns an instance of the ContainerNoForwardCheck.
func NewContainerNoForwardCheck(config ddconfig.Reader, syscfg *sysconfigtypes.Config) *ContainerNoForwardCheck {
	_, npmModuleEnabled := syscfg.EnabledModules[sysconfig.NetworkTracerModule]
	return &ContainerNoForwardCheck{
		config:         config,
		runInCoreAgent: config.GetBool("process_config.run_in_core_agent.enabled"),
		npmEnabled:     npmModuleEnabled && syscfg.Enabled,
	}
}

// ContainerNoForwardCheck is a check that returns container metadata and stats.
type ContainerNoForwardCheck struct {
	sync.Mutex

	config ddconfig.Reader

	containerProvider proccontainers.ContainerProvider
	lastRates         map[string]*proccontainers.ContainerRateMetrics
	networkID         string

	containerFailedLogLimit *util.LogLimit

	runInCoreAgent bool
	npmEnabled     bool
}

// Init initializes a ContainerNoForwardCheck instance.
func (c *ContainerNoForwardCheck) Init(_ *SysProbeConfig, _ *HostInfo, _ bool) error {
	c.containerProvider = proccontainers.GetSharedContainerProvider()

	networkID, err := cloudproviders.GetNetworkID(context.TODO())
	if err != nil {
		log.Infof("no network ID detected: %s", err)
	}
	c.networkID = networkID

	c.containerFailedLogLimit = util.NewLogLimit(10, time.Minute*10)

	return nil
}

// IsEnabled returns true if the check is enabled by configuration
func (c *ContainerNoForwardCheck) IsEnabled() bool {
	return flavor.GetFlavor() == flavor.ProcessAgent && c.runInCoreAgent && c.npmEnabled
}

// SupportsRunOptions returns true if the check supports RunOptions
func (c *ContainerNoForwardCheck) SupportsRunOptions() bool {
	return false
}

// Name returns the name of the ContainerNoForwardCheck.
func (c *ContainerNoForwardCheck) Name() string { return ContainerNoForwardCheckName }

// Realtime indicates if this check only runs in real-time mode.
func (c *ContainerNoForwardCheck) Realtime() bool { return false }

// ShouldSaveLastRun indicates if the output from the last run should be saved for use in flares
func (c *ContainerNoForwardCheck) ShouldSaveLastRun() bool { return true }

// Run runs the ContainerConnectionsCheck to collect a list of running ctrList and the
// stats for each container.
//
//nolint:revive // TODO(PROC) Fix revive linter
func (c *ContainerNoForwardCheck) Run(nextGroupID func() int32, options *RunOptions) (RunResult, error) {
	c.Lock()
	defer c.Unlock()

	var err error
	var containers []*model.Container
	var pidToCid map[int]string
	var lastRates map[string]*proccontainers.ContainerRateMetrics
	containers, lastRates, pidToCid, err = c.containerProvider.GetContainers(cacheValidityNoRT, c.lastRates)
	if err == nil {
		c.lastRates = lastRates
	} else {
		log.Debugf("Unable to gather stats for containers, err: %v", err)
	}

	if len(containers) == 0 {
		log.Trace("No containers found")
		return nil, nil
	}

	// Keep track of containers addresses
	LocalResolver.LoadAddrs(containers, pidToCid)

	return nil, nil
}

// Cleanup frees any resource held by the ContainerCheck before the agent exits
func (c *ContainerNoForwardCheck) Cleanup() {}
