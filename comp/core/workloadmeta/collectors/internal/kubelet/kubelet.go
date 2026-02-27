// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubelet

// Package kubelet implements the kubelet Workloadmeta collector.
package kubelet

import (
	"context"
	"time"

	"go.uber.org/fx"

	"github.com/DataDog/datadog-agent/comp/core/config"
	"github.com/DataDog/datadog-agent/comp/core/workloadmeta/collectors/util"
	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	"github.com/DataDog/datadog-agent/pkg/config/env"
	"github.com/DataDog/datadog-agent/pkg/errors"
	"github.com/DataDog/datadog-agent/pkg/util/containers"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes/kubelet"
)

const (
	collectorID             = "kubelet"
	componentName           = "workloadmeta-kubelet"
	expireFreq              = 15 * time.Second
	kubeletConfigExpireFreq = 20 * time.Minute // It's unlikely that the kubelet config would change frequently

)

type dependencies struct {
	fx.In

	Config config.Component
}

type collector struct {
	id                         string
	catalog                    workloadmeta.AgentType
	store                      workloadmeta.Component
	collectEphemeralContainers bool

	kubeUtil             kubelet.KubeUtilInterface
	lastSeenPodUIDs      map[string]time.Time
	lastSeenContainerIDs map[string]time.Time

	// These fields are used to pull the kubelet config
	kubeletConfigLastExpire time.Time
}

// NewCollector returns a kubelet CollectorProvider that instantiates its collector
func NewCollector(deps dependencies) (workloadmeta.CollectorProvider, error) {
	return workloadmeta.CollectorProvider{
		Collector: &collector{
			id:                         collectorID,
			catalog:                    workloadmeta.NodeAgent | workloadmeta.ProcessAgent,
			collectEphemeralContainers: deps.Config.GetBool("include_ephemeral_containers"),
		},
	}, nil
}

// GetFxOptions returns the FX framework options for the collector
func GetFxOptions() fx.Option {
	return fx.Provide(NewCollector)
}

func (c *collector) Start(_ context.Context, store workloadmeta.Component) error {
	if !env.IsFeaturePresent(env.Kubernetes) {
		return errors.NewDisabled(componentName, "Agent is not running on Kubernetes")
	}

	c.store = store

	var err error
	c.kubeUtil, err = kubelet.GetKubeUtil()
	if err != nil {
		return err
	}
	c.lastSeenPodUIDs = make(map[string]time.Time)
	c.lastSeenContainerIDs = make(map[string]time.Time)

	return nil
}

func (c *collector) Pull(ctx context.Context) error {
	return c.pullFromKubelet(ctx)
}

func (c *collector) pullKubeletConfig(ctx context.Context) (workloadmeta.CollectorEvent, error) {
	rawKubeletConfig, config, err := c.kubeUtil.GetConfig(ctx)
	if err != nil {
		return workloadmeta.CollectorEvent{}, err
	}

	wmetaConfigDocument := workloadmeta.KubeletConfigDocument{
		KubeletConfig: workloadmeta.KubeletConfigSpec{
			CPUManagerPolicy: config.KubeletConfig.CPUManagerPolicy,
		},
	}

	nodeName, _ := c.kubeUtil.GetNodename(ctx)

	return workloadmeta.CollectorEvent{
		Type:   workloadmeta.EventTypeSet,
		Source: workloadmeta.SourceNodeOrchestrator,
		Entity: &workloadmeta.Kubelet{
			EntityID: workloadmeta.EntityID{
				ID:   workloadmeta.KubeletID,
				Kind: workloadmeta.KindKubelet,
			},
			EntityMeta: workloadmeta.EntityMeta{
				Name: workloadmeta.KubeletName,
			},
			ConfigDocument: wmetaConfigDocument,
			RawConfig:      rawKubeletConfig,
			NodeName:       nodeName,
		},
	}, nil
}

