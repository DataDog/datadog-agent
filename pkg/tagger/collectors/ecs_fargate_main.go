// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2017-present Datadog, Inc.

// +build docker

package collectors

import (
	"context"
	"fmt"
	"time"

	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/errors"
	ecsutil "github.com/DataDog/datadog-agent/pkg/util/ecs"
	ecsmeta "github.com/DataDog/datadog-agent/pkg/util/ecs/metadata"
	v2 "github.com/DataDog/datadog-agent/pkg/util/ecs/metadata/v2"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

const (
	ecsFargateCollectorName = "ecs_fargate"
	ecsFargateExpireFreq    = 5 * time.Minute
)

// ECSFargateCollector polls the ecs metadata api.
type ECSFargateCollector struct {
	client       *v2.Client
	infoOut      chan<- []*TagInfo
	expire       *expire
	labelsAsTags map[string]string
}

// Detect tries to connect to the ECS metadata API
func (c *ECSFargateCollector) Detect(ctx context.Context, out chan<- []*TagInfo) (CollectionMode, error) {
	var err error

	if !config.IsFeaturePresent(config.ECSFargate) {
		return NoCollection, nil
	}

	if !ecsutil.IsFargateInstance(ctx) {
		return NoCollection, fmt.Errorf("Failed to connect to task metadata API, ECS tagging will not work")
	}

	client, err := ecsmeta.V2()
	if err != nil {
		log.Debugf("error while initializing ECS metadata V2 client: %s", err)
		return NoCollection, err
	}

	c.client = client
	c.infoOut = out
	c.expire, err = newExpire(ecsFargateCollectorName, ecsFargateExpireFreq)
	c.labelsAsTags = retrieveMappingFromConfig("docker_labels_as_tags")

	if err != nil {
		return PullCollection, fmt.Errorf("Failed to instantiate the container expiration process")
	}

	return PullCollection, nil
}

// Pull looks for new containers and computes deletions
func (c *ECSFargateCollector) Pull(ctx context.Context) error {
	taskMeta, err := c.client.GetTask(ctx)
	if err != nil {
		return err
	}
	// Only parse new containers
	updates, err := c.parseMetadata(taskMeta, false)
	if err != nil {
		return err
	}
	c.infoOut <- updates

	expires := c.expire.ComputeExpires()
	if len(expires) > 0 {
		c.infoOut <- expires
	}

	return nil
}

// Fetch parses tags for a container on cache miss. We avoid races with Pull,
// we re-parse the whole list, but don't send updates on other containers.
func (c *ECSFargateCollector) Fetch(ctx context.Context, container string) ([]string, []string, []string, error) {
	taskMeta, err := c.client.GetTask(ctx)
	if err != nil {
		return []string{}, []string{}, []string{}, err
	}
	// Force a full parse to avoid missing the container in a race with Pull
	updates, err := c.parseMetadata(taskMeta, true)
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

func ecsFargateFactory() Collector {
	return &ECSFargateCollector{}
}

func init() {
	registerCollector(ecsFargateCollectorName, ecsFargateFactory, NodeOrchestrator)
}
