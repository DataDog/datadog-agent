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

	ddconfig "github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/process/statsd"
	"github.com/DataDog/datadog-agent/pkg/process/util"
	"github.com/DataDog/datadog-agent/pkg/util/cloudproviders"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

const (
	cacheValidityNoRT = 2 * time.Second
)

// NewContainerCheck returns an instance of the ContainerCheck.
func NewContainerCheck() *ContainerCheck {
	return &ContainerCheck{}
}

// ContainerCheck is a check that returns container metadata and stats.
type ContainerCheck struct {
	sync.Mutex

	hostInfo          *HostInfo
	containerProvider util.ContainerProvider
	lastRates         map[string]*util.ContainerRateMetrics
	networkID         string

	containerFailedLogLimit *util.LogLimit

	maxBatchSize int
}

// Init initializes a ContainerCheck instance.
func (c *ContainerCheck) Init(_ *SysProbeConfig, info *HostInfo) error {
	c.containerProvider = util.GetSharedContainerProvider()
	c.hostInfo = info

	networkID, err := cloudproviders.GetNetworkID(context.TODO())
	if err != nil {
		log.Infof("no network ID detected: %s", err)
	}
	c.networkID = networkID

	c.containerFailedLogLimit = util.NewLogLimit(10, time.Minute*10)
	c.maxBatchSize = getMaxBatchSize()
	return nil
}

// IsEnabled returns true if the check is enabled by configuration
// Keep in mind that ContainerRTCheck.IsEnabled should only be enabled if the `ContainerCheck` is enabled
func (c *ContainerCheck) IsEnabled() bool {
	return canEnableContainerChecks(ddconfig.Datadog, true)
}

// SupportsRunOptions returns true if the check supports RunOptions
func (c *ContainerCheck) SupportsRunOptions() bool {
	return false
}

// Name returns the name of the ProcessCheck.
func (c *ContainerCheck) Name() string { return ContainerCheckName }

// Realtime indicates if this check only runs in real-time mode.
func (c *ContainerCheck) Realtime() bool { return false }

// ShouldSaveLastRun indicates if the output from the last run should be saved for use in flares
func (c *ContainerCheck) ShouldSaveLastRun() bool { return true }

// Run runs the ContainerCheck to collect a list of running ctrList and the
// stats for each container.
func (c *ContainerCheck) Run(nextGroupID func() int32, options *RunOptions) (RunResult, error) {
	c.Lock()
	defer c.Unlock()
	startTime := time.Now()

	var err error
	var containers []*model.Container
	var pidToCid map[int]string
	var lastRates map[string]*util.ContainerRateMetrics
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

	groupSize := len(containers) / c.maxBatchSize
	if len(containers)%c.maxBatchSize != 0 {
		groupSize++
	}
	chunked := chunkContainers(containers, groupSize)
	messages := make([]model.MessageBody, 0, groupSize)
	groupID := nextGroupID()
	for i := 0; i < groupSize; i++ {
		messages = append(messages, &model.CollectorContainer{
			HostName:          c.hostInfo.HostName,
			NetworkId:         c.networkID,
			Info:              c.hostInfo.SystemInfo,
			Containers:        chunked[i],
			GroupId:           groupID,
			GroupSize:         int32(groupSize),
			ContainerHostType: c.hostInfo.ContainerHostType,
		})
	}

	numContainers := float64(len(containers))
	statsd.Client.Gauge("datadog.process.containers.host_count", numContainers, []string{}, 1) //nolint:errcheck
	log.Debugf("collected %d containers in %s", int(numContainers), time.Now().Sub(startTime))
	return StandardRunResult(messages), nil
}

// Cleanup frees any resource held by the ContainerCheck before the agent exits
func (c *ContainerCheck) Cleanup() {}

// chunkContainers formats and chunks the ctrList into a slice of chunks using a specific number of chunks.
func chunkContainers(containers []*model.Container, chunks int) [][]*model.Container {
	perChunk := (len(containers) / chunks) + 1
	chunked := make([][]*model.Container, 0, chunks)
	chunk := make([]*model.Container, 0, perChunk)

	for _, ctr := range containers {
		chunk = append(chunk, ctr)
		if len(chunk) == perChunk {
			chunked = append(chunked, chunk)
			chunk = make([]*model.Container, 0, perChunk)
		}
	}
	if len(chunk) > 0 {
		chunked = append(chunked, chunk)
	}
	return chunked
}
