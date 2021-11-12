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

	podSource       = workloadmetaCollectorName + "-" + string(workloadmeta.KindKubernetesPod)
	taskSource      = workloadmetaCollectorName + "-" + string(workloadmeta.KindECSTask)
	containerSource = workloadmetaCollectorName + "-" + string(workloadmeta.KindContainer)
)

// WorkloadMetaCollector collects tags from the metadata in the workloadmeta
// store.
type WorkloadMetaCollector struct {
	store    workloadmeta.Store
	children map[string]map[string]struct{}
	out      chan<- []*TagInfo
	stop     chan struct{}

	containerEnvAsTags    map[string]string
	containerLabelsAsTags map[string]string

	staticTags             map[string]string
	labelsAsTags           map[string]string
	annotationsAsTags      map[string]string
	nsLabelsAsTags         map[string]string
	globLabels             map[string]glob.Glob
	globAnnotations        map[string]glob.Glob
	globNsLabels           map[string]glob.Glob
	globContainerLabels    map[string]glob.Glob
	globContainerEnvLabels map[string]glob.Glob

	collectEC2ResourceTags bool
}

// Detect initializes the WorkloadMetaCollector.
func (c *WorkloadMetaCollector) Detect(ctx context.Context, out chan<- []*TagInfo) (CollectionMode, error) {
	c.out = out
	c.stop = make(chan struct{})
	c.children = make(map[string]map[string]struct{})
	c.collectEC2ResourceTags = config.Datadog.GetBool("ecs_collect_resource_tags_ec2")

	containerLabelsAsTags := mergeMaps(
		retrieveMappingFromConfig("docker_labels_as_tags"),
		retrieveMappingFromConfig("container_labels_as_tags"),
	)
	containerEnvAsTags := mergeMaps(
		retrieveMappingFromConfig("docker_env_as_tags"),
		retrieveMappingFromConfig("container_env_as_tags"),
	)
	c.initContainerMetaAsTags(containerLabelsAsTags, containerEnvAsTags)

	labelsAsTags := config.Datadog.GetStringMapString("kubernetes_pod_labels_as_tags")
	annotationsAsTags := config.Datadog.GetStringMapString("kubernetes_pod_annotations_as_tags")
	nsLabelsAsTags := config.Datadog.GetStringMapString("kubernetes_namespace_labels_as_tags")
	c.initPodMetaAsTags(labelsAsTags, annotationsAsTags, nsLabelsAsTags)

	c.staticTags = fargateStaticTags(ctx)

	return StreamCollection, nil
}

func (c *WorkloadMetaCollector) initContainerMetaAsTags(labelsAsTags, envAsTags map[string]string) {
	c.containerLabelsAsTags, c.globContainerLabels = utils.InitMetadataAsTags(labelsAsTags)
	c.containerEnvAsTags, c.globContainerEnvLabels = utils.InitMetadataAsTags(envAsTags)
}

func (c *WorkloadMetaCollector) initPodMetaAsTags(labelsAsTags, annotationsAsTags, nsLabelsAsTags map[string]string) {
	c.labelsAsTags, c.globLabels = utils.InitMetadataAsTags(labelsAsTags)
	c.annotationsAsTags, c.globAnnotations = utils.InitMetadataAsTags(annotationsAsTags)
	c.nsLabelsAsTags, c.globNsLabels = utils.InitMetadataAsTags(nsLabelsAsTags)
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

func workloadmetaFactory() Collector {
	return &WorkloadMetaCollector{
		store: workloadmeta.GetGlobalStore(),
	}
}

func fargateStaticTags(ctx context.Context) map[string]string {
	// fargate (ECS or EKS) does not have host tags, so we need to
	// add static tags to each container manually

	if !fargate.IsFargateInstance(ctx) {
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

// retrieveMappingFromConfig gets a stringmapstring config key and
// lowercases all map keys to make envvar and yaml sources consistent
func retrieveMappingFromConfig(configKey string) map[string]string {
	labelsList := config.Datadog.GetStringMapString(configKey)
	for label, value := range labelsList {
		delete(labelsList, label)
		labelsList[strings.ToLower(label)] = value
	}

	return labelsList
}

// mergeMaps merges two maps, in case of conflict the first argument is prioritized
func mergeMaps(first, second map[string]string) map[string]string {
	for k, v := range second {
		if _, found := first[k]; !found {
			first[k] = v
		}
	}

	return first
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
	CollectorPriorities[taskSource] = NodeOrchestrator
	CollectorPriorities[containerSource] = NodeRuntime
}
