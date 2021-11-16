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

	tags := utils.NewTagList()
	tags.AddHigh("container_name", container.Name)
	tags.AddHigh("container_id", container.ID)

	image := container.Image
	tags.AddLow("image_name", image.Name)
	tags.AddLow("short_image", image.ShortName)
	tags.AddLow("image_tag", image.Tag)

	if container.Runtime == workloadmeta.ContainerRuntimeDocker {
		tags.AddLow("docker_image", fmt.Sprintf("%s:%s", image.Name, image.Tag))
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

	// static tags for ECS Fargate
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

func (c *WorkloadMetaCollector) handleKubePod(ev workloadmeta.Event) []*TagInfo {
	pod := ev.Entity.(*workloadmeta.KubernetesPod)

	tags := utils.NewTagList()
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

func (c *WorkloadMetaCollector) handleECSTask(ev workloadmeta.Event) []*TagInfo {
	task := ev.Entity.(*workloadmeta.ECSTask)

	tagInfos := make([]*TagInfo, 0, len(task.Containers))
	for _, taskContainer := range task.Containers {
		container, err := c.store.GetContainer(taskContainer.ID)
		if err != nil {
			log.Debugf("task %q has reference to non-existing container %q", task.Name, taskContainer.ID)
			continue
		}

		c.registerChild(task.EntityID, container.EntityID)

		tags := utils.NewTagList()

		// as of Agent 7.33, tasks have a name internally, but before that
		// task_name already was task.Family, so we keep it for backwards
		// compatibility
		tags.AddLow("task_name", task.Family)
		tags.AddLow("task_family", task.Family)
		tags.AddLow("task_version", task.Version)
		tags.AddOrchestrator("task_arn", task.ID)

		tags.AddLow("ecs_container_name", taskContainer.Name)

		if task.ClusterName != "" {
			if !config.Datadog.GetBool("disable_cluster_name_tag_key") {
				tags.AddLow("cluster_name", task.ClusterName)
			}
			tags.AddLow("ecs_cluster_name", task.ClusterName)
		}

		if task.LaunchType == workloadmeta.ECSLaunchTypeFargate {
			tags.AddLow("region", task.Region)
			tags.AddLow("availability_zone", task.AvailabilityZone)
		} else if c.collectEC2ResourceTags {
			addResourceTags(tags, task.ContainerInstanceTags)
			addResourceTags(tags, task.Tags)
		}

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
		tags := utils.NewTagList()

		tags.AddOrchestrator("task_arn", task.ID)

		low, orch, high, standard := tags.Compute()
		tagInfos = append(tagInfos, &TagInfo{
			Source:               taskSource,
			Entity:               OrchestratorScopeEntityID,
			HighCardTags:         high,
			OrchestratorCardTags: orch,
			LowCardTags:          low,
			StandardTags:         standard,
		})
	}

	return tagInfos
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

	source := fmt.Sprintf("%s-%s", workloadmetaCollectorName, string(entityID.Kind))
	tagInfos := make([]*TagInfo, 0, len(children)+1)
	tagInfos = append(tagInfos, &TagInfo{
		Source:       source,
		Entity:       taggerEntityID,
		DeleteEntity: true,
	})

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
	case workloadmeta.KindECSTask:
		return fmt.Sprintf("ecs_task://%s", entityID.ID)
	default:
		log.Errorf("can't recognize entity %q with kind %q, but building a a tagger ID anyway", entityID.ID, entityID.Kind)
		return containers.BuildEntityName(string(entityID.Kind), entityID.ID)
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
