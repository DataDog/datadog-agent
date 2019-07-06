// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2019 Datadog, Inc.

// +build docker

package collectors

import (
	"time"

	"github.com/DataDog/datadog-agent/pkg/errors"
	taggerutil "github.com/DataDog/datadog-agent/pkg/tagger/utils"
	"github.com/DataDog/datadog-agent/pkg/util/containers"
	"github.com/DataDog/datadog-agent/pkg/util/ecs"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

const (
	ecsCollectorName = "ecs"
	ecsExpireFreq    = 5 * time.Minute
)

// ECSCollector listen to the ECS agent to get ECS metadata.
// Relies on the DockerCollector to trigger deletions, it's not intended to run standalone
type ECSCollector struct {
	infoOut     chan<- []*TagInfo
	expire      *taggerutil.Expire
	lastExpire  time.Time
	expireFreq  time.Duration
	ecsUtil     *ecs.Util
	clusterName string
}

// Detect tries to connect to the ECS agent
func (c *ECSCollector) Detect(out chan<- []*TagInfo) (CollectionMode, error) {
	ecsUtil, err := ecs.GetUtil()
	if err != nil {
		return NoCollection, err
	}
	c.ecsUtil = ecsUtil
	c.infoOut = out
	c.lastExpire = time.Now()
	c.expireFreq = ecsExpireFreq

	c.expire, err = taggerutil.NewExpire(ecsExpireFreq)
	if err != nil {
		return NoCollection, err
	}

	c.clusterName, err = ecsUtil.GetClusterName()
	if err != nil {
		log.Warnf("Cannot determine ECS cluster name: %s", err)
	}
	return FetchOnlyCollection, nil
}

// Fetch fetches ECS tags
func (c *ECSCollector) Fetch(container string) ([]string, []string, []string, error) {
	_, cID := containers.SplitEntityName(container)
	if len(cID) == 0 {
		return nil, nil, nil, nil
	}

	tasks_list, err := c.ecsUtil.GetTasks()
	if err != nil {
		return []string{}, []string{}, []string{}, err
	}
	updates, err := c.parseTasks(tasks_list, cID)
	if err != nil {
		return []string{}, []string{}, []string{}, err
	}
	c.infoOut <- updates

	// Only run the expire process with the most up to date tasks parsed.
	// Using a go routine as the expire process can be done asynchronously.
	// We do not use the output as the ECSCollector is not meant run in standalone.
	if time.Now().Sub(c.lastExpire) >= c.expireFreq {
		go c.expire.ComputeExpires()
		c.lastExpire = time.Now()
	}

	for _, info := range updates {
		if info.Entity == container {
			return info.LowCardTags, info.OrchestratorCardTags, info.HighCardTags, nil
		}
	}
	// container not found in updates
	return []string{}, []string{}, []string{}, errors.NewNotFound(container)
}

func ecsFactory() Collector {
	return &ECSCollector{}
}

func init() {
	registerCollector(ecsCollectorName, ecsFactory, NodeRuntime)
}
