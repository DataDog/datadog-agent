// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package collectors

import (
	"context"
	"strings"

	"github.com/gobwas/glob"

	"github.com/DataDog/datadog-agent/comp/core/config"
	"github.com/DataDog/datadog-agent/comp/core/tagger/common"
	k8smetadata "github.com/DataDog/datadog-agent/comp/core/tagger/k8s_metadata"
	"github.com/DataDog/datadog-agent/comp/core/tagger/taglist"
	"github.com/DataDog/datadog-agent/comp/core/tagger/types"
	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	configutils "github.com/DataDog/datadog-agent/pkg/config/utils"
	"github.com/DataDog/datadog-agent/pkg/status/health"
	"github.com/DataDog/datadog-agent/pkg/util"
	"github.com/DataDog/datadog-agent/pkg/util/flavor"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes/clustername"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

const (
	workloadmetaCollectorName = "workloadmeta"

	staticSource         = workloadmetaCollectorName + "-static"
	podSource            = workloadmetaCollectorName + "-" + string(workloadmeta.KindKubernetesPod)
	taskSource           = workloadmetaCollectorName + "-" + string(workloadmeta.KindECSTask)
	containerSource      = workloadmetaCollectorName + "-" + string(workloadmeta.KindContainer)
	containerImageSource = workloadmetaCollectorName + "-" + string(workloadmeta.KindContainerImageMetadata)
	processSource        = workloadmetaCollectorName + "-" + string(workloadmeta.KindProcess)
	kubeMetadataSource   = workloadmetaCollectorName + "-" + string(workloadmeta.KindKubernetesMetadata)
	deploymentSource     = workloadmetaCollectorName + "-" + string(workloadmeta.KindKubernetesDeployment)

	clusterTagNamePrefix = "kube_cluster_name"
)

// CollectorPriorities holds collector priorities
var CollectorPriorities = make(map[string]types.CollectorPriority)

type processor interface {
	ProcessTagInfo([]*types.TagInfo)
}

// WorkloadMetaCollector collects tags from the metadata in the workloadmeta
// store.
type WorkloadMetaCollector struct {
	store        workloadmeta.Component
	children     map[types.EntityID]map[types.EntityID]struct{}
	tagProcessor processor

	containerEnvAsTags    map[string]string
	containerLabelsAsTags map[string]string

	staticTags                    map[string]string
	k8sResourcesAnnotationsAsTags map[string]map[string]string
	k8sResourcesLabelsAsTags      map[string]map[string]string
	globContainerLabels           map[string]glob.Glob
	globContainerEnvLabels        map[string]glob.Glob
	globK8sResourcesAnnotations   map[string]map[string]glob.Glob
	globK8sResourcesLabels        map[string]map[string]glob.Glob

	collectEC2ResourceTags            bool
	collectPersistentVolumeClaimsTags bool
}

func (c *WorkloadMetaCollector) initContainerMetaAsTags(labelsAsTags, envAsTags map[string]string) {
	c.containerLabelsAsTags, c.globContainerLabels = k8smetadata.InitMetadataAsTags(labelsAsTags)
	c.containerEnvAsTags, c.globContainerEnvLabels = k8smetadata.InitMetadataAsTags(envAsTags)
}

func (c *WorkloadMetaCollector) initK8sResourcesMetaAsTags(resourcesLabelsAsTags, resourcesAnnotationsAsTags map[string]map[string]string) {
	c.k8sResourcesAnnotationsAsTags = map[string]map[string]string{}
	c.k8sResourcesLabelsAsTags = map[string]map[string]string{}
	c.globK8sResourcesAnnotations = map[string]map[string]glob.Glob{}
	c.globK8sResourcesLabels = map[string]map[string]glob.Glob{}

	for resource, labelsAsTags := range resourcesLabelsAsTags {
		c.k8sResourcesLabelsAsTags[resource], c.globK8sResourcesLabels[resource] = k8smetadata.InitMetadataAsTags(labelsAsTags)
	}

	for resource, annotationsAsTags := range resourcesAnnotationsAsTags {
		c.k8sResourcesAnnotationsAsTags[resource], c.globK8sResourcesAnnotations[resource] = k8smetadata.InitMetadataAsTags(annotationsAsTags)
	}
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
		tags := taglist.NewTagList()

		for tag, value := range c.staticTags {
			tags.AddLow(tag, value)
		}

		low, orch, high, standard := tags.Compute()
		c.tagProcessor.ProcessTagInfo([]*types.TagInfo{
			{
				Source:               staticSource,
				EntityID:             common.GetGlobalEntityID(),
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
func NewWorkloadMetaCollector(_ context.Context, cfg config.Component, store workloadmeta.Component, p processor) *WorkloadMetaCollector {
	c := &WorkloadMetaCollector{
		tagProcessor:                      p,
		store:                             store,
		children:                          make(map[types.EntityID]map[types.EntityID]struct{}),
		collectEC2ResourceTags:            cfg.GetBool("ecs_collect_resource_tags_ec2"),
		collectPersistentVolumeClaimsTags: cfg.GetBool("kubernetes_persistent_volume_claims_as_tags"),
	}

	containerLabelsAsTags := mergeMaps(
		retrieveMappingFromConfig(cfg, "docker_labels_as_tags"),
		retrieveMappingFromConfig(cfg, "container_labels_as_tags"),
	)
	// Adding new environment variables require adding them to pkg/util/containers/env_vars_filter.go
	containerEnvAsTags := mergeMaps(
		retrieveMappingFromConfig(cfg, "docker_env_as_tags"),
		retrieveMappingFromConfig(cfg, "container_env_as_tags"),
	)
	c.initContainerMetaAsTags(containerLabelsAsTags, containerEnvAsTags)

	// kubernetes resources metadata as tags
	metadataAsTags := configutils.GetMetadataAsTags(cfg)
	c.initK8sResourcesMetaAsTags(metadataAsTags.GetResourcesLabelsAsTags(), metadataAsTags.GetResourcesAnnotationsAsTags())

	return c
}

// retrieveMappingFromConfig gets a stringmapstring config key and
// lowercases all map keys to make envvar and yaml sources consistent
func retrieveMappingFromConfig(cfg config.Component, configKey string) map[string]string {
	labelsList := cfg.GetStringMapString(configKey)
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
	CollectorPriorities[podSource] = types.NodeOrchestrator
	CollectorPriorities[taskSource] = types.NodeOrchestrator
	CollectorPriorities[containerSource] = types.NodeRuntime
	CollectorPriorities[containerImageSource] = types.NodeRuntime
}
