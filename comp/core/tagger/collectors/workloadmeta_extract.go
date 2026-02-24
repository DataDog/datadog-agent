// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package collectors

import (
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/DataDog/datadog-agent/comp/core/tagger/common"
	k8smetadata "github.com/DataDog/datadog-agent/comp/core/tagger/k8s_metadata"
	"github.com/DataDog/datadog-agent/comp/core/tagger/taglist"
	"github.com/DataDog/datadog-agent/comp/core/tagger/tags"
	"github.com/DataDog/datadog-agent/comp/core/tagger/types"
	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	"github.com/DataDog/datadog-agent/pkg/config/env"
	pkgconfigsetup "github.com/DataDog/datadog-agent/pkg/config/setup"
	"github.com/DataDog/datadog-agent/pkg/discovery/tracermetadata"
	"github.com/DataDog/datadog-agent/pkg/util/fargate"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/datadog-agent/pkg/util/tmplvar"
)

const (
	podAnnotationPrefix              = "ad.datadoghq.com/"
	podContainerTagsAnnotationFormat = podAnnotationPrefix + "%s.tags"
	podTagsAnnotation                = podAnnotationPrefix + "tags"
	podStandardLabelPrefix           = "tags.datadoghq.com/"

	// Standard tag - Environment variables
	envVarEnv     = "DD_ENV"
	envVarVersion = "DD_VERSION"
	envVarService = "DD_SERVICE"

	// OpenTelemetry SDK - Environment variables
	// https://opentelemetry.io/docs/languages/sdk-configuration/general
	// https://opentelemetry.io/docs/specs/semconv/resource/
	envVarOtelService            = "OTEL_SERVICE_NAME"
	envVarOtelResourceAttributes = "OTEL_RESOURCE_ATTRIBUTES"

	// Docker label keys
	dockerLabelEnv     = "com.datadoghq.tags.env"
	dockerLabelVersion = "com.datadoghq.tags.version"
	dockerLabelService = "com.datadoghq.tags.service"

	autodiscoveryLabelTagsKey = "com.datadoghq.ad.tags"

	// Datadog Autoscaling annotation
	// (from pkg/clusteragent/autoscaling/workload/model/const.go)
	// no importing due to packages it pulls
	datadogAutoscalingIDAnnotation = "autoscaling.datadoghq.com/autoscaler-id"
)

var (
	// When adding new environment variables, they need to be added to
	// pkg/util/containers/env_vars_filter.go
	standardEnvKeys = map[string]string{
		envVarEnv:     tags.Env,
		envVarVersion: tags.Version,
		envVarService: tags.Service,
	}

	otelStandardEnvKeys = map[string]string{
		envVarOtelService: tags.Service,
	}

	otelResourceAttributesMapping = map[string]string{
		"service.name":                tags.Service,
		"service.version":             tags.Version,
		"deployment.environment":      tags.Env,
		"deployment.environment.name": tags.Env,
	}

	lowCardOrchestratorEnvKeys = map[string]string{
		"DD_GIT_COMMIT_SHA":     tags.GitCommitSha,
		"DD_GIT_REPOSITORY_URL": tags.GitRepository,

		"MARATHON_APP_ID": tags.MarathonApp,

		"CHRONOS_JOB_NAME":  tags.ChronosJob,
		"CHRONOS_JOB_OWNER": tags.ChronosJobOwner,

		"NOMAD_TASK_NAME":  tags.NomadTask,
		"NOMAD_JOB_NAME":   tags.NomadJob,
		"NOMAD_GROUP_NAME": tags.NomadGroup,
		"NOMAD_NAMESPACE":  tags.NomadNamespace,
		"NOMAD_DC":         tags.NomadDC,
	}

	orchCardOrchestratorEnvKeys = map[string]string{
		"MESOS_TASK_ID": tags.MesosTask,
	}

	standardDockerLabels = map[string]string{
		dockerLabelEnv:     tags.Env,
		dockerLabelVersion: tags.Version,
		dockerLabelService: tags.Service,
	}

	lowCardOrchestratorLabels = map[string]string{
		"com.docker.swarm.service.name": tags.SwarmService,
		"com.docker.stack.namespace":    tags.SwarmNamespace,

		"io.rancher.stack.name":         tags.RancherStack,
		"io.rancher.stack_service.name": tags.RancherService,

		// Automatically extract git commit sha from image for source code integration
		"org.opencontainers.image.revision": tags.GitCommitSha,

		// Automatically extract repository url from image for source code integration
		"org.opencontainers.image.source": tags.GitRepository,
	}

	highCardOrchestratorLabels = map[string]string{
		"io.rancher.container.name": tags.RancherContainer,
	}

	logLimiter = log.NewLogLimit(20, 10*time.Minute)
)