func (c *collector) pullFromKubelet(ctx context.Context) error {
	events := []workloadmeta.CollectorEvent{}

	podList, err := c.kubeUtil.GetLocalPodListWithMetadata(ctx)
	if err != nil {
		return err
	}

	// Pull the kubelet config every kubeletConfigExpireFreq
	// This needs to be before the pod list
	if time.Since(c.kubeletConfigLastExpire) > kubeletConfigExpireFreq {
		configEvent, err := c.pullKubeletConfig(ctx)
		if err == nil {
			events = append(events, configEvent)
			// only update last expiry if the config was successfully retrieved
			c.kubeletConfigLastExpire = time.Now()
		}
	}

	if podList == nil {
		c.store.Notify(events)
		return nil
	}

	events = append(events, util.ParseKubeletPods(podList.Items, c.collectEphemeralContainers, c.store)...)

	// Mark return pods and containers as seen now
	now := time.Now()
	for _, pod := range podList.Items {
		if pod.Metadata.UID != "" {
			c.lastSeenPodUIDs[pod.Metadata.UID] = now
		}
		for _, container := range pod.Status.GetAllContainers() {
			if container.ID != "" {
				c.lastSeenContainerIDs[container.ID] = now
			}
		}
	}

	expireEvents := c.eventsForExpiredEntities(now)
	events = append(events, expireEvents...)

	// Report expired pod count. This is needed by the Kubelet check
	events = append(events, eventForKubeletMetrics(podList.ExpiredCount))

	c.store.Notify(events)

	return nil
}

// eventsForExpiredEntities returns a list of workloadmeta.CollectorEvent
// containing events for expired pods and containers.
// The old implementation based on a pod watcher expired pods and containers
// at a set frequency (expireFreq). Instead, we could delete them on every
// pull by keeping a list of items from the last pull and removing those
// not seen in the current one. That would be simpler and likely safe,
// but to avoid unexpected issues, we’ll keep the old behavior for now.
func (c *collector) eventsForExpiredEntities(now time.Time) []workloadmeta.CollectorEvent {
	var events []workloadmeta.CollectorEvent

	// Find expired pods
	var expiredPodUIDs []string
	for uid, lastSeen := range c.lastSeenPodUIDs {
		if now.Sub(lastSeen) > expireFreq {
			expiredPodUIDs = append(expiredPodUIDs, uid)
			delete(c.lastSeenPodUIDs, uid)
		}
	}

	// Find expired containers
	var expiredContainerIDs []string
	for containerID, lastSeen := range c.lastSeenContainerIDs {
		if now.Sub(lastSeen) > expireFreq {
			expiredContainerIDs = append(expiredContainerIDs, containerID)
			delete(c.lastSeenContainerIDs, containerID)
		}
	}

	events = append(events, parseExpiredPods(expiredPodUIDs)...)
	events = append(events, parseExpiredContainers(expiredContainerIDs)...)

	return events
}

func parseExpiredPods(expiredPodUIDs []string) []workloadmeta.CollectorEvent {
	events := make([]workloadmeta.CollectorEvent, 0, len(expiredPodUIDs))

	for _, uid := range expiredPodUIDs {
		entity := &workloadmeta.KubernetesPod{
			EntityID: workloadmeta.EntityID{
				Kind: workloadmeta.KindKubernetesPod,
				ID:   uid,
			},
			FinishedAt: time.Now(),
		}

		events = append(events, workloadmeta.CollectorEvent{
			Source: workloadmeta.SourceNodeOrchestrator,
			Type:   workloadmeta.EventTypeUnset,
			Entity: entity,
		})
	}

	return events
}

func parseExpiredContainers(expiredContainerIDs []string) []workloadmeta.CollectorEvent {
	events := make([]workloadmeta.CollectorEvent, 0, len(expiredContainerIDs))

	for _, containerID := range expiredContainerIDs {
		// Split the container ID to get just the ID part (remove runtime prefix like "docker://")
		_, id := containers.SplitEntityName(containerID)

		entity := &workloadmeta.Container{
			EntityID: workloadmeta.EntityID{
				Kind: workloadmeta.KindContainer,
				ID:   id,
			},
		}

		events = append(events, workloadmeta.CollectorEvent{
			Source: workloadmeta.SourceNodeOrchestrator,
			Type:   workloadmeta.EventTypeUnset,
			Entity: entity,
		})
	}

	return events
}

func (c *collector) GetID() string {
	return c.id
}

func (c *collector) GetTargetCatalog() workloadmeta.AgentType {
	return c.catalog
}

func eventForKubeletMetrics(expiredPodCount int) workloadmeta.CollectorEvent {
	kubeletMetrics := &workloadmeta.KubeletMetrics{
		EntityID: workloadmeta.EntityID{
			Kind: workloadmeta.KindKubeletMetrics,
			ID:   workloadmeta.KubeletMetricsID,
		},
		ExpiredPodCount: expiredPodCount,
	}

	return workloadmeta.CollectorEvent{
		Type:   workloadmeta.EventTypeSet,
		Entity: kubeletMetrics,
	}
}
