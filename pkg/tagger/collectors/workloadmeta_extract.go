// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package collectors

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/tagger/utils"
	"github.com/DataDog/datadog-agent/pkg/util/containers"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes/kubelet"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/datadog-agent/pkg/workloadmeta"
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
	var tagInfos []*TagInfo

	for _, ev := range evBundle.Events {
		entity := ev.Entity
		entityID := entity.GetID()

		switch ev.Type {
		case workloadmeta.EventTypeSet:
			taggerEntityID := buildTaggerEntityID(entityID)

			// keep track of children of this entity from previous
			// iterations ...
			unseen := make(map[string]struct{})
			for childTaggerID := range c.children[taggerEntityID] {
				unseen[childTaggerID] = struct{}{}
			}

			// ... and create a new empty map to store the children
			// seen in this iteration.
			c.children[taggerEntityID] = make(map[string]struct{})

			switch entityID.Kind {
			case workloadmeta.KindContainer:
				tagInfos = append(tagInfos, c.handleContainer(ev)...)
			case workloadmeta.KindKubernetesPod:
				tagInfos = append(tagInfos, c.handleKubePod(ev)...)
			case workloadmeta.KindKubernetesNode:
				tagInfos = append(tagInfos, c.handleKubeNode(ev)...)
			case workloadmeta.KindECSTask:
				tagInfos = append(tagInfos, c.handleECSTask(ev)...)
			case workloadmeta.KindContainerImageMetadata:
				tagInfos = append(tagInfos, c.handleContainerImage(ev)...)
			case workloadmeta.KindProcess:
				// tagInfos = append(tagInfos, c.handleProcess(ev)...) No tags for now
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

	close(evBundle.Ch)
}

func (c *WorkloadMetaCollector) handleContainer(ev workloadmeta.Event) []*TagInfo {
	container := ev.Entity.(*workloadmeta.Container)

	// Garden containers tagging is specific as we don't have any information locally
	// Metadata are not available and tags are retrieved as-is from Cluster Agent
	if container.Runtime == workloadmeta.ContainerRuntimeGarden {
		return c.handleGardenContainer(container)
	}

	tags := utils.NewTagList()
	tags.AddHigh("container_name", container.Name)
	tags.AddHigh("container_id", container.ID)

	image := container.Image
	tags.AddLow("image_name", image.Name)
	tags.AddLow("short_image", image.ShortName)
	tags.AddLow("image_tag", image.Tag)
	tags.AddLow("image_id", image.ID)

	if container.Runtime == workloadmeta.ContainerRuntimeDocker {
		if image.Tag != "" {
			tags.AddLow("docker_image", fmt.Sprintf("%s:%s", image.Name, image.Tag))
		} else {
			tags.AddLow("docker_image", image.Name)
		}
	}

	c.labelsToTags(container.Labels, tags)

	// standard tags from environment
	c.extractFromMapWithFn(container.EnvVars, standardEnvKeys, tags.AddStandard)

	// orchestrator tags from environment
	c.extractFromMapWithFn(container.EnvVars, lowCardOrchestratorEnvKeys, tags.AddLow)
	c.extractFromMapWithFn(container.EnvVars, orchCardOrchestratorEnvKeys, tags.AddOrchestrator)

	// extract env as tags
	for envName, envValue := range container.EnvVars {
		utils.AddMetadataAsTags(envName, envValue, c.containerEnvAsTags, c.globContainerEnvLabels, tags)
	}

	// static tags for ECS and EKS Fargate containers
	for tag, value := range c.staticTags {
		tags.AddLow(tag, value)
	}

	low, orch, high, standard := tags.Compute()
	return []*TagInfo{
		{
			Source:               containerSource,
			Entity:               buildTaggerEntityID(container.EntityID),
			HighCardTags:         high,
			OrchestratorCardTags: orch,
			LowCardTags:          low,
			StandardTags:         standard,
		},
	}
}

func (c *WorkloadMetaCollector) handleContainerImage(ev workloadmeta.Event) []*TagInfo {
	image := ev.Entity.(*workloadmeta.ContainerImageMetadata)

	tags := utils.NewTagList()
	tags.AddLow("image_name", image.Name)

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
		tags.AddLow("short_image", shortName)
	}

	for _, repoTag := range image.RepoTags {
		repoTagSplitted := strings.SplitN(repoTag, ":", 2)
		if len(repoTagSplitted) == 2 {
			tags.AddLow("image_tag", repoTagSplitted[1])
		}
	}

	tags.AddLow("os_name", image.OS)
	tags.AddLow("os_version", image.OSVersion)
	tags.AddLow("architecture", image.Architecture)

	c.labelsToTags(image.Labels, tags)

	low, orch, high, standard := tags.Compute()
	return []*TagInfo{
		{
			Source:               containerImageSource,
			Entity:               buildTaggerEntityID(image.EntityID),
			HighCardTags:         high,
			OrchestratorCardTags: orch,
			LowCardTags:          low,
			StandardTags:         standard,
		},
	}
}

func (c *WorkloadMetaCollector) labelsToTags(labels map[string]string, tags *utils.TagList) {
	// standard tags from labels
	c.extractFromMapWithFn(labels, standardDockerLabels, tags.AddStandard)

	// container labels as tags
	for labelName, labelValue := range labels {
		utils.AddMetadataAsTags(labelName, labelValue, c.containerLabelsAsTags, c.globContainerLabels, tags)
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

func (c *WorkloadMetaCollector) handleKubePod(ev workloadmeta.Event) []*TagInfo {
	pod := ev.Entity.(*workloadmeta.KubernetesPod)

	tags := utils.NewTagList()
	tags.AddOrchestrator(kubernetes.PodTagName, pod.Name)
	tags.AddLow(kubernetes.NamespaceTagName, pod.Namespace)
	tags.AddLow("pod_phase", strings.ToLower(pod.Phase))
	tags.AddLow("kube_priority_class", pod.PriorityClass)
	tags.AddLow("kube_qos", pod.QOSClass)

	c.extractTagsFromPodLabels(pod, tags)

	for name, value := range pod.Annotations {
		utils.AddMetadataAsTags(name, value, c.annotationsAsTags, c.globAnnotations, tags)
	}

	for name, value := range pod.NamespaceLabels {
		utils.AddMetadataAsTags(name, value, c.nsLabelsAsTags, c.globNsLabels, tags)
	}

	kubeServiceDisabled := false
	for _, disabledTag := range config.Datadog.GetStringSlice("kubernetes_ad_tags_disabled") {
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
			tags.AddLow("kube_service", svc)
		}
	}

	c.extractTagsFromJSONInMap(podTagsAnnotation, pod.Annotations, tags)

	// OpenShift pod annotations
	if dcName, found := pod.Annotations["openshift.io/deployment-config.name"]; found {
		tags.AddLow("oshift_deployment_config", dcName)
	}
	if deployName, found := pod.Annotations["openshift.io/deployment.name"]; found {
		tags.AddOrchestrator("oshift_deployment", deployName)
	}

	// Admission + Remote Config correlation tags
	if rcID, found := pod.Annotations[kubernetes.RcIDAnnotKey]; found {
		tags.AddLow(kubernetes.RcIDTagName, rcID)
	}
	if rcRev, found := pod.Annotations[kubernetes.RcRevisionAnnotKey]; found {
		tags.AddLow(kubernetes.RcRevisionTagName, rcRev)
	}

	for _, owner := range pod.Owners {
		tags.AddLow(kubernetes.OwnerRefKindTagName, strings.ToLower(owner.Kind))
		tags.AddOrchestrator(kubernetes.OwnerRefNameTagName, owner.Name)

		c.extractTagsFromPodOwner(pod, owner, tags)
	}

	// static tags for EKS Fargate pods
	for tag, value := range c.staticTags {
		tags.AddLow(tag, value)
	}

	low, orch, high, standard := tags.Compute()
	tagInfos := []*TagInfo{
		{
			Source:               podSource,
			Entity:               buildTaggerEntityID(pod.EntityID),
			HighCardTags:         high,
			OrchestratorCardTags: orch,
			LowCardTags:          low,
			StandardTags:         standard,
		},
	}

	for _, podContainer := range pod.Containers {
		cTagInfo, err := c.extractTagsFromPodContainer(pod, podContainer, tags.Copy())
		if err != nil {
			log.Debugf("cannot extract tags from pod container: %s", err)
			continue
		}

		tagInfos = append(tagInfos, cTagInfo)
	}

	return tagInfos
}

func (c *WorkloadMetaCollector) handleKubeNode(ev workloadmeta.Event) []*TagInfo {
	node := ev.Entity.(*workloadmeta.KubernetesNode)

	tags := utils.NewTagList()

	// Add tags for node here

	low, orch, high, standard := tags.Compute()
	tagInfos := []*TagInfo{
		{
			Source:               nodeSource,
			Entity:               buildTaggerEntityID(node.EntityID),
			HighCardTags:         high,
			OrchestratorCardTags: orch,
			LowCardTags:          low,
			StandardTags:         standard,
		},
	}

	return tagInfos
}

func (c *WorkloadMetaCollector) handleECSTask(ev workloadmeta.Event) []*TagInfo {
	task := ev.Entity.(*workloadmeta.ECSTask)

	taskTags := utils.NewTagList()

	// as of Agent 7.33, tasks have a name internally, but before that
	// task_name already was task.Family, so we keep it for backwards
	// compatibility
	taskTags.AddLow("task_name", task.Family)
	taskTags.AddLow("task_family", task.Family)
	taskTags.AddLow("task_version", task.Version)
	taskTags.AddOrchestrator("task_arn", task.ID)

	if task.ClusterName != "" {
		if !config.Datadog.GetBool("disable_cluster_name_tag_key") {
			taskTags.AddLow("cluster_name", task.ClusterName)
		}
		taskTags.AddLow("ecs_cluster_name", task.ClusterName)
	}

	if task.LaunchType == workloadmeta.ECSLaunchTypeFargate {
		taskTags.AddLow("region", task.Region)
		taskTags.AddLow("availability_zone", task.AvailabilityZone) // Deprecated
		taskTags.AddLow("availability-zone", task.AvailabilityZone)
	} else if c.collectEC2ResourceTags {
		addResourceTags(taskTags, task.ContainerInstanceTags)
		addResourceTags(taskTags, task.Tags)
	}

	tagInfos := make([]*TagInfo, 0, len(task.Containers))
	for _, taskContainer := range task.Containers {
		container, err := c.store.GetContainer(taskContainer.ID)
		if err != nil {
			log.Debugf("task %q has reference to non-existing container %q", task.Name, taskContainer.ID)
			continue
		}

		c.registerChild(task.EntityID, container.EntityID)

		tags := taskTags.Copy()

		tags.AddLow("ecs_container_name", taskContainer.Name)

		low, orch, high, standard := tags.Compute()
		tagInfos = append(tagInfos, &TagInfo{
			// taskSource here is not a mistake. the source is
			// always from the parent resource.
			Source:               taskSource,
			Entity:               buildTaggerEntityID(container.EntityID),
			HighCardTags:         high,
			OrchestratorCardTags: orch,
			LowCardTags:          low,
			StandardTags:         standard,
		})
	}

	if task.LaunchType == workloadmeta.ECSLaunchTypeFargate {
		low, orch, high, standard := taskTags.Compute()
		tagInfos = append(tagInfos, &TagInfo{
			Source:               taskSource,
			Entity:               GlobalEntityID,
			HighCardTags:         high,
			OrchestratorCardTags: orch,
			LowCardTags:          low,
			StandardTags:         standard,
		})
	}

	return tagInfos
}

func (c *WorkloadMetaCollector) handleGardenContainer(container *workloadmeta.Container) []*TagInfo {
	return []*TagInfo{
		{
			Source:       containerSource,
			Entity:       buildTaggerEntityID(container.EntityID),
			HighCardTags: container.CollectorTags,
		},
	}
}

func (c *WorkloadMetaCollector) extractTagsFromPodLabels(pod *workloadmeta.KubernetesPod, tags *utils.TagList) {
	for name, value := range pod.Labels {
		switch name {
		case kubernetes.EnvTagLabelKey:
			tags.AddStandard(tagKeyEnv, value)
		case kubernetes.VersionTagLabelKey:
			tags.AddStandard(tagKeyVersion, value)
		case kubernetes.ServiceTagLabelKey:
			tags.AddStandard(tagKeyService, value)
		case kubernetes.KubeAppNameLabelKey:
			tags.AddLow(tagKeyKubeAppName, value)
		case kubernetes.KubeAppInstanceLabelKey:
			tags.AddLow(tagKeyKubeAppInstance, value)
		case kubernetes.KubeAppVersionLabelKey:
			tags.AddLow(tagKeyKubeAppVersion, value)
		case kubernetes.KubeAppComponentLabelKey:
			tags.AddLow(tagKeyKubeAppComponent, value)
		case kubernetes.KubeAppPartOfLabelKey:
			tags.AddLow(tagKeyKubeAppPartOf, value)
		case kubernetes.KubeAppManagedByLabelKey:
			tags.AddLow(tagKeyKubeAppManagedBy, value)
		}

		utils.AddMetadataAsTags(name, value, c.labelsAsTags, c.globLabels, tags)
	}
}

func (c *WorkloadMetaCollector) extractTagsFromPodOwner(pod *workloadmeta.KubernetesPod, owner workloadmeta.KubernetesPodOwner, tags *utils.TagList) {
	switch owner.Kind {
	case kubernetes.DeploymentKind:
		tags.AddLow(kubernetes.DeploymentTagName, owner.Name)

	case kubernetes.DaemonSetKind:
		tags.AddLow(kubernetes.DaemonSetTagName, owner.Name)

	case kubernetes.ReplicationControllerKind:
		tags.AddLow(kubernetes.ReplicationControllerTagName, owner.Name)

	case kubernetes.StatefulSetKind:
		tags.AddLow(kubernetes.StatefulSetTagName, owner.Name)
		for _, pvc := range pod.PersistentVolumeClaimNames {
			if pvc != "" {
				tags.AddLow(kubernetes.PersistentVolumeClaimTagName, pvc)
			}
		}

	case kubernetes.JobKind:
		cronjob, _ := kubernetes.ParseCronJobForJob(owner.Name)
		if cronjob != "" {
			tags.AddOrchestrator(kubernetes.JobTagName, owner.Name)
			tags.AddLow(kubernetes.CronJobTagName, cronjob)
		} else {
			tags.AddLow(kubernetes.JobTagName, owner.Name)
		}

	case kubernetes.ReplicaSetKind:
		deployment := kubernetes.ParseDeploymentForReplicaSet(owner.Name)
		if len(deployment) > 0 {
			tags.AddLow(kubernetes.DeploymentTagName, deployment)
		}
		tags.AddLow(kubernetes.ReplicaSetTagName, owner.Name)
	}
}

func (c *WorkloadMetaCollector) extractTagsFromPodContainer(pod *workloadmeta.KubernetesPod, podContainer workloadmeta.OrchestratorContainer, tags *utils.TagList) (*TagInfo, error) {
	container, err := c.store.GetContainer(podContainer.ID)
	if err != nil {
		return nil, fmt.Errorf("pod %q has reference to non-existing container %q", pod.Name, podContainer.ID)
	}

	c.registerChild(pod.EntityID, container.EntityID)

	tags.AddLow("kube_container_name", podContainer.Name)
	tags.AddHigh("container_id", container.ID)

	if container.Name != "" && pod.Name != "" {
		tags.AddHigh("display_container_name", fmt.Sprintf("%s_%s", container.Name, pod.Name))
	}

	image := podContainer.Image
	tags.AddLow("image_name", image.Name)
	tags.AddLow("short_image", image.ShortName)
	tags.AddLow("image_tag", image.Tag)
	tags.AddLow("image_id", image.ID)

	// enrich with standard tags from labels for this container if present
	containerName := podContainer.Name
	standardTagKeys := map[string]string{
		fmt.Sprintf(podStandardLabelPrefix+"%s.%s", containerName, tagKeyEnv):     tagKeyEnv,
		fmt.Sprintf(podStandardLabelPrefix+"%s.%s", containerName, tagKeyVersion): tagKeyVersion,
		fmt.Sprintf(podStandardLabelPrefix+"%s.%s", containerName, tagKeyService): tagKeyService,
	}
	c.extractFromMapWithFn(pod.Labels, standardTagKeys, tags.AddStandard)

	// enrich with standard tags from environment variables
	c.extractFromMapWithFn(container.EnvVars, standardEnvKeys, tags.AddStandard)

	// container-specific tags provided through pod annotation
	annotation := fmt.Sprintf(podContainerTagsAnnotationFormat, containerName)
	c.extractTagsFromJSONInMap(annotation, pod.Annotations, tags)

	low, orch, high, standard := tags.Compute()
	return &TagInfo{
		// podSource here is not a mistake. the source is
		// always from the parent resource.
		Source:               podSource,
		Entity:               buildTaggerEntityID(container.EntityID),
		HighCardTags:         high,
		OrchestratorCardTags: orch,
		LowCardTags:          low,
		StandardTags:         standard,
	}, nil
}

func (c *WorkloadMetaCollector) registerChild(parent, child workloadmeta.EntityID) {
	parentTaggerEntityID := buildTaggerEntityID(parent)
	childTaggerEntityID := buildTaggerEntityID(child)

	m, ok := c.children[parentTaggerEntityID]
	if !ok {
		c.children[parentTaggerEntityID] = make(map[string]struct{})
		m = c.children[parentTaggerEntityID]
	}

	m[childTaggerEntityID] = struct{}{}
}

func (c *WorkloadMetaCollector) handleDelete(ev workloadmeta.Event) []*TagInfo {
	entityID := ev.Entity.GetID()
	taggerEntityID := buildTaggerEntityID(entityID)

	children := c.children[taggerEntityID]

	source := buildTaggerSource(entityID)
	tagInfos := make([]*TagInfo, 0, len(children)+1)
	tagInfos = append(tagInfos, &TagInfo{
		Source:       source,
		Entity:       taggerEntityID,
		DeleteEntity: true,
	})
	tagInfos = append(tagInfos, c.handleDeleteChildren(source, children)...)

	delete(c.children, taggerEntityID)

	return tagInfos
}

func (c *WorkloadMetaCollector) handleDeleteChildren(source string, children map[string]struct{}) []*TagInfo {
	tagInfos := make([]*TagInfo, 0, len(children))

	for childEntityID := range children {
		t := TagInfo{
			Source:       source,
			Entity:       childEntityID,
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

func (c *WorkloadMetaCollector) extractTagsFromJSONInMap(key string, input map[string]string, tags *utils.TagList) {
	jsonTags, found := input[key]
	if !found {
		return
	}

	err := parseJSONValue(jsonTags, tags)
	if err != nil {
		log.Errorf("can't parse value for annotation %s: %s", key, err)
	}
}

func buildTaggerEntityID(entityID workloadmeta.EntityID) string {
	switch entityID.Kind {
	case workloadmeta.KindContainer:
		return containers.BuildTaggerEntityName(entityID.ID)
	case workloadmeta.KindKubernetesPod:
		return kubelet.PodUIDToTaggerEntityName(entityID.ID)
	case workloadmeta.KindKubernetesNode:
		return kubelet.NodeUIDToTaggerEntityName(entityID.ID)
	case workloadmeta.KindECSTask:
		return fmt.Sprintf("ecs_task://%s", entityID.ID)
	case workloadmeta.KindContainerImageMetadata:
		return fmt.Sprintf("container_image_metadata://%s", entityID.ID)
	case workloadmeta.KindProcess:
		return fmt.Sprintf("process://%s", entityID.ID)
	default:
		log.Errorf("can't recognize entity %q with kind %q; trying %s://%s as tagger entity",
			entityID.ID, entityID.Kind, entityID.ID, entityID.Kind)
		return fmt.Sprintf("%s://%s", string(entityID.Kind), entityID.ID)
	}
}

func buildTaggerSource(entityID workloadmeta.EntityID) string {
	return fmt.Sprintf("%s-%s", workloadmetaCollectorName, string(entityID.Kind))
}

func parseJSONValue(value string, tags *utils.TagList) error {
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

func parseContainerADTagsLabels(tags *utils.TagList, labelValue string) {
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

//lint:ignore U1000 Ignore unused function until the collector is implemented
func (c *WorkloadMetaCollector) handleProcess(ev workloadmeta.Event) []*TagInfo {
	process := ev.Entity.(*workloadmeta.Process)
	tags := utils.NewTagList()
	if process.Language != nil {
		tags.AddLow("language", string(process.Language.Name))
	}
	low, orch, high, standard := tags.Compute()
	return []*TagInfo{{
		Source:               processSource,
		Entity:               buildTaggerEntityID(process.EntityID),
		HighCardTags:         high,
		OrchestratorCardTags: orch,
		LowCardTags:          low,
		StandardTags:         standard,
	},
	}
}
