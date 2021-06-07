// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// +build docker

package collectors

import (
	"fmt"
	"time"

	"github.com/DataDog/datadog-agent/pkg/errors"
	"github.com/DataDog/datadog-agent/pkg/tagger/utils"
	taggerutil "github.com/DataDog/datadog-agent/pkg/tagger/utils"
	"github.com/DataDog/datadog-agent/pkg/util/log"

	ecsmeta "github.com/DataDog/datadog-agent/pkg/util/ecs/metadata"
	v1 "github.com/DataDog/datadog-agent/pkg/util/ecs/metadata/v1"
	v3 "github.com/DataDog/datadog-agent/pkg/util/ecs/metadata/v3"
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
	metaV1      *v1.Client
	clusterName string
}

// We emulate Detect to be always successful
func (c *ECSCollector) Detect(out chan<- []*TagInfo) (CollectionMode, error) {
	return FetchOnlyCollection, nil
}

// Emulate Fetch to fail allways with timeout
func (c *ECSCollector) Fetch(entity string) ([]string, []string, []string, error) {
  log.Info("-------Fetch tags----------")
  //Here we emulated timeout for failed fetchContainerTaskWithTagsV3
  time.Sleep(500 * time.Millisecond)
  err := errors.NewPartial(entity)
  return []string{}, []string{}, []string{}, err
}

func addTagsForContainer(containerID string, tags *utils.TagList) error {
	task, err := fetchContainerTaskWithTagsV3(containerID)
	if err != nil {
		return fmt.Errorf("Unable to get resource tags for container %s: %w", containerID, err)
	}

	addResourceTags(tags, task.ContainerInstanceTags)
	addResourceTags(tags, task.TaskTags)

	return nil
}

func fetchContainerTaskWithTagsV3(containerID string) (*v3.Task, error) {
	metaV3, err := ecsmeta.V3(containerID)
	if err != nil {
		return nil, fmt.Errorf("unable to initialize client for metadata v3 API: %s", err)
	}
	task, err := metaV3.GetTaskWithTags()
	if err != nil {
		return nil, fmt.Errorf("failed to get task with tags from metadata v3 API: %s", err)
	}
	return task, nil
}

func ecsFactory() Collector {
	return &ECSCollector{}
}

func init() {
	registerCollector(ecsCollectorName, ecsFactory, NodeRuntime)
}
