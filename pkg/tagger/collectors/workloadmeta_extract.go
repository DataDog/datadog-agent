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
	// OrchestratorScopeEntityID defines the orchestrator scope entity ID
	OrchestratorScopeEntityID = "internal://orchestrator-scope-entity-id"

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

	k8sLabelPodUID  = "io.kubernetes.pod.uid"
	ecsLabelTaskARN = "com.amazonaws.ecs.task-arn"
)

var (
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
			switch entityID.Kind {
			case workloadmeta.KindContainer:
				tagInfos = append(tagInfos, c.handleContainer(ev)...)
			case workloadmeta.KindKubernetesPod:
				tagInfos = append(tagInfos, c.handleKubePod(ev)...)
			case workloadmeta.KindECSTask:
				tagInfos = append(tagInfos, c.handleECSTask(ev)...)
			case workloadmeta.KindGardenContainer:
				tagInfos = append(tagInfos, c.handleGardenContainer(ev)...)
			default:
				log.Errorf("cannot handle event for entity %q with kind %q", entityID.ID, entityID.Kind)
			}

		case workloadmeta.EventTypeUnset:
			tagInfos = append(tagInfos, c.handleDelete(ev)...)

		default:
			log.Errorf("cannot handle event of type %d", ev.Type)
		}

	}

	// NOTE: haha, this is still async and race conditions will still
	// happen :D since the workloadmeta will be the only collector in the
	// tagger in the end, this can be turned into a sync call to
	// processTagInfo
	if len(tagInfos) > 0 {
		c.out <- tagInfos
	}

	close(evBundle.Ch)
}

func (c *WorkloadMetaCollector) handleContainer(ev workloadmeta.Event) []*TagInfo {
	container := ev.Entity.(*workloadmeta.Container)
	image := container.Image

	tags := utils.NewTagList()
	tags.AddHigh("container_name", container.Name)
	tags.AddHigh("container_id", container.ID)

	var kubeTags bool
	if podID, ok := container.Labels[k8sLabelPodUID]; ok {
		pod, err := c.store.GetKubernetesPod(podID)
		if err == nil {
			err = c.extractTagsFromPodContainer(pod, container, tags)
		}

		if err == nil {
			kubeTags = true
		} else {
			log.Debugf("container %q cannot get tags from pod %s: %s (sources: %s)", container.ID, podID, err, ev.Sources)
		}
	}

	if !kubeTags {
		// we only collect image names and tags from the container
		// runtime itself outside of kubernetes, as an image name in
		// the pod might not match the image name resolved by the
		// container runtime. for instance, "datadog/agent" in the pod
		// spec might be resolved to "docker.io/datadog/agent" by the
		// runtime, and might confuse users (and break backwards
		// compatibility!)
		tags.AddLow("image_name", image.Name)
		tags.AddLow("short_image", image.ShortName)
		tags.AddLow("image_tag", image.Tag)
	}

	if taskARN, ok := container.Labels[ecsLabelTaskARN]; ok {
		task, err := c.store.GetECSTask(taskARN)
		if err == nil {
			err = c.extractTagsFromTaskContainer(task, container, tags)
		}

		if err != nil {
			log.Debugf("container %q cannot get tags from task %s: %s", container.ID, taskARN, err)
		}
	}

	if container.Runtime == workloadmeta.ContainerRuntimeDocker {
		if image.Tag != "" {
			tags.AddLow("docker_image", fmt.Sprintf("%s:%s", image.Name, image.Tag))
		} else {
			tags.AddLow("docker_image", image.Name)
		}
	}

	// standard tags from labels
	c.extractFromMapWithFn(container.Labels, standardDockerLabels, tags.AddStandard)

	// container labels as tags
	for labelName, labelValue := range container.Labels {
		utils.AddMetadataAsTags(labelName, labelValue, c.containerLabelsAsTags, c.globContainerLabels, tags)
	}

	// orchestrator tags from labels
	c.extractFromMapWithFn(container.Labels, lowCardOrchestratorLabels, tags.AddLow)
	c.extractFromMapWithFn(container.Labels, highCardOrchestratorLabels, tags.AddHigh)

	// extract labels as tags
	c.extractFromMapNormalizedWithFn(container.Labels, c.containerLabelsAsTags, tags.AddAuto)

	// custom tags from label
	if lbl, ok := container.Labels[autodiscoveryLabelTagsKey]; ok {
		parseContainerADTagsLabels(tags, lbl)
	}

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
			Entity:               buildTaggerEntityID(container.EntityID),
			HighCardTags:         high,
			OrchestratorCardTags: orch,
			LowCardTags:          low,
			StandardTags:         standard,
		},
	}
}

