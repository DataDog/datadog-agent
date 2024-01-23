// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package collectors

import (
	"github.com/DataDog/datadog-agent/comp/core/workloadmeta"
	"github.com/DataDog/datadog-agent/pkg/tagger/utils"
)

const (
	// GlobalEntityID defines the entity ID that holds global tags
	GlobalEntityID = "internal://global-entity-id"

	podAnnotationPrefix              = "ad.datadoghq.com/"
	podContainerTagsAnnotationFormat = podAnnotationPrefix + "%s.tags"
	podTagsAnnotation                = podAnnotationPrefix + "tags"
	podStandardLabelPrefix           = "tags.datadoghq.com/"

	// Standard tag - Tag keys
	tagKeyEnv     = "env"
	tagKeyVersion = "version"
	tagKeyService = "service"

	// Standard K8s labels - Tag keys
	tagKeyKubeAppName      = "kube_app_name"
	tagKeyKubeAppInstance  = "kube_app_instance"
	tagKeyKubeAppVersion   = "kube_app_version"
	tagKeyKubeAppComponent = "kube_app_component"
	tagKeyKubeAppPartOf    = "kube_app_part_of"
	tagKeyKubeAppManagedBy = "kube_app_managed_by"

	// Standard tag - Environment variables
	envVarEnv     = "DD_ENV"
	envVarVersion = "DD_VERSION"
	envVarService = "DD_SERVICE"

	// Docker label keys
	dockerLabelEnv     = "com.datadoghq.tags.env"
	dockerLabelVersion = "com.datadoghq.tags.version"
	dockerLabelService = "com.datadoghq.tags.service"

	autodiscoveryLabelTagsKey = "com.datadoghq.ad.tags"
)

var (
	// When adding new environment variables, they need to be added to
	// pkg/util/containers/env_vars_filter.go
	standardEnvKeys = map[string]string{
		envVarEnv:     tagKeyEnv,
		envVarVersion: tagKeyVersion,
		envVarService: tagKeyService,
	}

	lowCardOrchestratorEnvKeys = map[string]string{
		"MARATHON_APP_ID": "marathon_app",

		"CHRONOS_JOB_NAME":  "chronos_job",
		"CHRONOS_JOB_OWNER": "chronos_job_owner",

		"NOMAD_TASK_NAME":  "nomad_task",
		"NOMAD_JOB_NAME":   "nomad_job",
		"NOMAD_GROUP_NAME": "nomad_group",
		"NOMAD_NAMESPACE":  "nomad_namespace",
		"NOMAD_DC":         "nomad_dc",
	}

	orchCardOrchestratorEnvKeys = map[string]string{
		"MESOS_TASK_ID": "mesos_task",
	}

	standardDockerLabels = map[string]string{
		dockerLabelEnv:     tagKeyEnv,
		dockerLabelVersion: tagKeyVersion,
		dockerLabelService: tagKeyService,
	}

	lowCardOrchestratorLabels = map[string]string{
		"com.docker.swarm.service.name": "swarm_service",
		"com.docker.stack.namespace":    "swarm_namespace",

		"io.rancher.stack.name":         "rancher_stack",
		"io.rancher.stack_service.name": "rancher_service",

		// Automatically extract git commit sha from image for source code integration
		"org.opencontainers.image.revision": "git.commit.sha",

		// Automatically extract repository url from image for source code integration
		"org.opencontainers.image.source": "git.repository_url",
	}

	highCardOrchestratorLabels = map[string]string{
		"io.rancher.container.name": "rancher_container",
	}
)

func (c *WorkloadMetaCollector) processEvents(evBundle workloadmeta.EventBundle) {
	panic("not called")
}

func (c *WorkloadMetaCollector) handleContainer(ev workloadmeta.Event) []*TagInfo {
	panic("not called")
}

func (c *WorkloadMetaCollector) handleContainerImage(ev workloadmeta.Event) []*TagInfo {
	panic("not called")
}

func (c *WorkloadMetaCollector) labelsToTags(labels map[string]string, tags *utils.TagList) {
	panic("not called")
}

func (c *WorkloadMetaCollector) handleKubePod(ev workloadmeta.Event) []*TagInfo {
	panic("not called")
}

func (c *WorkloadMetaCollector) handleKubeNode(ev workloadmeta.Event) []*TagInfo {
	panic("not called")
}

func (c *WorkloadMetaCollector) handleECSTask(ev workloadmeta.Event) []*TagInfo {
	panic("not called")
}

func (c *WorkloadMetaCollector) handleGardenContainer(container *workloadmeta.Container) []*TagInfo {
	panic("not called")
}

func (c *WorkloadMetaCollector) extractTagsFromPodLabels(pod *workloadmeta.KubernetesPod, tags *utils.TagList) {
	panic("not called")
}

func (c *WorkloadMetaCollector) extractTagsFromPodOwner(pod *workloadmeta.KubernetesPod, owner workloadmeta.KubernetesPodOwner, tags *utils.TagList) {
	panic("not called")
}

func (c *WorkloadMetaCollector) extractTagsFromPodContainer(pod *workloadmeta.KubernetesPod, podContainer workloadmeta.OrchestratorContainer, tags *utils.TagList) (*TagInfo, error) {
	panic("not called")
}

func (c *WorkloadMetaCollector) registerChild(parent, child workloadmeta.EntityID) {
	panic("not called")
}

func (c *WorkloadMetaCollector) handleDelete(ev workloadmeta.Event) []*TagInfo {
	panic("not called")
}

func (c *WorkloadMetaCollector) handleDeleteChildren(source string, children map[string]struct{}) []*TagInfo {
	panic("not called")
}

func (c *WorkloadMetaCollector) extractFromMapWithFn(input map[string]string, mapping map[string]string, fn func(string, string)) {
	panic("not called")
}

func (c *WorkloadMetaCollector) extractFromMapNormalizedWithFn(input map[string]string, mapping map[string]string, fn func(string, string)) {
	panic("not called")
}

func (c *WorkloadMetaCollector) extractTagsFromJSONInMap(key string, input map[string]string, tags *utils.TagList) {
	panic("not called")
}

func buildTaggerEntityID(entityID workloadmeta.EntityID) string {
	panic("not called")
}

func buildTaggerSource(entityID workloadmeta.EntityID) string {
	panic("not called")
}

func parseJSONValue(value string, tags *utils.TagList) error {
	panic("not called")
}

func parseContainerADTagsLabels(tags *utils.TagList, labelValue string) {
	panic("not called")
}

//lint:ignore U1000 Ignore unused function until the collector is implemented
func (c *WorkloadMetaCollector) handleProcess(ev workloadmeta.Event) []*TagInfo {
	panic("not called")
}