func (c *WorkloadMetaCollector) processEvents(evBundle workloadmeta.EventBundle) {
	var tagInfos []*types.TagInfo

	for _, ev := range evBundle.Events {
		entity := ev.Entity
		entityID := entity.GetID()

		switch ev.Type {
		case workloadmeta.EventTypeSet:
			if entityID.Kind == workloadmeta.KindKubeletMetrics ||
				entityID.Kind == workloadmeta.KindKubelet {
				// No tags. Ignore
				continue
			}

			taggerEntityID := common.BuildTaggerEntityID(entityID)

			// keep track of children of this entity from previous
			// iterations ...
			unseen := make(map[types.EntityID]struct{})
			for childTaggerID := range c.children[taggerEntityID] {
				unseen[childTaggerID] = struct{}{}
			}

			// ... and create a new empty map to store the children
			// seen in this iteration.
			c.children[taggerEntityID] = make(map[types.EntityID]struct{})

			switch entityID.Kind {
			case workloadmeta.KindContainer:
				tagInfos = append(tagInfos, c.handleContainer(ev)...)
			case workloadmeta.KindKubernetesPod:
				tagInfos = append(tagInfos, c.handleKubePod(ev)...)
			case workloadmeta.KindECSTask:
				tagInfos = append(tagInfos, c.handleECSTask(ev)...)
			case workloadmeta.KindContainerImageMetadata:
				tagInfos = append(tagInfos, c.handleContainerImage(ev)...)
			case workloadmeta.KindKubernetesMetadata:
				tagInfos = append(tagInfos, c.handleKubeMetadata(ev)...)
			case workloadmeta.KindProcess:
				tagInfos = append(tagInfos, c.handleProcess(ev)...)
			case workloadmeta.KindKubernetesDeployment:
				tagInfos = append(tagInfos, c.handleKubeDeployment(ev)...)
			case workloadmeta.KindGPU:
				tagInfos = append(tagInfos, c.handleGPU(ev)...)
			case workloadmeta.KindCRD:
				tagInfos = append(tagInfos, c.handleCRD(ev)...)
			case workloadmeta.KindKubeCapabilities:
				tagInfos = append(tagInfos, c.handleKubeCapabilities(ev)...)
			default:
				log.Errorf("cannot handle event for entity %q with kind %q", entityID.ID, entityID.Kind)
			}

			// remove the children seen in this iteration from the
			// unseen list ...
			for childTaggerID := range c.children[taggerEntityID] {
				delete(unseen, childTaggerID)
			}

			// ... and remove entities for everything that has been
			// left
			source := buildTaggerSource(entityID)
			tagInfos = append(tagInfos, c.handleDeleteChildren(source, unseen)...)

		case workloadmeta.EventTypeUnset:
			tagInfos = append(tagInfos, c.handleDelete(ev)...)

		default:
			log.Errorf("cannot handle event of type %d", ev.Type)
		}

	}

	if len(tagInfos) > 0 {
		c.tagProcessor.ProcessTagInfo(tagInfos)
	}

	evBundle.Acknowledge()
}

func (c *WorkloadMetaCollector) handleContainer(ev workloadmeta.Event) []*types.TagInfo {
	container := ev.Entity.(*workloadmeta.Container)

	c.entityCompleteness[container.EntityID] = ev.IsComplete

	// Garden containers tagging is specific as we don't have any information locally
	// Metadata are not available and tags are retrieved as-is from Cluster Agent
	if container.Runtime == workloadmeta.ContainerRuntimeGarden {
		return c.handleGardenContainer(container, ev.IsComplete)
	}

	tagList := taglist.NewTagList()
	tagList.AddHigh(tags.ContainerName, container.Name)
	tagList.AddHigh(tags.ContainerID, container.ID)

	image := container.Image
	tagList.AddLow(tags.ImageName, image.Name)
	tagList.AddLow(tags.ShortImage, image.ShortName)
	tagList.AddLow(tags.ImageTag, image.Tag)
	tagList.AddLow(tags.ImageID, image.ID)

	if container.Runtime == workloadmeta.ContainerRuntimeDocker {
		if image.Tag != "" {
			tagList.AddLow(tags.DockerImage, image.Name+":"+image.Tag)
		} else {
			tagList.AddLow(tags.DockerImage, image.Name)
		}
	}

	c.labelsToTags(container.Labels, tagList)

	// standard tags from environment
	c.extractFromMapWithFn(container.EnvVars, standardEnvKeys, tagList.AddStandard)

	// standard tags in OpenTelemetry SDK format from environment
	c.addOpenTelemetryStandardTags(container, tagList)

	// orchestrator tags from environment
	c.extractFromMapWithFn(container.EnvVars, lowCardOrchestratorEnvKeys, tagList.AddLow)
	c.extractFromMapWithFn(container.EnvVars, orchCardOrchestratorEnvKeys, tagList.AddOrchestrator)

	// extract env as tags
	for envName, envValue := range container.EnvVars {
		k8smetadata.AddMetadataAsTags(envName, envValue, c.containerEnvAsTags, c.globContainerEnvLabels, tagList)
	}

	// static tags for ECS and EKS Fargate containers
	for tag, valueList := range c.staticTags {
		for _, value := range valueList {
			tagList.AddLow(tag, value)
		}
	}

	// gpu tags from container resource requests
	for _, gpuVendor := range container.Resources.GPUVendorList {
		tagList.AddLow(tags.KubeGPUVendor, gpuVendor)
	}

	// resize policy tags
	if container.ResizePolicy.CPURestartPolicy != "" {
		tagList.AddLow(tags.CPURestartPolicy, container.ResizePolicy.CPURestartPolicy)
	}

	if container.ResizePolicy.MemoryRestartPolicy != "" {
		tagList.AddLow(tags.MemoryRestartPolicy, container.ResizePolicy.MemoryRestartPolicy)
	}

	low, orch, high, standard := tagList.Compute()
	return []*types.TagInfo{
		{
			Source:               containerSource,
			EntityID:             common.BuildTaggerEntityID(container.EntityID),
			HighCardTags:         high,
			OrchestratorCardTags: orch,
			LowCardTags:          low,
			StandardTags:         standard,
			IsComplete:           c.containerCompleteness(container.ID, ev.IsComplete),
		},
	}
}