func (c *WorkloadMetaCollector) handleKubePod(ev workloadmeta.Event) []*TagInfo {
	pod := ev.Entity.(*workloadmeta.KubernetesPod)

	tags := utils.NewTagList()
	c.extractTagsFromPod(pod, tags)

	// static tags for EKS Fargate pods
	for tag, value := range c.staticTags {
		tags.AddLow(tag, value)
	}

	low, orch, high, standard := tags.Compute()
	tagInfos := []*TagInfo{
		{
			Entity:               buildTaggerEntityID(pod.EntityID),
			HighCardTags:         high,
			OrchestratorCardTags: orch,
			LowCardTags:          low,
			StandardTags:         standard,
		},
	}

	return tagInfos
}

func (c *WorkloadMetaCollector) handleECSTask(ev workloadmeta.Event) []*TagInfo {
	var tagInfos []*TagInfo

	task := ev.Entity.(*workloadmeta.ECSTask)

	if task.LaunchType == workloadmeta.ECSLaunchTypeFargate {
		tags := utils.NewTagList()

		tags.AddOrchestrator("task_arn", task.ID)

		low, orch, high, standard := tags.Compute()
		tagInfos = append(tagInfos, &TagInfo{
			Entity:               OrchestratorScopeEntityID,
			HighCardTags:         high,
			OrchestratorCardTags: orch,
			LowCardTags:          low,
			StandardTags:         standard,
		})
	}

	return tagInfos
}

func (c *WorkloadMetaCollector) handleGardenContainer(ev workloadmeta.Event) []*TagInfo {
	container := ev.Entity.(*workloadmeta.GardenContainer)

	return []*TagInfo{
		{
			Entity:       buildTaggerEntityID(container.EntityID),
			HighCardTags: container.Tags,
		},
	}
}

func (c *WorkloadMetaCollector) extractTagsFromPod(pod *workloadmeta.KubernetesPod, tags *utils.TagList) {
	tags.AddOrchestrator(kubernetes.PodTagName, pod.Name)
	tags.AddLow(kubernetes.NamespaceTagName, pod.Namespace)
	tags.AddLow("pod_phase", strings.ToLower(pod.Phase))
	tags.AddLow("kube_priority_class", pod.PriorityClass)

	c.extractTagsFromPodLabels(pod, tags)

	for name, value := range pod.Annotations {
		utils.AddMetadataAsTags(name, value, c.annotationsAsTags, c.globAnnotations, tags)
	}

	for name, value := range pod.NamespaceLabels {
		utils.AddMetadataAsTags(name, value, c.nsLabelsAsTags, c.globNsLabels, tags)
	}

	for _, svc := range pod.KubeServices {
		tags.AddLow("kube_service", svc)
	}

	c.extractTagsFromJSONInMap(podTagsAnnotation, pod.Annotations, tags)

	// OpenShift pod annotations
	if dcName, found := pod.Annotations["openshift.io/deployment-config.name"]; found {
		tags.AddLow("oshift_deployment_config", dcName)
	}
	if deployName, found := pod.Annotations["openshift.io/deployment.name"]; found {
		tags.AddOrchestrator("oshift_deployment", deployName)
	}

	for _, owner := range pod.Owners {
		tags.AddLow(kubernetes.OwnerRefKindTagName, strings.ToLower(owner.Kind))
		tags.AddOrchestrator(kubernetes.OwnerRefNameTagName, owner.Name)

		c.extractTagsFromPodOwner(pod, owner, tags)
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
		cronjob := kubernetes.ParseCronJobForJob(owner.Name)
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

	case "":

	default:
		log.Debugf("unknown owner kind %q for pod %q", owner.Kind, pod.Name)
	}
}

