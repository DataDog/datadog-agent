// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package collectors

import (
	"context"
	"strings"

	"github.com/gobwas/glob"

	"github.com/DataDog/datadog-agent/comp/core/workloadmeta"
	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/status/health"
	"github.com/DataDog/datadog-agent/pkg/tagger/utils"
	"github.com/DataDog/datadog-agent/pkg/util"
	"github.com/DataDog/datadog-agent/pkg/util/flavor"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes/clustername"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

const (
	workloadmetaCollectorName = "workloadmeta"

	staticSource         = workloadmetaCollectorName + "-static"
	podSource            = workloadmetaCollectorName + "-" + string(workloadmeta.KindKubernetesPod)
	nodeSource           = workloadmetaCollectorName + "-" + string(workloadmeta.KindKubernetesNode)
	taskSource           = workloadmetaCollectorName + "-" + string(workloadmeta.KindECSTask)
	containerSource      = workloadmetaCollectorName + "-" + string(workloadmeta.KindContainer)
	containerImageSource = workloadmetaCollectorName + "-" + string(workloadmeta.KindContainerImageMetadata)
	processSource        = workloadmetaCollectorName + "-" + string(workloadmeta.KindProcess)

	clusterTagNamePrefix = "kube_cluster_name"
)

// CollectorPriorities holds collector priorities
var CollectorPriorities = make(map[string]CollectorPriority)

type processor interface {
	ProcessTagInfo([]*TagInfo)
}

// WorkloadMetaCollector collects tags from the metadata in the workloadmeta
// store.
type WorkloadMetaCollector struct {
	store        workloadmeta.Component
	children     map[string]map[string]struct{}
	tagProcessor processor

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

func (c *WorkloadMetaCollector) initContainerMetaAsTags(labelsAsTags, envAsTags map[string]string) {
	c.containerLabelsAsTags, c.globContainerLabels = utils.InitMetadataAsTags(labelsAsTags)
	c.containerEnvAsTags, c.globContainerEnvLabels = utils.InitMetadataAsTags(envAsTags)
}

func (c *WorkloadMetaCollector) initPodMetaAsTags(labelsAsTags, annotationsAsTags, nsLabelsAsTags map[string]string) {
	c.labelsAsTags, c.globLabels = utils.InitMetadataAsTags(labelsAsTags)
	c.annotationsAsTags, c.globAnnotations = utils.InitMetadataAsTags(annotationsAsTags)
	c.nsLabelsAsTags, c.globNsLabels = utils.InitMetadataAsTags(nsLabelsAsTags)
}

// Run runs the continuous event watching loop and sends new tags to the
// tagger based on the events sent by the workloadmeta.
func (c *WorkloadMetaCollector) Run(ctx context.Context) {
	c.collectStaticGlobalTags(ctx)
	c.stream(ctx)
}

func (c *WorkloadMetaCollector) collectStaticGlobalTags(ctx context.Context) {
	c.staticTags = util.GetStaticTags(ctx)
	if _, exists := c.staticTags[clusterTagNamePrefix]; flavor.GetFlavor() == flavor.ClusterAgent && !exists {
		// If we are running the cluster agent, we want to set the kube_cluster_name tag as a global tag if we are able
		// to read it, for the instances where we are running in an environment where hostname cannot be detected.
		if cluster := clustername.GetClusterNameTagValue(ctx, ""); cluster != "" {
			if c.staticTags == nil {
				c.staticTags = make(map[string]string, 1)
			}
			c.staticTags[clusterTagNamePrefix] = cluster
		}
	}
	if len(c.staticTags) > 0 {
		tags := utils.NewTagList()

		for tag, value := range c.staticTags {
			tags.AddLow(tag, value)
		}

		low, orch, high, standard := tags.Compute()
		c.tagProcessor.ProcessTagInfo([]*TagInfo{
			{
				Source:               staticSource,
				Entity:               GlobalEntityID,
				HighCardTags:         high,
				OrchestratorCardTags: orch,
				LowCardTags:          low,
				StandardTags:         standard,
			},
		})
	}
}

func (c *WorkloadMetaCollector) stream(ctx context.Context) {
	const name = "tagger-workloadmeta"

	health := health.RegisterLiveness(name)
	defer func() {
		err := health.Deregister()
		if err != nil {
			log.Warnf("error de-registering health check: %s", err)
		}
	}()

	ch := c.store.Subscribe(name, workloadmeta.TaggerPriority, nil)

	log.Infof("workloadmeta tagger collector started")

	for {
		select {
		case evBundle, ok := <-ch:
			if !ok {
				return
			}

			c.processEvents(evBundle)

		case <-health.C:

		case <-ctx.Done():
			c.store.Unsubscribe(ch)

			return
		}
	}
}

// NewWorkloadMetaCollector returns a new WorkloadMetaCollector.
func NewWorkloadMetaCollector(ctx context.Context, store workloadmeta.Component, p processor) *WorkloadMetaCollector {
	c := &WorkloadMetaCollector{
		tagProcessor:           p,
		store:                  store,
		children:               make(map[string]map[string]struct{}),
		collectEC2ResourceTags: config.Datadog.GetBool("ecs_collect_resource_tags_ec2"),
	}

	containerLabelsAsTags := mergeMaps(
		retrieveMappingFromConfig("docker_labels_as_tags"),
		retrieveMappingFromConfig("container_labels_as_tags"),
	)
	// Adding new environment variables require adding them to pkg/util/containers/env_vars_filter.go
	containerEnvAsTags := mergeMaps(
		retrieveMappingFromConfig("docker_env_as_tags"),
		retrieveMappingFromConfig("container_env_as_tags"),
	)
	c.initContainerMetaAsTags(containerLabelsAsTags, containerEnvAsTags)

	labelsAsTags := config.Datadog.GetStringMapString("kubernetes_pod_labels_as_tags")
	annotationsAsTags := config.Datadog.GetStringMapString("kubernetes_pod_annotations_as_tags")
	nsLabelsAsTags := config.Datadog.GetStringMapString("kubernetes_namespace_labels_as_tags")
	c.initPodMetaAsTags(labelsAsTags, annotationsAsTags, nsLabelsAsTags)

	return c
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
	CollectorPriorities[podSource] = NodeOrchestrator
	CollectorPriorities[taskSource] = NodeOrchestrator
	CollectorPriorities[containerSource] = NodeRuntime
	CollectorPriorities[containerImageSource] = NodeRuntime
}