func (c *WorkloadMetaCollector) handleProcess(ev workloadmeta.Event) []*types.TagInfo {
	process := ev.Entity.(*workloadmeta.Process)

	tagList := taglist.NewTagList()

	// Add Unified Service Tagging tags if present in the service metadata
	if process.Service != nil {
		ustService := process.Service.UST.Service
		ustEnv := process.Service.UST.Env
		ustVersion := process.Service.UST.Version

		tagList.AddStandard(tags.Service, ustService)
		tagList.AddStandard(tags.Env, ustEnv)
		tagList.AddStandard(tags.Version, ustVersion)

		for _, tracerMeta := range process.Service.TracerMetadata {
			for key, value := range tracerMeta.Tags() {
				if tracermetadata.ShouldSkipServiceTagKV(key, value, ustService, ustEnv, ustVersion) {
					continue
				}

				// Add as low cardinality tag since these are application-level
				// metadata
				tagList.AddLow(key, value)
			}
		}
	}

	for _, gpuEntityID := range process.GPUs {
		gpu, err := c.store.GetGPU(gpuEntityID.ID)
		if err != nil {
			if logLimiter.ShouldLog() {
				log.Debugf("cannot get GPU entity for process %s: %s", process.EntityID.ID, err)
			}
			continue
		}

		c.extractGPUTags(gpu, tagList)
	}

	low, orch, high, standard := tagList.Compute()
	if len(low)+len(orch)+len(high)+len(standard) == 0 {
		return nil
	}

	return []*types.TagInfo{
		{
			Source:               processSource,
			EntityID:             common.BuildTaggerEntityID(process.EntityID),
			HighCardTags:         high,
			OrchestratorCardTags: orch,
			LowCardTags:          low,
			StandardTags:         standard,
			IsComplete:           ev.IsComplete,
		},
	}
}

func (c *WorkloadMetaCollector) handleContainerImage(ev workloadmeta.Event) []*types.TagInfo {
	image := ev.Entity.(*workloadmeta.ContainerImageMetadata)

	tagList := taglist.NewTagList()
	tagList.AddLow(tags.ImageName, image.Name)

	// In containerd some images are created without a repo digest, and it's
	// also possible to remove repo digests manually.
	// This means that the set of repos that we need to handle is the union of
	// the repos present in the repo digests and the ones present in the repo
	// tags.
	repos := make(map[string]struct{})
	for _, repoDigest := range image.RepoDigests {
		repos[strings.SplitN(repoDigest, "@sha256:", 2)[0]] = struct{}{}
	}
	for _, repoTag := range image.RepoTags {
		repos[strings.SplitN(repoTag, ":", 2)[0]] = struct{}{}
	}
	for repo := range repos {
		repoSplitted := strings.Split(repo, "/")
		shortName := repoSplitted[len(repoSplitted)-1]
		tagList.AddLow(tags.ShortImage, shortName)
	}

	for _, repoTag := range image.RepoTags {
		repoTagSplitted := strings.SplitN(repoTag, ":", 2)
		if len(repoTagSplitted) == 2 {
			tagList.AddLow(tags.ImageTag, repoTagSplitted[1])
		}
	}

	tagList.AddLow(tags.OSName, image.OS)
	tagList.AddLow(tags.OSVersion, image.OSVersion)
	tagList.AddLow(tags.Architecture, image.Architecture)

	c.labelsToTags(image.Labels, tagList)

	low, orch, high, standard := tagList.Compute()
	return []*types.TagInfo{
		{
			Source:               containerImageSource,
			EntityID:             common.BuildTaggerEntityID(image.EntityID),
			HighCardTags:         high,
			OrchestratorCardTags: orch,
			LowCardTags:          low,
			StandardTags:         standard,
			IsComplete:           ev.IsComplete,
		},
	}
}

func (c *WorkloadMetaCollector) labelsToTags(labels map[string]string, tags *taglist.TagList) {
	// standard tags from labels
	c.extractFromMapWithFn(labels, standardDockerLabels, tags.AddStandard)

	// container labels as tags
	for labelName, labelValue := range labels {
		k8smetadata.AddMetadataAsTags(labelName, labelValue, c.containerLabelsAsTags, c.globContainerLabels, tags)
	}

	// orchestrator tags from labels
	c.extractFromMapWithFn(labels, lowCardOrchestratorLabels, tags.AddLow)
	c.extractFromMapWithFn(labels, highCardOrchestratorLabels, tags.AddHigh)

	// extract labels as tags
	c.extractFromMapNormalizedWithFn(labels, c.containerLabelsAsTags, tags.AddAuto)

	// custom tags from label
	if lbl, ok := labels[autodiscoveryLabelTagsKey]; ok {
		parseContainerADTagsLabels(tags, lbl)
	}
}

