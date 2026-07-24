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
	tracermetadata "github.com/DataDog/datadog-agent/pkg/discovery/tracermetadata/model"
	pkgimage "github.com/DataDog/datadog-agent/pkg/util/containers/image"
	"github.com/DataDog/datadog-agent/pkg/util/fargate"
	gpuutil "github.com/DataDog/datadog-agent/pkg/util/gpu"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/datadog-agent/pkg/util/tmplvar"
)

const (
	podStandardLabelPrefix = "tags.datadoghq.com/"

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
			case workloadmeta.KindKubernetesKueueQueue:
				tagInfos = append(tagInfos, c.handleKubeKueueQueue(ev)...)
			case workloadmeta.KindKubernetesKueueResourceFlavor:
				tagInfos = append(tagInfos, c.handleKubeKueueResourceFlavor(ev)...)
			case workloadmeta.KindKubernetesKueueWorkload:
				tagInfos = append(tagInfos, c.handleKubeKueueWorkload(ev)...)
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

		ExtractGPUTags(gpu, tagList)
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
		// Split on the last colon (after the last slash) so registries that
		// include a port are parsed correctly.
		repoName, _ := pkgimage.SplitRepoTag(repoTag)
		repos[repoName] = struct{}{}
	}
	for repo := range repos {
		repoSplitted := strings.Split(repo, "/")
		shortName := repoSplitted[len(repoSplitted)-1]
		tagList.AddLow(tags.ShortImage, shortName)
	}

	for _, repoTag := range image.RepoTags {
		if _, tag := pkgimage.SplitRepoTag(repoTag); tag != "" {
			tagList.AddLow(tags.ImageTag, tag)
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

	kubeServiceDisabled := slices.Contains(c.cfg.GetStringSlice("kubernetes_ad_tags_disabled"), "kube_service")
	if slices.Contains(strings.Split(pod.Annotations["tags.datadoghq.com/disable"], ","), "kube_service") {
		kubeServiceDisabled = true
	}
	if !kubeServiceDisabled {
		for _, svc := range pod.KubeServices {
			tagList.AddLow(tags.KubeService, svc)
		}
	}

	podAdapter := newResolvableAdapter(pod, nil)
	c.extractTagsFromJSONWithResolution(kubernetes.ADTagsAnnotation, pod.Annotations, tagList, podAdapter)

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
	c.extractTagsFromPodLabels(pod, tagList)
	c.extractTagsFromPodKueueInfo(pod, tagList)

	tagInfos := []*types.TagInfo{c.extractTagsFromPodEntity(pod, tagList, ev.IsComplete)}

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

	// ECS tasks are reported by a single collector (ECS). So they are always
	// marked as complete by workloadmeta. However, the ECS collector can report
	// incomplete data. It seems that this happens when the v4 metadata endpoint
	// is not yet available when a task is created. This is infrequent, but when
	// it happens, the ECS collector doesn't set a cluster name, so tags are
	// incomplete.
	ecsTaskIsComplete := ev.IsComplete && task.ClusterName != ""
	c.entityCompleteness[task.EntityID] = ecsTaskIsComplete

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
	taskTags.AddLow(tags.EcsDaemonARN, task.DaemonARN)
	taskTags.AddOrchestrator(tags.TaskARN, task.ID)
	if task.DaemonName != "" {
		taskTags.AddOrchestrator(tags.DaemonTaskDefinitionARN, task.TaskDefinitionARN)
	} else {
		taskTags.AddOrchestrator(tags.TaskDefinitionARN, task.TaskDefinitionARN)
	}

	clusterTags := taglist.NewTagList()
	if task.ClusterName != "" {
		// only add cluster_name to the task level tags, not global
		if !c.cfg.GetBool("disable_cluster_name_tag_key") {
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
		addResourceTags(c.cfg, taskTags, task.ContainerInstanceTags)
		addResourceTags(c.cfg, taskTags, task.Tags)
	}

	if task.ServiceName != "" {
		taskTags.AddLow(tags.EcsServiceName, strings.ToLower(task.ServiceName))
	}

	if task.DaemonName != "" {
		taskTags.AddLow(tags.EcsDaemonName, strings.ToLower(task.DaemonName))
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

		containerComplete := c.entityCompleteness[container.EntityID]

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
			IsComplete:           ecsTaskIsComplete && containerComplete,
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
			IsComplete:           ecsTaskIsComplete,
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
				IsComplete:           ecsTaskIsComplete,
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

func (c *WorkloadMetaCollector) handleKubeKueueQueue(ev workloadmeta.Event) []*types.TagInfo {
	queue := ev.Entity.(*workloadmeta.KubernetesKueueQueue)

	tagList := taglist.NewTagList()
	c.extractKueueQueueTags(queue, tagList)
	low, orch, high, standard := tagList.Compute()

	if len(low)+len(orch)+len(high)+len(standard) == 0 {
		return nil
	}

	return []*types.TagInfo{
		{
			Source:               kueueQueueSource,
			EntityID:             common.BuildTaggerEntityID(queue.EntityID),
			HighCardTags:         high,
			OrchestratorCardTags: orch,
			LowCardTags:          low,
			StandardTags:         standard,
			IsComplete:           ev.IsComplete,
		},
	}
}

func (c *WorkloadMetaCollector) handleKubeKueueResourceFlavor(ev workloadmeta.Event) []*types.TagInfo {
	flavor := ev.Entity.(*workloadmeta.KubernetesKueueResourceFlavor)

	tagList := taglist.NewTagList()
	c.extractKueueResourceFlavorTags(flavor, tagList)
	low, orch, high, standard := tagList.Compute()

	if len(low)+len(orch)+len(high)+len(standard) == 0 {
		return nil
	}

	return []*types.TagInfo{
		{
			Source:               kueueResourceFlavorSource,
			EntityID:             common.BuildTaggerEntityID(flavor.EntityID),
			HighCardTags:         high,
			OrchestratorCardTags: orch,
			LowCardTags:          low,
			StandardTags:         standard,
			IsComplete:           ev.IsComplete,
		},
	}
}

func (c *WorkloadMetaCollector) handleKubeKueueWorkload(ev workloadmeta.Event) []*types.TagInfo {
	workload := ev.Entity.(*workloadmeta.KubernetesKueueWorkload)

	tagList := taglist.NewTagList()
	c.extractKueueWorkloadAndRelatedTags(workload, "", tagList)
	low, orch, high, standard := tagList.Compute()

	if len(low)+len(orch)+len(high)+len(standard) == 0 {
		return nil
	}

	return []*types.TagInfo{
		{
			Source:               kueueWorkloadSource,
			EntityID:             common.BuildTaggerEntityID(workload.EntityID),
			HighCardTags:         high,
			OrchestratorCardTags: orch,
			LowCardTags:          low,
			StandardTags:         standard,
			IsComplete:           ev.IsComplete,
		},
	}
}

func (c *WorkloadMetaCollector) handleGPU(ev workloadmeta.Event) []*types.TagInfo {
	gpu := ev.Entity.(*workloadmeta.GPU)

	tagList := taglist.NewTagList()
	ExtractGPUTags(gpu, tagList)

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

// ExtractGPUTags extracts GPU tags from a GPU entity and adds them to the provided tagList
func ExtractGPUTags(gpu *workloadmeta.GPU, tagList *taglist.TagList) {
	gpuUUID := strings.ToLower(gpu.ID)
	tagList.AddLow(tags.KubeGPUVendor, strings.ToLower(gpu.Vendor))
	tagList.AddLow(tags.KubeGPUDevice, gpuutil.NormalizeGPUDeviceName(gpu.Device))
	tagList.AddLow(tags.KubeGPUUUID, gpuUUID)
	tagList.AddLow(tags.GPUDriverVersion, gpu.DriverVersion)
	tagList.AddLow(tags.GPUVirtualizationMode, gpu.VirtualizationMode)
	tagList.AddLow(tags.GPUArchitecture, strings.ToLower(gpu.Architecture))
	tagList.AddLow(tags.GPUSlicingMode, gpu.SlicingMode())
	tagList.AddLow(tags.GPUPCIBusID, strings.ToLower(gpu.PCIBusID))
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

// addResourceLabelsAndAnnotationsAsTags applies the labels-as-tags and
// annotations-as-tags configuration for the given group resource to the
// provided labels and annotations.
func (c *WorkloadMetaCollector) addResourceLabelsAndAnnotationsAsTags(groupResource string, labels, annotations map[string]string, tagList *taglist.TagList) {
	labelsAsTags := c.k8sResourcesLabelsAsTags[groupResource]
	annotationsAsTags := c.k8sResourcesAnnotationsAsTags[groupResource]
	globLabels := c.globK8sResourcesLabels[groupResource]
	globAnnotations := c.globK8sResourcesAnnotations[groupResource]

	for name, value := range labels {
		k8smetadata.AddMetadataAsTags(name, value, labelsAsTags, globLabels, tagList)
	}

	for name, value := range annotations {
		k8smetadata.AddMetadataAsTags(name, value, annotationsAsTags, globAnnotations, tagList)
	}
}

func (c *WorkloadMetaCollector) extractKueueQueueTags(queue *workloadmeta.KubernetesKueueQueue, tagList *taglist.TagList) {
	switch queue.QueueType {
	case workloadmeta.KueueLocalQueue:
		tagList.AddLow(tags.KueueLocalQueue, queue.Name)
		tagList.AddLow(tags.KueueClusterQueue, queue.ClusterQueueName)
		tagList.AddLow(tags.KubeNamespace, queue.Namespace)
	case workloadmeta.KueueClusterQueue:
		tagList.AddLow(tags.KueueClusterQueue, queue.Name)
	}

	groupResource := kueueQueueGroupResource(queue.QueueType)
	c.addResourceLabelsAndAnnotationsAsTags(groupResource, queue.Labels, queue.Annotations, tagList)
}

func (c *WorkloadMetaCollector) extractKueueResourceFlavorTags(flavor *workloadmeta.KubernetesKueueResourceFlavor, tagList *taglist.TagList) {
	tagList.AddLow(tags.KueueResourceFlavor, flavor.Name)
	for name, value := range flavor.NodeAffinityLabels {
		if strings.HasPrefix(name, "nvidia.com/") {
			tagList.AddLow(tags.KubeGPUVendor, "nvidia")
		}

		switch name {
		case "nvidia.com/gpu.product":
			gpuDevice := gpuutil.GFDLabelToGPUDeviceName(value)
			tagList.AddLow(tags.KubeGPUDevice, gpuutil.NormalizeGPUDeviceName(gpuDevice))
			if gpuType := gpuutil.ExtractGPUType(gpuDevice); gpuType != "" {
				tagList.AddLow(tags.GPUType, gpuType)
			}
		case "nvidia.com/gpu.family":
			tagList.AddLow(tags.GPUArchitecture, strings.ToLower(value))
		case "nvidia.com/cuda.driver-version.full":
			tagList.AddLow(tags.GPUDriverVersion, value)
		default:
			if tagName, ok := nvidiaResourceFlavorNodeLabelTagName(name); ok {
				tagList.AddLow(tagName, value)
			}
		}
	}

	groupResource := kubernetes.KueueResourceFlavorResourceName + "." + kubernetes.KueueGroupName
	c.addResourceLabelsAndAnnotationsAsTags(groupResource, flavor.Labels, flavor.Annotations, tagList)
}

func (c *WorkloadMetaCollector) extractKueueWorkloadAndRelatedTags(workload *workloadmeta.KubernetesKueueWorkload, podSetName string, tagList *taglist.TagList) {
	// Add first the workload identification tags
	tagList.AddOrchestrator(tags.KueueWorkload, workload.Name)
	if workload.UID != "" {
		tagList.AddOrchestrator(tags.KueueWorkloadUID, workload.UID)
	}

	// and the ones from the labels/annotations
	groupResource := kubernetes.KueueWorkloadResourceName + "." + kubernetes.KueueGroupName
	c.addResourceLabelsAndAnnotationsAsTags(groupResource, workload.Labels, workload.Annotations, tagList)

	// now get the queue entities for more tags
	localQueue := c.extractKueueQueueTagsFromQueueName(workload.Namespace, tagList, workloadmeta.KueueLocalQueue, workload.QueueName)

	// The cluster queue name might not be present in the workload object. Local queues are always
	// associated to a cluster queue, so if the local queue entity is available, use its cluster
	// queue name as a fallback.
	clusterQueueName := workload.ClusterQueueName
	if clusterQueueName == "" && localQueue != nil {
		clusterQueueName = localQueue.ClusterQueueName
	}
	_ = c.extractKueueQueueTagsFromQueueName(workload.Namespace, tagList, workloadmeta.KueueClusterQueue, clusterQueueName)

	// now parse the pod set assignments to get the flavor names and add the corresponding tags
	flavorNames := make(map[string]struct{})
	for _, assignment := range workload.PodSetAssignments {
		if podSetName != "" && assignment.Name != podSetName {
			continue
		}
		for _, flavorName := range assignment.Flavors {
			if flavorName != "" {
				flavorNames[flavorName] = struct{}{}
			}
		}
	}

	for flavorName := range flavorNames {
		flavorID := workloadmeta.GenerateKueueResourceFlavorEntityID(flavorName)
		flavor, err := c.store.GetKubernetesKueueResourceFlavor(flavorID)
		if err != nil || flavor == nil {
			// add just the flavor name to the tag list as a fallback
			tagList.AddLow(tags.KueueResourceFlavor, flavorName)
			continue
		}

		c.extractKueueResourceFlavorTags(flavor, tagList)
	}
}

func (c *WorkloadMetaCollector) extractKueueQueueTagsFromQueueName(namespace string, tagList *taglist.TagList, queueType workloadmeta.KueueQueueType, queueName string) *workloadmeta.KubernetesKueueQueue {
	if queueName == "" {
		return nil
	}

	// add the queue name to the tag list as a fallback. If the queue entity is available, it will have the same
	// name and the taglist handles the de-duplication. If the queue entity is not available, we will at least have
	// the name in the tag list.
	if queueType == workloadmeta.KueueLocalQueue {
		tagList.AddLow(tags.KueueLocalQueue, queueName)
	} else {
		tagList.AddLow(tags.KueueClusterQueue, queueName)
	}

	// now get the queue entity for more tags
	queueID, err := workloadmeta.GenerateKueueQueueEntityID(queueType, namespace, queueName)
	if err != nil {
		log.Debugf("Could not generate Kueue %s entity ID for namespace %s and name %s: %v", queueType, namespace, queueName, err)
		return nil
	}

	queue, err := c.store.GetKubernetesKueueQueue(queueID)
	if err != nil || queue == nil {
		log.Debugf("Could not get Kueue %s entity for namespace %s and name %s: %v", queueType, namespace, queueName, err)
		return nil
	}

	c.extractKueueQueueTags(queue, tagList)

	return queue
}

func nvidiaResourceFlavorNodeLabelTagName(labelName string) (string, bool) {
	const nvidiaLabelPrefix = "nvidia.com/"
	tagName, ok := strings.CutPrefix(labelName, nvidiaLabelPrefix)
	if !ok || tagName == "" {
		return "", false
	}
	return strings.ReplaceAll(tagName, ".", "_"), true
}

func kueueQueueGroupResource(queueType workloadmeta.KueueQueueType) string {
	switch queueType {
	case workloadmeta.KueueLocalQueue:
		return kubernetes.KueueLocalQueueResourceName + "." + kubernetes.KueueGroupName
	case workloadmeta.KueueClusterQueue:
		return kubernetes.KueueClusterQueueResourceName + "." + kubernetes.KueueGroupName
	default:
		return ""
	}
}

func (c *WorkloadMetaCollector) extractTagsFromPodKueueInfo(pod *workloadmeta.KubernetesPod, tagList *taglist.TagList) {
	// The associated workload object is the main source of information for Kueue. If it is available, we use it to extract the tags
	// If not, we fall back to the pod labels and annotations to get the queue names and their tags
	workload := c.getKueueWorkloadForPod(pod)
	if workload != nil {
		c.extractKueueWorkloadAndRelatedTags(workload, pod.Labels[kubernetes.KueuePodSetLabelKey], tagList)
		return
	}

	clusterQueueName := pod.Labels[kubernetes.KueueClusterQueueNameLabelKey]
	localQueueName := pod.Labels[kubernetes.KueueLocalQueueNameLabelKey]
	if localQueueName == "" {
		// plain pods will not have the local-queue-name label but instead the queue-name one, so
		// fall back to that one
		localQueueName = pod.Labels[kubernetes.KueueQueueNameLabelKey]
	}

	localQueue := c.extractKueueQueueTagsFromQueueName(pod.Namespace, tagList, workloadmeta.KueueLocalQueue, localQueueName)

	if clusterQueueName == "" && localQueue != nil {
		// Local queues are always associated to a cluster queue, so if we don't have a cluster queue name in the pod,
		// use the one associated to the local queue
		clusterQueueName = localQueue.ClusterQueueName
	}
	_ = c.extractKueueQueueTagsFromQueueName(pod.Namespace, tagList, workloadmeta.KueueClusterQueue, clusterQueueName)
}

func (c *WorkloadMetaCollector) getKueueWorkloadForPod(pod *workloadmeta.KubernetesPod) *workloadmeta.KubernetesKueueWorkload {
	workloadName := pod.Annotations[kubernetes.KueueWorkloadAnnotationKey]
	if workloadName == "" {
		// Known limitation: for plain-Pod groups the Kueue Workload object name
		// is not guaranteed to equal the pod-group-name label value. When they
		// diverge, the lookup below fails and we fall back to pod-label queue
		// tags (fail-closed, no incorrect tags).
		workloadName = pod.Labels[kubernetes.KueuePodGroupNameLabelKey]
	}
	if workloadName == "" {
		return nil
	}

	workloadID := workloadmeta.GenerateKueueWorkloadEntityID(pod.Namespace, workloadName)
	workload, err := c.store.GetKubernetesKueueWorkload(workloadID)
	if err != nil || workload == nil {
		log.Debugf("Could not get Kueue workload entity for namespace %s and name %s: %v", pod.Namespace, workloadName, err)
		return nil
	}

	return workload
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
	annotation := fmt.Sprintf(kubernetes.ADContainerTagsAnnotationFormat, containerName)
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

// containerCompleteness computes the effective completeness for a container.
// Container tags depend on data from a parent entity (pod in Kubernetes, ECS
// task in ECS), so completeness requires both the container and its parent to
// be complete.
func (c *WorkloadMetaCollector) containerCompleteness(containerID string, containerComplete bool) bool {
	if env.IsFeaturePresent(env.Kubernetes) {
		return c.containerCompletenessKubernetes(containerID, containerComplete)
	}

	if env.IsFeaturePresent(env.ECSEC2) || env.IsFeaturePresent(env.ECSManagedInstances) {
		return c.containerCompletenessECS(containerID, containerComplete)
	}

	return containerComplete
}

func (c *WorkloadMetaCollector) containerCompletenessKubernetes(containerID string, containerComplete bool) bool {
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

func (c *WorkloadMetaCollector) containerCompletenessECS(containerID string, containerComplete bool) bool {
	if !containerComplete {
		return false
	}

	container, err := c.store.GetContainer(containerID)
	if err != nil {
		return false
	}

	if container.Owner == nil || container.Owner.Kind != workloadmeta.KindECSTask {
		return false
	}

	taskComplete, ok := c.entityCompleteness[*container.Owner]
	if !ok {
		return false
	}

	return taskComplete
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
