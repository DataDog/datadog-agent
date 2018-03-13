// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2017 Datadog, Inc.

// +build docker

package collectors

import (
	"fmt"
	"time"

	taggerutil "github.com/DataDog/datadog-agent/pkg/tagger/utils"
	"github.com/DataDog/datadog-agent/pkg/util/docker"
	ecsutil "github.com/DataDog/datadog-agent/pkg/util/ecs"
)

const (
	ecsFargateCollectorName = "ecs_fargate"
	ecsFargateExpireFreq    = 5 * time.Minute
)

// ECSFargateCollector polls the ecs metadata api.
type ECSFargateCollector struct {
	infoOut    chan<- []*TagInfo
	expire     *taggerutil.Expire
	lastExpire time.Time
	lastSeen   map[string]interface{}
	expireFreq time.Duration
}

// Detect tries to connect to the ECS metadata API
func (c *ECSFargateCollector) Detect(out chan<- []*TagInfo) (CollectionMode, error) {
	var err error

	if ecsutil.IsFargateInstance() {
		c.infoOut = out
		c.lastExpire = time.Now()
		c.expireFreq = ecsFargateExpireFreq

		c.expire, err = taggerutil.NewExpire(ecsFargateExpireFreq)

		if err != nil {
			return FetchOnlyCollection, fmt.Errorf("Failed to instantiate the container expiring process")
		}
		return FetchOnlyCollection, nil
	}

	return NoCollection, fmt.Errorf("Failed to connect to task metadata API, ECS tagging will not work")
}

// Pull triggers a container-list refresh and sends new info. It also triggers
// container deletion computation every 'expireFreq'
func (c *ECSFargateCollector) Pull() error {
	// Compute new/updated containers
	meta, err := ecsutil.GetTaskMetadata()
	if err != nil {
		return err
	}
	updates, deadCo, err := c.pullMetadata(meta)
	if err != nil {
		return err
	}
	c.infoOut <- updates

	// Throttle deletion computations
	if time.Now().Sub(c.lastExpire) < c.expireFreq {
		return nil
	}

	expiries, err := c.parseExpires(deadCo)
	if err != nil {
		return err
	}
	c.infoOut <- expiries
	c.lastExpire = time.Now()
	return nil
}

// Fetch fetches ECS tags for a container on demand
func (c *ECSFargateCollector) Fetch(container string) ([]string, []string, error) {
	meta, err := ecsutil.GetTaskMetadata()
	if err != nil {
		return []string{}, []string{}, err
	}

	// since we download the metadata anyway might as well do a Pull refresh
	updates, deadCo, err := c.pullMetadata(meta)
	if err != nil {
		return []string{}, []string{}, err
	}

	c.infoOut <- updates

	expiries, err := c.parseExpires(deadCo)
	if err != nil {
		return nil, nil, err
	}
	c.infoOut <- expiries
	c.lastExpire = time.Now()

	return c.fetchMetadata(meta, container)
}

// parseExpires transforms event from the PodWatcher to TagInfo objects
func (c *ECSFargateCollector) parseExpires(idList []string) ([]*TagInfo, error) {
	var output []*TagInfo
	for _, id := range idList {
		info := &TagInfo{
			Source:       ecsFargateCollectorName,
			Entity:       docker.ContainerIDToEntityName(id),
			DeleteEntity: true,
		}
		output = append(output, info)
	}
	return output, nil
}

func ecsFargateFactory() Collector {
	return &ECSFargateCollector{}
}

func init() {
	registerCollector(ecsFargateCollectorName, ecsFargateFactory, HighPriority)
}
