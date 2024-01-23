// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//nolint:revive // TODO(PLINT) Fix revive linter
package containertagger

import (
	"context"
	"fmt"
	"strings"
	"time"

	"code.cloudfoundry.org/garden"

	"github.com/DataDog/datadog-agent/comp/core/workloadmeta"
	hostMetadataUtils "github.com/DataDog/datadog-agent/comp/metadata/host/hostimpl/utils"
	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/tagger/utils"
	"github.com/DataDog/datadog-agent/pkg/util/cloudproviders/cloudfoundry"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

const (
	componentName = "cloudfoundry-container-tagger"
)

// ContainerTagger is a simple component that injects host tags and CAPI metadata
// into cloudfoundry containers. It listens to container collection events from
// the workloadmeta store and injects tags accordingly if it detects a diff
// with the previously injected tags.
type ContainerTagger struct {
	gardenUtil            cloudfoundry.GardenUtilInterface
	store                 workloadmeta.Component
	seen                  map[string]struct{}
	tagsHashByContainerID map[string]string
	retryCount            int
	retryInterval         time.Duration
}

// NewContainerTagger creates a new container tagger.
// Call Start to start the container tagger.
func NewContainerTagger() (*ContainerTagger, error) {
	gu, err := cloudfoundry.GetGardenUtil()
	if err != nil {
		return nil, err
	}

	retryCount := config.Datadog.GetInt("cloud_foundry_container_tagger.retry_count")
	retryInterval := time.Second * time.Duration(config.Datadog.GetInt("cloud_foundry_container_tagger.retry_interval"))

	return &ContainerTagger{
		gardenUtil: gu,
		// TODO)components): stop using global and rely instead on injected workloadmeta component.
		store:                 workloadmeta.GetGlobalStore(),
		seen:                  make(map[string]struct{}),
		tagsHashByContainerID: make(map[string]string),
		retryCount:            retryCount,
		retryInterval:         retryInterval,
	}, nil
}

// Start starts the container tagger.
// Cancel the context to stop the container tagger.
func (c *ContainerTagger) Start(ctx context.Context) {
	go func() {
		filterParams := workloadmeta.FilterParams{
			Kinds:     []workloadmeta.Kind{workloadmeta.KindContainer},
			Source:    workloadmeta.SourceClusterOrchestrator,
			EventType: workloadmeta.EventTypeAll,
		}
		filter := workloadmeta.NewFilter(&filterParams)

		ch := c.store.Subscribe(componentName, workloadmeta.NormalPriority, filter)
		defer c.store.Unsubscribe(ch)
		for {
			select {
			case bundle, ok := <-ch:
				if !ok {
					return
				}

				// Acknowledge the evBundle to indicate that the Store can proceed to the next subscriber
				bundle.Acknowledge()

				for _, evt := range bundle.Events {
					err := c.processEvent(ctx, evt)
					if err != nil {
						log.Warnf("%v", err)
					}
				}
			case <-ctx.Done():
				return
			}
		}
	}()
	log.Infof("CloudFoundry container tagger successfully started")
}

func (c *ContainerTagger) processEvent(ctx context.Context, evt workloadmeta.Event) error {
	entity := evt.Entity
	containerID := entity.GetID().ID
	if evt.Type == workloadmeta.EventTypeSet {
		eventTimestamp := time.Now().UnixNano()
		storeContainer := entity.(*workloadmeta.Container)
		eventID := fmt.Sprintf("%s-%d", containerID, eventTimestamp)
		log.Debugf("Processing Event (id %s): %+v", eventID, storeContainer)

		// extract tags
		hostTags := hostMetadataUtils.GetHostTags(ctx, true, config.Datadog)
		tags := storeContainer.CollectorTags
		tags = append(tags, hostTags.System...)
		tags = append(tags, hostTags.GoogleCloudPlatform...)

		// hashing tags to keep track of containers tags
		// as they contain the container_id as `app_instance_guid`
		tagsHash := utils.ComputeTagsHash(tags)

		// will be useful for deletion
		c.tagsHashByContainerID[containerID] = tagsHash

		// check if the tags were already injected
		if _, exist := c.seen[tagsHash]; exist {
			return nil
		}

		// mark as seen
		c.seen[tagsHash] = struct{}{}

		container, err := c.gardenUtil.GetContainer(containerID)
		if err != nil {
			return fmt.Errorf("error retrieving container %s from the garden API: %v", containerID, err)
		}

		go func() {
			var exitCode int
			var err error
			for attempt := 1; attempt <= c.retryCount; attempt++ {
				log.Infof("Updating tags in container `%s` attempt #%d", containerID, attempt)
				log.Debugf("Update attempt #%d for event %s", attempt, eventID)
				exitCode, err = updateTagsInContainer(container, tags)
				if err != nil {
					log.Warnf("Error running a process inside container `%s`: %v", containerID, err)
				} else if exitCode == 0 {
					log.Infof("Successfully updated tags in container `%s`", containerID)
					return
				}
				log.Debugf("Process for container '%s' exited with code: %d", containerID, exitCode)
				time.Sleep(c.retryInterval)
			}
			log.Debugf("Could not inject tags into container '%s' exit code is: %d", containerID, exitCode)
		}()

	} else if evt.Type == workloadmeta.EventTypeUnset {
		hash := c.tagsHashByContainerID[containerID]
		delete(c.seen, hash)
		delete(c.tagsHashByContainerID, containerID)
	}
	return nil
}

// updateTagsInContainer runs a script inside the container that handles updating the agent with the given tags
func updateTagsInContainer(container garden.Container, tags []string) (int, error) {
	//nolint:revive // TODO(PLINT) Fix revive linter
	shell_path := config.Datadog.GetString("cloud_foundry_container_tagger.shell_path")
	process, err := container.Run(garden.ProcessSpec{
		Path: shell_path,
		Args: []string{"/home/vcap/app/.datadog/scripts/update_agent_config.sh"},
		User: "vcap",
		Env:  []string{fmt.Sprintf("DD_NODE_AGENT_TAGS=%s", strings.Join(tags, ","))},
	}, garden.ProcessIO{})
	if err != nil {
		return -1, err
	}
	return process.Wait()
}
