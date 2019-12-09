// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2017 Datadog, Inc.

// +build docker

package collectors

import (
	"fmt"
	"time"

	"github.com/DataDog/datadog-agent/pkg/errors"
	taggerutil "github.com/DataDog/datadog-agent/pkg/tagger/utils"
	"github.com/DataDog/datadog-agent/pkg/util/containers"
	ecsutil "github.com/DataDog/datadog-agent/pkg/util/ecs"
)

const (
	ecsFargateCollectorName = "ecs_fargate"
	ecsFargateExpireFreq    = 5 * time.Minute
)

// ECSFargateCollector polls the ecs metadata api.
type ECSFargateCollector struct {
	infoOut      chan<- []*TagInfo
	expire       *taggerutil.Expire
	lastExpire   time.Time
	expireFreq   time.Duration
	labelsAsTags map[string]string
}

// Detect tries to connect to the ECS metadata API
func (c *ECSFargateCollector) Detect(out chan<- []*TagInfo) (CollectionMode, error) {
	var err error

	if ecsutil.IsFargateInstance() {
		c.infoOut = out
		c.lastExpire = time.Now()
		c.expireFreq = ecsFargateExpireFreq
		c.expire, err = taggerutil.NewExpire(ecsFargateExpireFreq)
		c.labelsAsTags = retrieveMappingFromConfig("docker_labels_as_tags")

		if err != nil {
			return PullCollection, fmt.Errorf("Failed to instantiate the container expiring process")
		}
		return PullCollection, nil
	}

	return NoCollection, fmt.Errorf("Failed to connect to task metadata API, ECS tagging will not work")
}

// Pull looks for new containers and computes deletions
func (c *ECSFargateCollector) Pull() error {
	meta, err := ecsutil.GetTaskMetadata()
	if err != nil {
		return err
	}
	// Only parse new containers
	updates, err := c.parseMetadata(meta, false)
	if err != nil {
		return err
	}
	c.infoOut <- updates

	// Throttle deletions
	if time.Now().Before(c.lastExpire.Add(c.expireFreq)) {
		return nil
	}

	expireList, err := c.expire.ComputeExpires()
	if err != nil {
		return err
	}
	expiries, err := c.parseExpires(expireList)
	if err != nil {
		return err
	}
	c.infoOut <- expiries
	c.lastExpire = time.Now()
	return nil
}

// Fetch parses tags for a container on cache miss. We avoid races with Pull,
// we re-parse the whole list, but don't send updates on other containers.
func (c *ECSFargateCollector) Fetch(container string) ([]string, []string, []string, error) {
	meta, err := ecsutil.GetTaskMetadata()
	if err != nil {
		return []string{}, []string{}, []string{}, err
	}
	// Force a full parse to avoid missing the container in a race with Pull
	updates, err := c.parseMetadata(meta, true)
	if err != nil {
		return []string{}, []string{}, []string{}, err
	}

	for _, info := range updates {
		if info.Entity == container {
			return info.LowCardTags, info.OrchestratorCardTags, info.HighCardTags, nil
		}
	}
	// container not found in updates
	return []string{}, []string{}, []string{}, errors.NewNotFound(container)
}

// parseExpires transforms event from the PodWatcher to TagInfo objects
func (c *ECSFargateCollector) parseExpires(idList []string) ([]*TagInfo, error) {
	var output []*TagInfo
	for _, id := range idList {
		info := &TagInfo{
			Source:       ecsFargateCollectorName,
			Entity:       containers.BuildTaggerEntityName(id),
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
	registerCollector(ecsFargateCollectorName, ecsFargateFactory, NodeOrchestrator)
}