func (c *WorkloadMetaCollector) extractTagsFromPodEntity(pod *workloadmeta.KubernetesPod, tagList *taglist.TagList, isComplete bool) *types.TagInfo {
	tagList.AddOrchestrator(tags.KubePod, pod.Name)
	tagList.AddLow(tags.KubeNamespace, pod.Namespace)
	tagList.AddLow(tags.PodPhase, strings.ToLower(pod.Phase))
	tagList.AddLow(tags.KubePriorityClass, pod.PriorityClass)
	tagList.AddLow(tags.KubeQOS, pod.QOSClass)
	tagList.AddLow(tags.KubeRuntimeClass, pod.RuntimeClass)

	c.extractTagsFromPodLabels(pod, tagList)

	// pod labels as tags
	for name, value := range pod.Labels {
		k8smetadata.AddMetadataAsTags(name, value, c.k8sResourcesLabelsAsTags["pods"], c.globK8sResourcesLabels["pods"], tagList)
	}

	// pod annotations as tags
	for name, value := range pod.Annotations {
		k8smetadata.AddMetadataAsTags(name, value, c.k8sResourcesAnnotationsAsTags["pods"], c.globK8sResourcesAnnotations["pods"], tagList)
	}

	// namespace labels as tags
	for name, value := range pod.NamespaceLabels {
		k8smetadata.AddMetadataAsTags(name, value, c.k8sResourcesLabelsAsTags["namespaces"], c.globK8sResourcesLabels["namespaces"], tagList)
	}

	// namespace annotations as tags
	for name, value := range pod.NamespaceAnnotations {
		k8smetadata.AddMetadataAsTags(name, value, c.k8sResourcesAnnotationsAsTags["namespaces"], c.globK8sResourcesAnnotations["namespaces"], tagList)
	}

	// gpu requested vendor as tags
	for _, gpuVendor := range pod.GPUVendorList {
		tagList.AddLow(tags.KubeGPUVendor, gpuVendor)
	}

	// autoscaler presence
	if pod.Annotations[datadogAutoscalingIDAnnotation] != "" {
		tagList.AddLow(tags.KubeAutoscalerKind, "datadogpodautoscaler")
	}

	kubeServiceDisabled := slices.Contains(pkgconfigsetup.Datadog().GetStringSlice("kubernetes_ad_tags_disabled"), "kube_service")
	if slices.Contains(strings.Split(pod.Annotations["tags.datadoghq.com/disable"], ","), "kube_service") {
		kubeServiceDisabled = true
	}
	if !kubeServiceDisabled {
		for _, svc := range pod.KubeServices {
			tagList.AddLow(tags.KubeService, svc)
		}
	}

	podAdapter := newResolvableAdapter(pod, nil)
	c.extractTagsFromJSONWithResolution(podTagsAnnotation, pod.Annotations, tagList, podAdapter)

	// OpenShift pod annotations
	if dcName, found := pod.Annotations["openshift.io/deployment-config.name"]; found {
		tagList.AddLow(tags.OpenshiftDeploymentConfig, dcName)
	}
	if deployName, found := pod.Annotations["openshift.io/deployment.name"]; found {
		tagList.AddOrchestrator(tags.OpenshiftDeployment, deployName)
	}

	// Admission + Remote Config correlation tags
	if rcID, found := pod.Annotations[kubernetes.RcIDAnnotKey]; found {
		tagList.AddLow(tags.RemoteConfigID, rcID)
	}
	if rcRev, found := pod.Annotations[kubernetes.RcRevisionAnnotKey]; found {
		tagList.AddLow(tags.RemoteConfigRevision, rcRev)
	}

	for _, owner := range pod.Owners {
		tagList.AddLow(tags.KubeOwnerRefKind, strings.ToLower(owner.Kind))
		tagList.AddOrchestrator(tags.KubeOwnerRefName, owner.Name)

		c.extractTagsFromPodOwner(pod, owner, tagList)
	}

	// static tags for EKS Fargate pods
	for tag, valueList := range c.staticTags {
		for _, value := range valueList {
			tagList.AddLow(tag, value)
		}
	}

	low, orch, high, standard := tagList.Compute()
	tagInfo := &types.TagInfo{
		Source:               podSource,
		EntityID:             common.BuildTaggerEntityID(pod.EntityID),
		HighCardTags:         high,
		OrchestratorCardTags: orch,
		LowCardTags:          low,
		StandardTags:         standard,
		IsComplete:           isComplete,
	}

	return tagInfo
}

func (c *WorkloadMetaCollector) handleKubePod(ev workloadmeta.Event) []*types.TagInfo {
	pod := ev.Entity.(*workloadmeta.KubernetesPod)

	c.entityCompleteness[pod.EntityID] = ev.IsComplete

	tagList := taglist.NewTagList()
	tagInfos := []*types.TagInfo{c.extractTagsFromPodEntity(pod, tagList, ev.IsComplete)}

	c.extractTagsFromPodLabels(pod, tagList)

	for _, podContainer := range pod.GetAllContainers() {
		cTagInfo, err := c.extractTagsFromPodContainer(pod, podContainer, tagList.Copy(), ev.IsComplete)
		if err != nil {
			log.Debugf("cannot extract tags from pod container: %s", err)
			continue
		}

		tagInfos = append(tagInfos, cTagInfo)
	}

	return tagInfos
}

