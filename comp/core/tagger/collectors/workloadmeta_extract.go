// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package collectors

import (
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/DataDog/datadog-agent/comp/core/tagger/common"
	k8smetadata "github.com/DataDog/datadog-agent/comp/core/tagger/k8s_metadata"
	"github.com/DataDog/datadog-agent/comp/core/tagger/taglist"
	"github.com/DataDog/datadog-agent/comp/core/tagger/tags"
	"github.com/DataDog/datadog-agent/comp/core/tagger/types"
	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	pkgconfigsetup "github.com/DataDog/datadog-agent/pkg/config/setup"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes"
	"github.com/DataDog/datadog-agent/pkg/util/log"
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
		"service.name":           tags.Service,
		"service.version":        tags.Version,
		"deployment.environment": tags.Env,
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
)

func (c *WorkloadMetaCollector) processEvents(evBundle workloadmeta.EventBundle) {
	var tagInfos []*types.TagInfo

	for _, ev := range evBundle.Events {
		entity := ev.Entity
		entityID := entity.GetID()

		switch ev.Type {
		case workloadmeta.EventTypeSet:
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
				// tagInfos = append(tagInfos, c.handleProcess(ev)...) No tags for now
			case workloadmeta.KindKubernetesDeployment:
				tagInfos = append(tagInfos, c.handleKubeDeployment(ev)...)
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

	// Garden containers tagging is specific as we don't have any information locally
	// Metadata are not available and tags are retrieved as-is from Cluster Agent
	if container.Runtime == workloadmeta.ContainerRuntimeGarden {
		return c.handleGardenContainer(container)
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
			tagList.AddLow(tags.DockerImage, fmt.Sprintf("%s:%s", image.Name, image.Tag))
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
	for tag, value := range c.staticTags {
		tagList.AddLow(tag, value)
	}

	// gpu tags from container resource requests
	for _, gpuVendor := range container.Resources.GPUVendorList {
		tagList.AddLow(tags.KubeGPUVendor, gpuVendor)
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

func (c *WorkloadMetaCollector) extractTagsFromPodEntity(pod *workloadmeta.KubernetesPod, tagList *taglist.TagList) *types.TagInfo {
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

	kubeServiceDisabled := false
	for _, disabledTag := range pkgconfigsetup.Datadog().GetStringSlice("kubernetes_ad_tags_disabled") {
		if disabledTag == "kube_service" {
			kubeServiceDisabled = true
			break
		}
	}
	for _, disabledTag := range strings.Split(pod.Annotations["tags.datadoghq.com/disable"], ",") {
		if disabledTag == "kube_service" {
			kubeServiceDisabled = true
			break
		}
	}
	if !kubeServiceDisabled {
		for _, svc := range pod.KubeServices {
			tagList.AddLow(tags.KubeService, svc)
		}
	}

	c.extractTagsFromJSONInMap(podTagsAnnotation, pod.Annotations, tagList)

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
	for tag, value := range c.staticTags {
		tagList.AddLow(tag, value)
	}

	low, orch, high, standard := tagList.Compute()
	tagInfo := &types.TagInfo{
		Source:               podSource,
		EntityID:             common.BuildTaggerEntityID(pod.EntityID),
		HighCardTags:         high,
		OrchestratorCardTags: orch,
		LowCardTags:          low,
		StandardTags:         standard,
	}

	return tagInfo
}

func (c *WorkloadMetaCollector) handleKubePod(ev workloadmeta.Event) []*types.TagInfo {
	pod := ev.Entity.(*workloadmeta.KubernetesPod)
	tagList := taglist.NewTagList()
	tagInfos := []*types.TagInfo{c.extractTagsFromPodEntity(pod, tagList)}

	c.extractTagsFromPodLabels(pod, tagList)

	for _, podContainer := range pod.GetAllContainers() {
		cTagInfo, err := c.extractTagsFromPodContainer(pod, podContainer, tagList.Copy())
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
	taskTags.AddLow(tags.AwsAccount, strconv.Itoa(task.AWSAccountID))
	taskTags.AddLow(tags.Region, task.Region)
	taskTags.AddOrchestrator(tags.TaskARN, task.ID)

	if task.ClusterName != "" {
		if !pkgconfigsetup.Datadog().GetBool("disable_cluster_name_tag_key") {
			taskTags.AddLow(tags.ClusterName, task.ClusterName)
		}
		taskTags.AddLow(tags.EcsClusterName, task.ClusterName)
	}

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
			LowCardTags:          low,
			StandardTags:         standard,
		})
	}

	if task.LaunchType == workloadmeta.ECSLaunchTypeFargate {
		low, orch, high, standard := taskTags.Compute()
		tagInfos = append(tagInfos, &types.TagInfo{
			Source:               taskSource,
			EntityID:             common.GetGlobalEntityID(),
			HighCardTags:         high,
			OrchestratorCardTags: orch,
			LowCardTags:          low,
			StandardTags:         standard,
		})
	}

	return tagInfos
}

func (c *WorkloadMetaCollector) handleGardenContainer(container *workloadmeta.Container) []*types.TagInfo {
	return []*types.TagInfo{
		{
			Source:       containerSource,
			EntityID:     common.BuildTaggerEntityID(container.EntityID),
			HighCardTags: container.CollectorTags,
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
		}
		tagList.AddLow(tags.KubeReplicaSet, owner.Name)
	}
}

func (c *WorkloadMetaCollector) extractTagsFromPodContainer(pod *workloadmeta.KubernetesPod, podContainer workloadmeta.OrchestratorContainer, tagList *taglist.TagList) (*types.TagInfo, error) {
	container, err := c.store.GetContainer(podContainer.ID)
	if err != nil {
		return nil, fmt.Errorf("pod %q has reference to non-existing container %q", pod.Name, podContainer.ID)
	}

	c.registerChild(pod.EntityID, container.EntityID)

	tagList.AddLow(tags.KubeContainerName, podContainer.Name)
	tagList.AddHigh(tags.ContainerID, container.ID)

	if container.Name != "" && pod.Name != "" {
		tagList.AddHigh(tags.DisplayContainerName, fmt.Sprintf("%s_%s", container.Name, pod.Name))
	} else if podContainer.Name != "" && pod.Name != "" {
		tagList.AddHigh(tags.DisplayContainerName, fmt.Sprintf("%s_%s", podContainer.Name, pod.Name))
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
	c.extractTagsFromJSONInMap(annotation, pod.Annotations, tagList)

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

	return tagInfos
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

func (c *WorkloadMetaCollector) extractTagsFromJSONInMap(key string, input map[string]string, tags *taglist.TagList) {
	jsonTags, found := input[key]
	if !found {
		return
	}

	err := parseJSONValue(jsonTags, tags)
	if err != nil {
		log.Errorf("can't parse value for annotation %s: %s", key, err)
	}
}

func (c *WorkloadMetaCollector) addOpenTelemetryStandardTags(container *workloadmeta.Container, tags *taglist.TagList) {
	if otelResourceAttributes, ok := container.EnvVars[envVarOtelResourceAttributes]; ok {
		for _, pair := range strings.Split(otelResourceAttributes, ",") {
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
	return fmt.Sprintf("%s-%s", workloadmetaCollectorName, string(entityID.Kind))
}

func parseJSONValue(value string, tags *taglist.TagList) error {
	if value == "" {
		return errors.New("value is empty")
	}

	result := map[string]interface{}{}
	if err := json.Unmarshal([]byte(value), &result); err != nil {
		return fmt.Errorf("failed to unmarshal JSON: %s", err)
	}

	for key, value := range result {
		switch v := value.(type) {
		case string:
			tags.AddAuto(key, v)
		case []interface{}:
			for _, tag := range v {
				tags.AddAuto(key, fmt.Sprint(tag))
			}
		default:
			log.Debugf("Tag value %s is not valid, must be a string or an array, skipping", v)
		}
	}

	return nil
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
