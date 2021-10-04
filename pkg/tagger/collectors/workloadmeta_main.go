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
	"github.com/DataDog/datadog-agent/pkg/util/fargate"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/datadog-agent/pkg/workloadmeta"
)

const (
	workloadmetaCollectorName = "workloadmeta"
)

type metaStore interface {
	Subscribe(string, *workloadmeta.Filter) chan workloadmeta.EventBundle
	Unsubscribe(chan workloadmeta.EventBundle)
	GetContainer(string) (workloadmeta.Container, error)
}

// WorkloadMetaCollector collects tags from the metadata in the workloadmeta
// store.
type WorkloadMetaCollector struct {
	store metaStore
	out   chan<- []*TagInfo
	stop  chan struct{}

	staticTags        map[string]string
	labelsAsTags      map[string]string
	annotationsAsTags map[string]string
	globLabels        map[string]glob.Glob
	globAnnotations   map[string]glob.Glob
}

// Detect initializes the WorkloadMetaCollector.
func (c *WorkloadMetaCollector) Detect(ctx context.Context, out chan<- []*TagInfo) (CollectionMode, error) {
	c.out = out
	c.stop = make(chan struct{})

	labelsAsTags := config.Datadog.GetStringMapString("kubernetes_pod_labels_as_tags")
	annotationsAsTags := config.Datadog.GetStringMapString("kubernetes_pod_annotations_as_tags")
	c.init(labelsAsTags, annotationsAsTags)

	c.staticTags = fargateStaticTags()

	return StreamCollection, nil
}

func (c *WorkloadMetaCollector) init(labelsAsTags, annotationsAsTags map[string]string) {
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

func fargateStaticTags() map[string]string {
	// fargate (ECS or EKS) does not have host tags, so we need to
	// add static tags to each container manually

	if !fargate.IsFargateInstance(context.TODO()) {
		return nil
	}

	tags := make(map[string]string)

	// DD_TAGS
	for _, tag := range config.GetConfiguredTags(false) {
		tagParts := strings.SplitN(tag, ":", 2)
		if len(tagParts) != 2 {
			log.Warnf("Cannot split tag %s", tag)
			continue
		}
		tags[tagParts[0]] = tagParts[1]
	}

	// EKS Fargate specific tags
	if fargate.IsEKSFargateInstance() {
		node, err := fargate.GetEKSFargateNodename()
		if err != nil {
			tags["eks_fargate_node"] = node
		} else {
			log.Infof("Couldn't build the 'eks_fargate_node' tag: %w", err)
		}

	}

	return tags
}

func init() {
	// NOTE: WorkloadMetaCollector is meant to be used as the single
	// collector, so priority doesn't matter and should be removed entirely
	// after migration is done.
	registerCollector(workloadmetaCollectorName, workloadmetaFactory, NodeRuntime)
}