func (c *WorkloadMetaCollector) handleECSTask(ev workloadmeta.Event) []*types.TagInfo {
	task := ev.Entity.(*workloadmeta.ECSTask)

	taskTags := taglist.NewTagList()

	// as of Agent 7.33, tasks have a name internally, but before that
	// task_name already was task.Family, so we keep it for backwards
	// compatibility
	taskTags.AddLow(tags.TaskName, task.Family)
	taskTags.AddLow(tags.TaskFamily, task.Family)
	taskTags.AddLow(tags.TaskVersion, task.Version)
	taskTags.AddLow(tags.AwsAccount, task.AWSAccountID)
	taskTags.AddLow(tags.Region, task.Region)
	taskTags.AddLow(tags.EcsServiceARN, task.ServiceARN)
	taskTags.AddOrchestrator(tags.TaskARN, task.ID)
	taskTags.AddOrchestrator(tags.TaskDefinitionARN, task.TaskDefinitionARN)

	clusterTags := taglist.NewTagList()
	if task.ClusterName != "" {
		// only add cluster_name to the task level tags, not global
		if !pkgconfigsetup.Datadog().GetBool("disable_cluster_name_tag_key") {
			taskTags.AddLow(tags.ClusterName, task.ClusterName)
		}
		clusterTags.AddLow(tags.EcsClusterName, task.ClusterName)
		clusterTags.AddLow(tags.EcsClusterARN, task.ClusterARN)
	}
	clusterLow, clusterOrch, clusterHigh, clusterStandard := clusterTags.Compute()

	if task.LaunchType == workloadmeta.ECSLaunchTypeFargate {
		taskTags.AddLow(tags.AvailabilityZoneDeprecated, task.AvailabilityZone) // Deprecated
		taskTags.AddLow(tags.AvailabilityZone, task.AvailabilityZone)
	} else if c.collectEC2ResourceTags {
		addResourceTags(taskTags, task.ContainerInstanceTags)
		addResourceTags(taskTags, task.Tags)
	}

	if task.ServiceName != "" {
		taskTags.AddLow(tags.EcsServiceName, strings.ToLower(task.ServiceName))
	}

	tagInfos := make([]*types.TagInfo, 0, len(task.Containers))

	for _, taskContainer := range task.Containers {
		container, err := c.store.GetContainer(taskContainer.ID)
		if err != nil {
			log.Debugf("task %q has reference to non-existing container %q", task.Name, taskContainer.ID)
			continue
		}

		c.registerChild(task.EntityID, container.EntityID)

		tagList := taskTags.Copy()

		tagList.AddLow(tags.EcsContainerName, taskContainer.Name)

		low, orch, high, standard := tagList.Compute()
		tagInfos = append(tagInfos, &types.TagInfo{
			// taskSource here is not a mistake. the source is
			// always from the parent resource.
			Source:               taskSource,
			EntityID:             common.BuildTaggerEntityID(container.EntityID),
			HighCardTags:         high,
			OrchestratorCardTags: orch,
			LowCardTags:          append(low, clusterLow...),
			StandardTags:         standard,
			IsComplete:           ev.IsComplete,
		})
	}

	// For Fargate and Managed Instances in sidecar mode, add task-level tags to global entity
	// These deployments don't report a hostname (task is the unit of identity)
	// IsSidecar() returns true for both ECS Fargate and managed instances in sidecar mode
	if task.LaunchType == workloadmeta.ECSLaunchTypeFargate ||
		(task.LaunchType == workloadmeta.ECSLaunchTypeManagedInstances && fargate.IsSidecar()) {
		low, orch, high, standard := taskTags.Compute()
		tagInfos = append(tagInfos, &types.TagInfo{
			Source:               taskSource,
			EntityID:             types.GetGlobalEntityID(),
			HighCardTags:         high,
			OrchestratorCardTags: orch,
			LowCardTags:          append(low, clusterLow...),
			StandardTags:         standard,
			IsComplete:           ev.IsComplete,
		})
	}

	// Global tags only updated when a valid ClusterName is provided
	// There exist edge cases in the metadata API returning a task without cluster info
	if task.ClusterName != "" {
		// Add global cluster tags for EC2 and Managed Instances in daemon mode
		// In daemon mode, the hostname is the EC2 instance, so we only add cluster tags (not task-specific tags)
		if task.LaunchType == workloadmeta.ECSLaunchTypeEC2 ||
			(task.LaunchType == workloadmeta.ECSLaunchTypeManagedInstances && !fargate.IsSidecar()) {
			tagInfos = append(tagInfos, &types.TagInfo{
				Source:               taskSource,
				EntityID:             types.GetGlobalEntityID(),
				HighCardTags:         clusterHigh,
				OrchestratorCardTags: clusterOrch,
				LowCardTags:          clusterLow,
				StandardTags:         clusterStandard,
				IsComplete:           ev.IsComplete,
			})
		}
	}
	return tagInfos
}

func (c *WorkloadMetaCollector) handleGardenContainer(container *workloadmeta.Container, isComplete bool) []*types.TagInfo {
	return []*types.TagInfo{
		{
			Source:       containerSource,
			EntityID:     common.BuildTaggerEntityID(container.EntityID),
			HighCardTags: container.CollectorTags,
			IsComplete:   isComplete,
		},
	}
}

func (c *WorkloadMetaCollector) handleKubeDeployment(ev workloadmeta.Event) []*types.TagInfo {
	deployment := ev.Entity.(*workloadmeta.KubernetesDeployment)

	groupResource := "deployments.apps"

	labelsAsTags := c.k8sResourcesLabelsAsTags[groupResource]
	annotationsAsTags := c.k8sResourcesAnnotationsAsTags[groupResource]

	if len(labelsAsTags)+len(annotationsAsTags) == 0 {
		return nil
	}

	globLabels := c.globK8sResourcesLabels[groupResource]
	globAnnotations := c.globK8sResourcesAnnotations[groupResource]

	tagList := taglist.NewTagList()

	for name, value := range deployment.Labels {
		k8smetadata.AddMetadataAsTags(name, value, labelsAsTags, globLabels, tagList)
	}

	for name, value := range deployment.Annotations {
		k8smetadata.AddMetadataAsTags(name, value, annotationsAsTags, globAnnotations, tagList)
	}

	low, orch, high, standard := tagList.Compute()

	if len(low)+len(orch)+len(high)+len(standard) == 0 {
		return nil
	}

	tagInfos := []*types.TagInfo{
		{
			Source:               deploymentSource,
			EntityID:             common.BuildTaggerEntityID(deployment.EntityID),
			HighCardTags:         high,
			OrchestratorCardTags: orch,
			LowCardTags:          low,
			StandardTags:         standard,
			IsComplete:           ev.IsComplete,
		},
	}

	return tagInfos
}

func (c *WorkloadMetaCollector) handleKubeMetadata(ev workloadmeta.Event) []*types.TagInfo {
	kubeMetadata := ev.Entity.(*workloadmeta.KubernetesMetadata)

	tagList := taglist.NewTagList()

	// Generic resource annotations and labels as tags
	groupResource := kubeMetadata.GVR.GroupResource().String()

	labelsAsTags := c.k8sResourcesLabelsAsTags[groupResource]
	annotationsAsTags := c.k8sResourcesAnnotationsAsTags[groupResource]

	globLabels := c.globK8sResourcesLabels[groupResource]
	globAnnotations := c.globK8sResourcesAnnotations[groupResource]

	for name, value := range kubeMetadata.Labels {
		k8smetadata.AddMetadataAsTags(name, value, labelsAsTags, globLabels, tagList)
	}

	for name, value := range kubeMetadata.Annotations {
		k8smetadata.AddMetadataAsTags(name, value, annotationsAsTags, globAnnotations, tagList)
	}

	low, orch, high, standard := tagList.Compute()

	if len(low)+len(orch)+len(high)+len(standard) == 0 {
		return nil
	}

	tagInfos := []*types.TagInfo{
		{
			Source:               kubeMetadataSource,
			EntityID:             common.BuildTaggerEntityID(kubeMetadata.EntityID),
			HighCardTags:         high,
			OrchestratorCardTags: orch,
			LowCardTags:          low,
			StandardTags:         standard,
			IsComplete:           ev.IsComplete,
		},
	}

	return tagInfos
}

