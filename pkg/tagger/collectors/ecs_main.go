// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018 Datadog, Inc.

// +build docker

package collectors

import (
	"fmt"
	"time"

	taggerutil "github.com/DataDog/datadog-agent/pkg/tagger/utils"
	ecsutil "github.com/DataDog/datadog-agent/pkg/util/ecs"
)

const (
	ecsCollectorName = "ecs"
	ecsExpireFreq    = 5 * time.Minute
)

// ECSCollector listen to the ECS agent to get ECS metadata.
// Relies on the DockerCollector to trigger deletions, it's not intended to run standalone
type ECSCollector struct {
	infoOut    chan<- []*TagInfo
	expire     *taggerutil.Expire
	lastExpire time.Time
	expireFreq time.Duration
}

// Detect tries to connect to the ECS agent
func (c *ECSCollector) Detect(out chan<- []*TagInfo) (CollectionMode, error) {
	var err error
	if ecsutil.IsInstance() {
		c.infoOut = out
		c.lastExpire = time.Now()
		c.expireFreq = ecsExpireFreq

		c.expire, err = taggerutil.NewExpire(ecsExpireFreq)

		if err != nil {
			return NoCollection, err
		}
		return FetchOnlyCollection, nil
	}
	return NoCollection, fmt.Errorf("cannot find ECS agent")
}

// Fetch fetches ECS tags
func (c *ECSCollector) Fetch(container string) ([]string, []string, error) {

	tasks_list, err := ecsutil.GetTasks()
	if err != nil {
		return []string{}, []string{}, err
	}
	updates, err := c.parseTasks(tasks_list)
	if err != nil {
		return []string{}, []string{}, err
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
			return info.LowCardTags, info.HighCardTags, nil
		}
	}
	// container not found in updates
	return []string{}, []string{}, ErrNotFound
}

func ecsFactory() Collector {
	return &ECSCollector{}
}

func init() {
	registerCollector(ecsCollectorName, ecsFactory, LowPriority)
}
