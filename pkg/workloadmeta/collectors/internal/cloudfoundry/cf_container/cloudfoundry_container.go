// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package cloudfoundry_container

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/errors"
	"github.com/DataDog/datadog-agent/pkg/util/cloudproviders/cloudfoundry"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/datadog-agent/pkg/workloadmeta"
)

const (
	collectorID             = "cloudfoundry-container"
	componentName           = "workloadmeta-cloudfoundry-container"
	sharedNodeAgentTagsFile = "/home/vcap/app/.datadog/node_agent_tags.txt"
)

type collector struct {
	store    workloadmeta.Store
	nodeName string
}

func init() {
	workloadmeta.RegisterCollector(collectorID, func() workloadmeta.Collector {
		return &collector{}
	})
}

func (c *collector) Start(ctx context.Context, store workloadmeta.Store) error {
	if !config.IsFeaturePresent(config.CloudFoundry) {
		return errors.NewDisabled(componentName, "Agent is not running on CloudFoundry")
	}

	// Detect if we're on a PCF container
	if !config.Datadog.GetBool("cloud_foundry_buildpack") {
		return errors.NewDisabled(componentName, "Agent is not running on a CloudFoundry container")
	}

	c.store = store

	// In PCF the container_id is the hostname
	containerHostname, err := os.Hostname()
	if err != nil {
		return err
	}
	c.nodeName = containerHostname
	return nil
}

func (c *collector) Pull(ctx context.Context) error {
	events := make([]workloadmeta.CollectorEvent, 0, 1)
	entityID := workloadmeta.EntityID{
		Kind: workloadmeta.KindContainer,
		ID:   c.nodeName,
	}
	containerEntity := &workloadmeta.Container{
		EntityID: entityID,
		EntityMeta: workloadmeta.EntityMeta{
			Name: c.nodeName,
		},
		Runtime: workloadmeta.ContainerRuntimeGarden,
	}

	containerEntity.CollectorTags = []string{
		fmt.Sprintf("%s:%s", cloudfoundry.ContainerNameTagKey, c.nodeName),
		fmt.Sprintf("%s:%s", cloudfoundry.AppInstanceGUIDTagKey, c.nodeName),
	}

	// read shared node tags file if it exists
	sharedNodeTagsBytes, err := os.ReadFile(sharedNodeAgentTagsFile)
	if err != nil {
		log.Errorf("Error reading shared node agent tags file under '%s': %v", sharedNodeAgentTagsFile, err)
	} else {
		// TODO: handle json tags
		sharedNodeTags := strings.Split(string(sharedNodeTagsBytes), ",")
		containerEntity.CollectorTags = append(containerEntity.CollectorTags, sharedNodeTags...)
	}

	events = append(events, workloadmeta.CollectorEvent{
		Type:   workloadmeta.EventTypeSet,
		Source: workloadmeta.SourceClusterOrchestrator,
		Entity: containerEntity,
	})

	c.store.Notify(events)
	return nil
}