func (c *WorkloadMetaCollector) handleGPU(ev workloadmeta.Event) []*types.TagInfo {
	gpu := ev.Entity.(*workloadmeta.GPU)

	tagList := taglist.NewTagList()
	c.extractGPUTags(gpu, tagList)

	low, orch, high, standard := tagList.Compute()

	if len(low)+len(orch)+len(high)+len(standard) == 0 {
		return nil
	}

	tagInfos := []*types.TagInfo{
		{
			Source:               gpuSource,
			EntityID:             common.BuildTaggerEntityID(gpu.EntityID),
			HighCardTags:         high,
			OrchestratorCardTags: orch,
			LowCardTags:          low,
			StandardTags:         standard,
			IsComplete:           ev.IsComplete,
		},
	}

	return tagInfos
}

// extractGPUTags extracts GPU tags from a GPU entity and adds them to the provided tagList
func (c *WorkloadMetaCollector) extractGPUTags(gpu *workloadmeta.GPU, tagList *taglist.TagList) {
	gpuUUID := strings.ToLower(gpu.ID)
	tagList.AddLow(tags.KubeGPUVendor, strings.ToLower(gpu.Vendor))
	tagList.AddLow(tags.KubeGPUDevice, strings.ToLower(strings.ReplaceAll(gpu.Device, " ", "_")))
	tagList.AddLow(tags.KubeGPUUUID, gpuUUID)
	tagList.AddLow(tags.GPUDriverVersion, gpu.DriverVersion)
	tagList.AddLow(tags.GPUVirtualizationMode, gpu.VirtualizationMode)
	tagList.AddLow(tags.GPUArchitecture, strings.ToLower(gpu.Architecture))
	tagList.AddLow(tags.GPUSlicingMode, gpu.SlicingMode())
	if gpu.GPUType != "" {
		tagList.AddLow(tags.GPUType, strings.ToLower(gpu.GPUType))
	}

	if gpu.ParentGPUUUID == "" {
		tagList.AddLow(tags.GPUParentGPUUUID, gpuUUID)
	} else {
		tagList.AddLow(tags.GPUParentGPUUUID, strings.ToLower(gpu.ParentGPUUUID))
	}
}

func (c *WorkloadMetaCollector) handleCRD(ev workloadmeta.Event) []*types.TagInfo {
	crd := ev.Entity.(*workloadmeta.CRD)

	tagList := taglist.NewTagList()

	tagList.AddLow("crd_group", crd.Group)
	tagList.AddLow("crd_kind", crd.Kind)
	tagList.AddLow("crd_version", crd.Version)
	tagList.AddLow("crd_name", crd.Name)
	tagList.AddLow("crd_namespace", crd.Namespace)

	low, orch, high, standard := tagList.Compute()

	tagInfos := []*types.TagInfo{
		{
			Source:               crdSource,
			EntityID:             common.BuildTaggerEntityID(crd.EntityID),
			HighCardTags:         high,
			OrchestratorCardTags: orch,
			LowCardTags:          low,
			StandardTags:         standard,
			IsComplete:           ev.IsComplete,
		},
	}

	return tagInfos
}

func (c *WorkloadMetaCollector) handleKubeCapabilities(ev workloadmeta.Event) []*types.TagInfo {
	kubeCapabilities := ev.Entity.(*workloadmeta.KubeCapabilities)

	tagList := taglist.NewTagList()
	tagList.AddLow(tags.KubeServerVersion, kubeCapabilities.Version.String())

	low, orch, high, standard := tagList.Compute()

	tagInfos := []*types.TagInfo{
		{
			Source:               kubeCapabilitiesSource,
			EntityID:             common.BuildTaggerEntityID(kubeCapabilities.EntityID),
			HighCardTags:         high,
			OrchestratorCardTags: orch,
			LowCardTags:          low,
			StandardTags:         standard,
			IsComplete:           ev.IsComplete,
		},
	}

	return tagInfos
}

func (c *WorkloadMetaCollector) extractTagsFromPodLabels(pod *workloadmeta.KubernetesPod, tagList *taglist.TagList) {
	for name, value := range pod.Labels {
		switch name {
		case kubernetes.EnvTagLabelKey:
			tagList.AddStandard(tags.Env, value)
		case kubernetes.VersionTagLabelKey:
			tagList.AddStandard(tags.Version, value)
		case kubernetes.ServiceTagLabelKey:
			tagList.AddStandard(tags.Service, value)
		case kubernetes.KubeAppNameLabelKey:
			tagList.AddLow(tags.KubeAppName, value)
		case kubernetes.KubeAppInstanceLabelKey:
			tagList.AddLow(tags.KubeAppInstance, value)
		case kubernetes.KubeAppVersionLabelKey:
			tagList.AddLow(tags.KubeAppVersion, value)
		case kubernetes.KubeAppComponentLabelKey:
			tagList.AddLow(tags.KubeAppComponent, value)
		case kubernetes.KubeAppPartOfLabelKey:
			tagList.AddLow(tags.KubeAppPartOf, value)
		case kubernetes.KubeAppManagedByLabelKey:
			tagList.AddLow(tags.KubeAppManagedBy, value)
		}

		k8smetadata.AddMetadataAsTags(name, value, c.k8sResourcesLabelsAsTags["pods"], c.globK8sResourcesLabels["pods"], tagList)
	}
}