func (c *WorkloadMetaCollector) extractTagsFromTaskContainer(task *workloadmeta.ECSTask, container *workloadmeta.Container, tags *utils.TagList) error {
	var taskContainer *workloadmeta.OrchestratorContainer

	for _, c := range task.Containers {
		if c.ID == container.ID {
			taskContainer = &c
			break
		}
	}

	if taskContainer == nil {
		return fmt.Errorf("task does not have expected reference to container")
	}

	// this method should not use any information from the container, and
	// should get it from the taskContainer instead. to prevent mistakes,
	// we nil the variable here
	container = nil

	// as of Agent 7.33, tasks have a name internally, but before that
	// task_name already was task.Family, so we keep it for backwards
	// compatibility
	tags.AddLow("task_name", task.Family)
	tags.AddLow("task_family", task.Family)
	tags.AddLow("task_version", task.Version)
	tags.AddOrchestrator("task_arn", task.ID)

	tags.AddLow("region", task.Region)
	tags.AddLow("availability_zone", task.AvailabilityZone)

	tags.AddLow("ecs_container_name", taskContainer.Name)

	if task.ClusterName != "" {
		if !config.Datadog.GetBool("disable_cluster_name_tag_key") {
			tags.AddLow("cluster_name", task.ClusterName)
		}
		tags.AddLow("ecs_cluster_name", task.ClusterName)
	}

	if c.collectEC2ResourceTags {
		addResourceTags(tags, task.ContainerInstanceTags)
		addResourceTags(tags, task.Tags)
	}

	return nil
}

func (c *WorkloadMetaCollector) extractTagsFromPodContainer(pod *workloadmeta.KubernetesPod, container *workloadmeta.Container, tags *utils.TagList) error {
	var podContainer *workloadmeta.OrchestratorContainer

	for _, c := range pod.Containers {
		if c.ID == container.ID {
			podContainer = &c
			break
		}
	}

	if podContainer == nil {
		return fmt.Errorf("pod does not have expected reference to container")
	}

	c.extractTagsFromPod(pod, tags)

	// this method should not use any information from the container, and
	// should get it from the podContainer instead. to prevent mistakes, we
	// nil the variable here
	container = nil

	containerName := podContainer.Name
	tags.AddLow("kube_container_name", containerName)
	tags.AddHigh("display_container_name", fmt.Sprintf("%s_%s", containerName, pod.Name))

	image := podContainer.Image
	tags.AddLow("image_name", image.Name)
	tags.AddLow("short_image", image.ShortName)
	tags.AddLow("image_tag", image.Tag)
	tags.AddLow("image_id", image.ID)

	// enrich with standard tags from labels for this container if present
	standardTagKeys := map[string]string{
		fmt.Sprintf(podStandardLabelPrefix+"%s.%s", containerName, tagKeyEnv):     tagKeyEnv,
		fmt.Sprintf(podStandardLabelPrefix+"%s.%s", containerName, tagKeyVersion): tagKeyVersion,
		fmt.Sprintf(podStandardLabelPrefix+"%s.%s", containerName, tagKeyService): tagKeyService,
	}
	c.extractFromMapWithFn(pod.Labels, standardTagKeys, tags.AddStandard)

	// container-specific tags provided through pod annotation
	annotation := fmt.Sprintf(podContainerTagsAnnotationFormat, containerName)
	c.extractTagsFromJSONInMap(annotation, pod.Annotations, tags)

	return nil
}

func (c *WorkloadMetaCollector) handleDelete(ev workloadmeta.Event) []*TagInfo {
	entityID := ev.Entity.GetID()

	return []*TagInfo{
		{
			Entity:       buildTaggerEntityID(entityID),
			DeleteEntity: true,
		},
	}
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
	case workloadmeta.KindContainer, workloadmeta.KindGardenContainer:
		return containers.BuildTaggerEntityName(entityID.ID)
	case workloadmeta.KindKubernetesPod:
		return kubelet.PodUIDToTaggerEntityName(entityID.ID)
	case workloadmeta.KindECSTask:
		return fmt.Sprintf("ecs_task://%s", entityID.ID)
	default:
		log.Errorf("can't recognize entity %q with kind %q; trying %s://%s as tagger entity",
			entityID.ID, entityID.Kind, entityID.ID, entityID.Kind)
		return fmt.Sprintf("%s://%s", string(entityID.Kind), entityID.ID)
	}
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
