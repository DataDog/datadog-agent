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
	"github.com/DataDog/datadog-agent/pkg/process/config"
	"github.com/DataDog/datadog-agent/pkg/process/statsd"
	"github.com/DataDog/datadog-agent/pkg/process/util"
	"github.com/DataDog/datadog-agent/pkg/util/cloudproviders"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

const (
	cacheValidityNoRT = 2 * time.Second
)

// Container is a singleton ContainerCheck.
var Container = &ContainerCheck{}

// ContainerCheck is a check that returns container metadata and stats.
type ContainerCheck struct {
	sync.Mutex

	sysInfo           *model.SystemInfo
	containerProvider util.ContainerProvider
	lastRates         map[string]*util.ContainerRateMetrics
	networkID         string

	containerFailedLogLimit *util.LogLimit

	maxBatchSize int
}

// Init initializes a ContainerCheck instance.
func (c *ContainerCheck) Init(cfg *config.AgentConfig, info *model.SystemInfo) {
	c.containerProvider = util.GetSharedContainerProvider()
	c.sysInfo = info

	networkID, err := cloudproviders.GetNetworkID(context.TODO())
	if err != nil {
		log.Infof("no network ID detected: %s", err)
	}
	c.networkID = networkID

	c.containerFailedLogLimit = util.NewLogLimit(10, time.Minute*10)
	c.maxBatchSize = getMaxBatchSize()
}

// Name returns the name of the ProcessCheck.
func (c *ContainerCheck) Name() string { return config.ContainerCheckName }

// RealTime indicates if this check only runs in real-time mode.
func (c *ContainerCheck) RealTime() bool { return false }

// Run runs the ContainerCheck to collect a list of running ctrList and the
// stats for each container.
func (c *ContainerCheck) Run(cfg *config.AgentConfig, groupID int32) ([]model.MessageBody, error) {
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
	for i := 0; i < groupSize; i++ {
		messages = append(messages, &model.CollectorContainer{
			HostName:          cfg.HostName,
			NetworkId:         c.networkID,
			Info:              c.sysInfo,
			Containers:        chunked[i],
			GroupId:           groupID,
			GroupSize:         int32(groupSize),
			ContainerHostType: cfg.ContainerHostType,
		})
	}

	numContainers := float64(len(containers))
	statsd.Client.Gauge("datadog.process.containers.host_count", numContainers, []string{}, 1) //nolint:errcheck
	log.Debugf("collected %d containers in %s", int(numContainers), time.Now().Sub(startTime))
	return messages, nil
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
