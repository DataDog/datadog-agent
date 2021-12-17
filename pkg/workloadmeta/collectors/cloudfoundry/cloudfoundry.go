// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package cloudfoundry

import (
	"context"
	"fmt"
	"time"

	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/errors"
	"github.com/DataDog/datadog-agent/pkg/util/cloudproviders/cloudfoundry"
	"github.com/DataDog/datadog-agent/pkg/util/clusteragent"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/datadog-agent/pkg/workloadmeta"
	"github.com/DataDog/datadog-agent/pkg/workloadmeta/collectors/util"
)

const (
	collectorID   = "cloudfoundry"
	componentName = "workloadmeta-cloudfoundry"
	expireFreq    = 15 * time.Second
)

type collector struct {
	store workloadmeta.Store

	expire              *util.Expire
	dcaClient           clusteragent.DCAClientInterface
	gardenUtil          cloudfoundry.GardenUtilInterface
	clusterAgentEnabled bool
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

	c.store = store
	c.expire = util.NewExpire(expireFreq)

	// Detect if we're on a compute VM by trying to connect to the local garden API
	var err error
	c.gardenUtil, err = cloudfoundry.GetGardenUtil()
	if err != nil {
		return err
	}

	// if DCA is enabled and can't communicate with the DCA, let the tagger retry.
	if config.Datadog.GetBool("cluster_agent.enabled") {
		c.dcaClient, err = clusteragent.GetClusterAgentClient()
		if err != nil {
			return fmt.Errorf("Could not initialise the communication with the cluster agent: %w", err)
		}
		c.clusterAgentEnabled = true
	} else {
		log.Debug("Cluster agent not enabled, tagging CF app with container id only")
	}

	return nil
}

func (c *collector) Pull(ctx context.Context) error {
	var (
		events []workloadmeta.CollectorEvent
	)

	now := time.Now()

	if c.clusterAgentEnabled {
		nodeName := config.Datadog.GetString("bosh_id")
		gardenContainerTags, err := c.dcaClient.GetCFAppsMetadataForNode(nodeName)
		if err != nil {
			return err
		}

		events = make([]workloadmeta.CollectorEvent, 0, len(gardenContainerTags))

		for id, tags := range gardenContainerTags {
			entityID := workloadmeta.EntityID{
				Kind: workloadmeta.KindGardenContainer,
				ID:   id,
			}

			c.expire.Update(entityID, now)

			events = append(events, workloadmeta.CollectorEvent{
				Type:   workloadmeta.EventTypeSet,
				Source: workloadmeta.SourceCloudfoundry,
				Entity: &workloadmeta.GardenContainer{
					EntityID: entityID,
					EntityMeta: workloadmeta.EntityMeta{
						Name: id,
					},
					Tags: tags,
				},
			})
		}

	} else {
		gardenContainers, err := c.gardenUtil.GetGardenContainers()
		if err != nil {
			return fmt.Errorf("cannot get container list from local garden API: %w", err)
		}

		events = make([]workloadmeta.CollectorEvent, 0, len(gardenContainers))

		for _, gardenContainer := range gardenContainers {
			id := gardenContainer.Handle()

			entityID := workloadmeta.EntityID{
				Kind: workloadmeta.KindGardenContainer,
				ID:   id,
			}

			c.expire.Update(entityID, now)

			events = append(events, workloadmeta.CollectorEvent{
				Type:   workloadmeta.EventTypeSet,
				Source: workloadmeta.SourceCloudfoundry,
				Entity: &workloadmeta.GardenContainer{
					EntityID: entityID,
					EntityMeta: workloadmeta.EntityMeta{
						Name: id,
					},
					Tags: []string{
						fmt.Sprintf("%s:%s", cloudfoundry.ContainerNameTagKey, gardenContainer.Handle()),
						fmt.Sprintf("%s:%s", cloudfoundry.AppInstanceGUIDTagKey, gardenContainer.Handle()),
					},
				},
			})
		}
	}

	expires := c.expire.ComputeExpires()
	for _, expired := range expires {
		events = append(events, workloadmeta.CollectorEvent{
			Type:   workloadmeta.EventTypeUnset,
			Source: workloadmeta.SourceCloudfoundry,
			Entity: expired,
		})
	}

	c.store.Notify(events)

	return nil
}
