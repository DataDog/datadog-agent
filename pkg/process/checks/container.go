// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package checks

import (
	"fmt"
	"math"
	"sync"
	"time"

	model "github.com/DataDog/agent-payload/v5/process"

	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	pkgconfigmodel "github.com/DataDog/datadog-agent/pkg/config/model"
	"github.com/DataDog/datadog-agent/pkg/process/net"
	"github.com/DataDog/datadog-agent/pkg/process/statsd"
	proccontainers "github.com/DataDog/datadog-agent/pkg/process/util/containers"
	"github.com/DataDog/datadog-agent/pkg/util/flavor"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

const (
	cacheValidityNoRT = 2 * time.Second
)

// NewContainerCheck returns an instance of the ContainerCheck.
func NewContainerCheck(config pkgconfigmodel.Reader, wmeta workloadmeta.Component) *ContainerCheck {
	return &ContainerCheck{
		config: config,
		wmeta:  wmeta,
	}
}

// ContainerCheck is a check that returns container metadata and stats.
type ContainerCheck struct {
	sync.Mutex

	config pkgconfigmodel.Reader

	hostInfo          *HostInfo
	containerProvider proccontainers.ContainerProvider
	lastRates         map[string]*proccontainers.ContainerRateMetrics
	networkID         string

	containerFailedLogLimit *log.Limit

	maxBatchSize int
	wmeta        workloadmeta.Component
}

// Init initializes a ContainerCheck instance.
func (c *ContainerCheck) Init(syscfg *SysProbeConfig, info *HostInfo, _ bool) error {
	sharedContainerProvider, err := proccontainers.GetSharedContainerProvider()
	if err != nil {
		return err
	}
	c.containerProvider = sharedContainerProvider
	c.hostInfo = info

	var tu net.SysProbeUtil
	if syscfg.NetworkTracerModuleEnabled {
		// Calling the remote tracer will cause it to initialize and check connectivity
		tu, err = net.GetRemoteSystemProbeUtil(syscfg.SystemProbeAddress)
		if err != nil {
			log.Warnf("could not initiate connection with system probe: %s", err)
		}
	}

	networkID, err := retryGetNetworkID(tu)
	if err != nil {
		log.Infof("no network ID detected: %s", err)
	}
	c.networkID = networkID

	c.containerFailedLogLimit = log.NewLogLimit(10, time.Minute*10)
	c.maxBatchSize = getMaxBatchSize(c.config)
	return nil
}

// IsEnabled returns true if the check is enabled by configuration
// Keep in mind that ContainerRTCheck.IsEnabled should only be enabled if the `ContainerCheck` is enabled
func (c *ContainerCheck) IsEnabled() bool {
	if c.config.GetBool("process_config.run_in_core_agent.enabled") && flavor.GetFlavor() == flavor.ProcessAgent {
		return false
	}

	return canEnableContainerChecks(c.config, true)
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
//
//nolint:revive // TODO(PROC) Fix revive linter
func (c *ContainerCheck) Run(nextGroupID func() int32, options *RunOptions) (RunResult, error) {
	c.Lock()
	defer c.Unlock()
	startTime := time.Now()

	var err error
	var containers []*model.Container
	var lastRates map[string]*proccontainers.ContainerRateMetrics
	containers, lastRates, _, err = c.containerProvider.GetContainers(cacheValidityNoRT, c.lastRates)
	if err == nil {
		c.lastRates = lastRates
	} else {
		log.Debugf("Unable to gather stats for containers, err: %v", err)
	}

	if len(containers) == 0 {
		log.Trace("No containers found")
		return nil, nil
	}

	groupSize := len(containers) / c.maxBatchSize
	if len(containers)%c.maxBatchSize != 0 {
		groupSize++
	}

	// For no chunking, set groupsize as 1 to ensure one chunk
	if options != nil && options.NoChunking {
		groupSize = 1
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
	agentNameTag := fmt.Sprintf("agent:%s", flavor.GetFlavor())
	statsd.Client.Gauge("datadog.process.containers.host_count", numContainers, []string{agentNameTag}, 1) //nolint:errcheck
	log.Debugf("collected %d containers in %s", int(numContainers), time.Since(startTime))
	return StandardRunResult(messages), nil
}

// Cleanup frees any resource held by the ContainerCheck before the agent exits
func (c *ContainerCheck) Cleanup() {}

// chunkContainers formats and chunks the ctrList into a slice of chunks using a specific number of chunks.
func chunkContainers(containers []*model.Container, chunks int) [][]*model.Container {
	perChunk := int(math.Ceil(float64(len(containers)) / float64(chunks)))
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