func (c *WorkloadMetaCollector) extractTagsFromPodOwner(pod *workloadmeta.KubernetesPod, owner workloadmeta.KubernetesPodOwner, tagList *taglist.TagList) {
	switch owner.Kind {
	case kubernetes.DeploymentKind:
		tagList.AddLow(tags.KubeDeployment, owner.Name)

	case kubernetes.DaemonSetKind:
		tagList.AddLow(tags.KubeDaemonSet, owner.Name)

	case kubernetes.ReplicationControllerKind:
		tagList.AddLow(tags.KubeReplicationController, owner.Name)

	case kubernetes.StatefulSetKind:
		tagList.AddLow(tags.KubeStatefulSet, owner.Name)
		if c.collectPersistentVolumeClaimsTags {
			for _, pvc := range pod.PersistentVolumeClaimNames {
				if pvc != "" {
					tagList.AddLow(tags.KubePersistentVolumeClaim, pvc)
				}
			}
		}

	case kubernetes.JobKind:
		cronjob, _ := kubernetes.ParseCronJobForJob(owner.Name)
		if cronjob != "" {
			tagList.AddOrchestrator(tags.KubeJob, owner.Name)
			tagList.AddLow(tags.KubeCronjob, cronjob)
		} else {
			tagList.AddLow(tags.KubeJob, owner.Name)
		}

	case kubernetes.ReplicaSetKind:
		deployment := kubernetes.ParseDeploymentForReplicaSet(owner.Name)
		if len(deployment) > 0 {
			tagList.AddLow(tags.KubeDeployment, deployment)
			// Add Argo Rollout tag key if the deployment is controlled by Argo Rollout
			if pod.Labels[kubernetes.ArgoRolloutLabelKey] != "" {
				tagList.AddLow(tags.KubeArgoRollout, deployment)
			}
		}
		tagList.AddLow(tags.KubeReplicaSet, owner.Name)
	}
}

func (c *WorkloadMetaCollector) extractTagsFromPodContainer(pod *workloadmeta.KubernetesPod, podContainer workloadmeta.OrchestratorContainer, tagList *taglist.TagList, podComplete bool) (*types.TagInfo, error) {
	container, err := c.store.GetContainer(podContainer.ID)
	if err != nil {
		return nil, fmt.Errorf("pod %q has reference to non-existing container %q", pod.Name, podContainer.ID)
	}

	c.registerChild(pod.EntityID, container.EntityID)

	tagList.AddLow(tags.KubeContainerName, podContainer.Name)
	tagList.AddHigh(tags.ContainerID, container.ID)

	if container.Name != "" && pod.Name != "" {
		tagList.AddHigh(tags.DisplayContainerName, container.Name+"_"+pod.Name)
	} else if podContainer.Name != "" && pod.Name != "" {
		tagList.AddHigh(tags.DisplayContainerName, podContainer.Name+"_"+pod.Name)
	}

	image := podContainer.Image
	tagList.AddLow(tags.ImageName, image.Name)
	tagList.AddLow(tags.ShortImage, image.ShortName)
	tagList.AddLow(tags.ImageTag, image.Tag)
	tagList.AddLow(tags.ImageID, image.ID)

	// enrich with standard tags from labels for this container if present
	containerName := podContainer.Name
	standardTagKeys := map[string]string{
		fmt.Sprintf(podStandardLabelPrefix+"%s.%s", containerName, tags.Env):     tags.Env,
		fmt.Sprintf(podStandardLabelPrefix+"%s.%s", containerName, tags.Version): tags.Version,
		fmt.Sprintf(podStandardLabelPrefix+"%s.%s", containerName, tags.Service): tags.Service,
	}
	c.extractFromMapWithFn(pod.Labels, standardTagKeys, tagList.AddStandard)

	// enrich with standard tags from environment variables
	c.extractFromMapWithFn(container.EnvVars, standardEnvKeys, tagList.AddStandard)

	// standard tags in OpenTelemetry SDK format from environment
	c.addOpenTelemetryStandardTags(container, tagList)

	// container-specific tags provided through pod annotation
	annotation := fmt.Sprintf(podContainerTagsAnnotationFormat, containerName)
	containerAdapter := newResolvableAdapter(pod, container)
	c.extractTagsFromJSONWithResolution(annotation, pod.Annotations, tagList, containerAdapter)

	containerComplete := c.entityCompleteness[container.EntityID]

	low, orch, high, standard := tagList.Compute()
	return &types.TagInfo{
		// podSource here is not a mistake. the source is
		// always from the parent resource.
		Source:               podSource,
		EntityID:             common.BuildTaggerEntityID(container.EntityID),
		HighCardTags:         high,
		OrchestratorCardTags: orch,
		LowCardTags:          low,
		StandardTags:         standard,
		IsComplete:           containerComplete && podComplete,
	}, nil
}

func (c *WorkloadMetaCollector) registerChild(parent, child workloadmeta.EntityID) {
	parentTaggerEntityID := common.BuildTaggerEntityID(parent)
	childTaggerEntityID := common.BuildTaggerEntityID(child)

	m, ok := c.children[parentTaggerEntityID]
	if !ok {
		c.children[parentTaggerEntityID] = make(map[types.EntityID]struct{})
		m = c.children[parentTaggerEntityID]
	}

	m[childTaggerEntityID] = struct{}{}
}

