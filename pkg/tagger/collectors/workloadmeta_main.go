// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package collectors

import (
	"context"
	"strings"

	"github.com/gobwas/glob"

	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/status/health"
	"github.com/DataDog/datadog-agent/pkg/tagger/utils"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/datadog-agent/pkg/workloadmeta"
)

const (
	workloadmetaCollectorName = "workloadmeta"

	podSource       = workloadmetaCollectorName + "-kubernetes_pod"
	containerSource = workloadmetaCollectorName + "-container"
)

type metaStore interface {
	Subscribe(string, *workloadmeta.Filter) chan workloadmeta.EventBundle
	Unsubscribe(chan workloadmeta.EventBundle)
	GetContainer(string) (*workloadmeta.Container, error)
}

// WorkloadMetaCollector collects tags from the metadata in the workloadmeta
// store.
type WorkloadMetaCollector struct {
	store metaStore
	out   chan<- []*TagInfo
	stop  chan struct{}

	containerEnvAsTags    map[string]string
	containerLabelsAsTags map[string]string

	labelsAsTags      map[string]string
	annotationsAsTags map[string]string
	globLabels        map[string]glob.Glob
	globAnnotations   map[string]glob.Glob
}

// Detect initializes the WorkloadMetaCollector.
func (c *WorkloadMetaCollector) Detect(ctx context.Context, out chan<- []*TagInfo) (CollectionMode, error) {
	c.out = out
	c.stop = make(chan struct{})

	containerLabelsAsTags := retrieveMappingFromConfig("docker_labels_as_tags")
	containerEnvAsTags := mergeMaps(
		retrieveMappingFromConfig("docker_env_as_tags"),
		retrieveMappingFromConfig("container_env_as_tags"),
	)
	c.initContainerMetaAsTags(containerLabelsAsTags, containerEnvAsTags)

	labelsAsTags := config.Datadog.GetStringMapString("kubernetes_pod_labels_as_tags")
	annotationsAsTags := config.Datadog.GetStringMapString("kubernetes_pod_annotations_as_tags")
	c.initPodMetaAsTags(labelsAsTags, annotationsAsTags)

	return StreamCollection, nil
}

func (c *WorkloadMetaCollector) initContainerMetaAsTags(labelsAsTags, envAsTags map[string]string) {
	c.containerLabelsAsTags = make(map[string]string)
	for label, tag := range labelsAsTags {
		c.containerLabelsAsTags[strings.ToLower(label)] = tag
	}

	c.containerEnvAsTags = make(map[string]string)
	for label, tag := range envAsTags {
		c.containerEnvAsTags[strings.ToLower(label)] = tag
	}
}

func (c *WorkloadMetaCollector) initPodMetaAsTags(labelsAsTags, annotationsAsTags map[string]string) {
	c.labelsAsTags, c.globLabels = utils.InitMetadataAsTags(labelsAsTags)
	c.annotationsAsTags, c.globAnnotations = utils.InitMetadataAsTags(annotationsAsTags)
}

// Stream runs the continuous event watching loop and sends new tags to the
// tagger based on the events sent by the workloadmeta.
func (c *WorkloadMetaCollector) Stream() error {
	const name = "tagger-workloadmeta"
	health := health.RegisterLiveness(name)

	ch := c.store.Subscribe(name, nil)

	for {
		select {
		case evBundle := <-ch:
			c.processEvents(evBundle)

		case <-health.C:

		case <-c.stop:
			err := health.Deregister()
			if err != nil {
				log.Warnf("error de-registering health check: %s", err)
			}

			c.store.Unsubscribe(ch)

			return nil
		}
	}
}

// Stop shuts down the WorkloadMetaCollector.
func (c *WorkloadMetaCollector) Stop() error {
	c.stop <- struct{}{}
	return nil
}

// Fetch is a no-op in the WorkloadMetaCollector to prevent expensive and
// race-condition prone forcing of pulls from upstream collectors.  Since
// workloadmeta.Store will eventually own notifying all downstream consumers,
// this codepath should never trigger anyway.
func (c *WorkloadMetaCollector) Fetch(ctx context.Context, entity string) ([]string, []string, []string, error) {
	return nil, nil, nil, nil
}

func workloadmetaFactory() Collector {
	return &WorkloadMetaCollector{
		store: workloadmeta.GetGlobalStore(),
	}
}

func init() {
	// NOTE: WorkloadMetaCollector is meant to be used as the single
	// collector, while emitting TagInfos with different sources. This is
	// different from the way older collectors work, where they have a
	// single priority. Until they all go away, we need to register the
	// collector with a dummy priority, then set the priority for the
	// actual sources we emit manually

	registerCollector(workloadmetaCollectorName, workloadmetaFactory, NodeRuntime)

	CollectorPriorities[podSource] = NodeOrchestrator
	CollectorPriorities[containerSource] = NodeRuntime
}
