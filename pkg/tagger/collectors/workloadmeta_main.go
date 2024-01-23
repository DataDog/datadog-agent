// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package collectors

import (
	"context"

	"github.com/gobwas/glob"

	"github.com/DataDog/datadog-agent/comp/core/workloadmeta"
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
	panic("not called")
}

func (c *WorkloadMetaCollector) initPodMetaAsTags(labelsAsTags, annotationsAsTags, nsLabelsAsTags map[string]string) {
	panic("not called")
}

// Run runs the continuous event watching loop and sends new tags to the
// tagger based on the events sent by the workloadmeta.
func (c *WorkloadMetaCollector) Run(ctx context.Context) {
	panic("not called")
}

func (c *WorkloadMetaCollector) collectStaticGlobalTags(ctx context.Context) {
	panic("not called")
}

func (c *WorkloadMetaCollector) stream(ctx context.Context) {
	panic("not called")
}

// NewWorkloadMetaCollector returns a new WorkloadMetaCollector.
func NewWorkloadMetaCollector(_ context.Context, store workloadmeta.Component, p processor) *WorkloadMetaCollector {
	panic("not called")
}

// retrieveMappingFromConfig gets a stringmapstring config key and
// lowercases all map keys to make envvar and yaml sources consistent
func retrieveMappingFromConfig(configKey string) map[string]string {
	panic("not called")
}

// mergeMaps merges two maps, in case of conflict the first argument is prioritized
func mergeMaps(first, second map[string]string) map[string]string {
	panic("not called")
}

func init() {
	CollectorPriorities[podSource] = NodeOrchestrator
	CollectorPriorities[taskSource] = NodeOrchestrator
	CollectorPriorities[containerSource] = NodeRuntime
	CollectorPriorities[containerImageSource] = NodeRuntime
}