func (c *WorkloadMetaCollector) handleDelete(ev workloadmeta.Event) []*types.TagInfo {
	entityID := ev.Entity.GetID()
	taggerEntityID := common.BuildTaggerEntityID(entityID)

	children := c.children[taggerEntityID]

	source := buildTaggerSource(entityID)
	tagInfos := make([]*types.TagInfo, 0, len(children)+1)
	tagInfos = append(tagInfos, &types.TagInfo{
		Source:       source,
		EntityID:     taggerEntityID,
		DeleteEntity: true,
	})
	tagInfos = append(tagInfos, c.handleDeleteChildren(source, children)...)

	delete(c.children, taggerEntityID)
	delete(c.entityCompleteness, entityID)

	return tagInfos
}

// containerCompleteness computes the effective completeness for a container. In
// Kubernetes, a container's tags also depend on its pod's data, so completeness
// requires both the container and its pod to be complete.
func (c *WorkloadMetaCollector) containerCompleteness(containerID string, containerComplete bool) bool {
	if !env.IsFeaturePresent(env.Kubernetes) {
		return containerComplete
	}

	if !containerComplete {
		return false
	}

	pod, err := c.store.GetKubernetesPodForContainer(containerID)
	if err != nil {
		return false
	}

	podComplete, ok := c.entityCompleteness[pod.EntityID]
	if !ok {
		return false
	}

	return podComplete
}

func (c *WorkloadMetaCollector) handleDeleteChildren(source string, children map[types.EntityID]struct{}) []*types.TagInfo {
	tagInfos := make([]*types.TagInfo, 0, len(children))

	for childEntityID := range children {
		t := types.TagInfo{
			Source:       source,
			EntityID:     childEntityID,
			DeleteEntity: true,
		}
		tagInfos = append(tagInfos, &t)
	}

	return tagInfos
}

func (c *WorkloadMetaCollector) extractFromMapWithFn(input map[string]string, mapping map[string]string, fn func(string, string)) {
	for key, tag := range mapping {
		if value, ok := input[key]; ok {
			fn(tag, value)
		}
	}
}

func (c *WorkloadMetaCollector) extractFromMapNormalizedWithFn(input map[string]string, mapping map[string]string, fn func(string, string)) {
	for key, value := range input {
		if tag, ok := mapping[strings.ToLower(key)]; ok {
			fn(tag, value)
		}
	}
}

func (c *WorkloadMetaCollector) extractTagsFromJSONWithResolution(key string, input map[string]string, tags *taglist.TagList, resolvable tmplvar.Resolvable) {
	jsonTags, found := input[key]
	if !found {
		return
	}

	err := parseJSONValueWithResolution(jsonTags, tags, resolvable)
	if err != nil {
		log.Errorf("can't parse value for annotation %s: %s", key, err)
	}
}

func (c *WorkloadMetaCollector) addOpenTelemetryStandardTags(container *workloadmeta.Container, tags *taglist.TagList) {
	if otelResourceAttributes, ok := container.EnvVars[envVarOtelResourceAttributes]; ok {
		for pair := range strings.SplitSeq(otelResourceAttributes, ",") {
			fields := strings.SplitN(pair, "=", 2)
			if len(fields) != 2 {
				log.Debugf("invalid OpenTelemetry resource attribute: %s", pair)
				continue
			}
			fields[0], fields[1] = strings.TrimSpace(fields[0]), strings.TrimSpace(fields[1])
			if tag, ok := otelResourceAttributesMapping[fields[0]]; ok {
				tags.AddStandard(tag, fields[1])
			}
		}
	}
	c.extractFromMapWithFn(container.EnvVars, otelStandardEnvKeys, tags.AddStandard)
}

func buildTaggerSource(entityID workloadmeta.EntityID) string {
	return workloadmetaCollectorName + "-" + string(entityID.Kind)
}

func parseJSONValue(value string, tags *taglist.TagList) error {
	result := map[string]interface{}{}
	if err := json.Unmarshal([]byte(value), &result); err != nil {
		return fmt.Errorf("failed to unmarshal JSON: %s", err)
	}

	for key, value := range result {
		switch v := value.(type) {
		case string:
			tags.AddAuto(key, v)
		case float64:
			tags.AddAuto(key, fmt.Sprint(v))
		case int64:
			tags.AddAuto(key, strconv.FormatInt(v, 10))
		case bool:
			tags.AddAuto(key, strconv.FormatBool(v))
		case []interface{}:
			for _, tag := range v {
				tags.AddAuto(key, fmt.Sprint(tag))
			}
		default:
			log.Debugf("Tag value %s is not valid, must be a string, int, float, bool or an array, skipping", v)
		}
	}
	return nil
}

func parseJSONValueWithResolution(value string, tags *taglist.TagList, resolvable tmplvar.Resolvable) error {
	if value == "" {
		return errors.New("value is empty")
	}

	// Parse without template resolution if no resolvable entity is provided.
	if resolvable == nil {
		log.Debug("no resolvable entity provided, parsing without template resolution")
		return parseJSONValue(value, tags)
	}

	resolver := tmplvar.NewTemplateResolver(tmplvar.JSONParser, nil, false)
	resolved, err := resolver.ResolveDataWithTemplateVars([]byte(value), resolvable)
	if err != nil {
		// If resolution fails, log but try to parse the original value
		log.Debugf("Failed to resolve template variables in tags: %v", err)
		return parseJSONValue(value, tags)
	}

	return parseJSONValue(string(resolved), tags)
}

func parseContainerADTagsLabels(tags *taglist.TagList, labelValue string) {
	tagNames := []string{}
	err := json.Unmarshal([]byte(labelValue), &tagNames)
	if err != nil {
		log.Debugf("Cannot unmarshal AD tags: %s", err)
	}
	for _, tag := range tagNames {
		tagParts := strings.Split(tag, ":")
		// skip if tag is not in expected k:v format
		if len(tagParts) != 2 {
			log.Debugf("Tag '%s' is not in k:v format", tag)
			continue
		}
		tags.AddHigh(tagParts[0], tagParts[1])
	}
}
