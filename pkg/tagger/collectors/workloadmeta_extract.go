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

	"github.com/DataDog/datadog-agent/pkg/tagger/utils"
	"github.com/DataDog/datadog-agent/pkg/util/containers"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes/kubelet"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/datadog-agent/pkg/workloadmeta"
)

const (
	podAnnotationPrefix              = "ad.datadoghq.com/"
	podContainerTagsAnnotationFormat = podAnnotationPrefix + "%s.tags"
	podTagsAnnotation                = podAnnotationPrefix + "tags"
	podStandardLabelPrefix           = "tags.datadoghq.com/"
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
	}

	highCardOrchestratorLabels = map[string]string{
		"io.rancher.container.name": "rancher_container",
	}
)

func (c *WorkloadMetaCollector) processEvents(evBundle workloadmeta.EventBundle) {
	tagInfos := []*TagInfo{}

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
				// TODO
			default:
				log.Errorf("cannot handle event for entity %q with kind %q", entityID.ID, entityID.Kind)
			}

		case workloadmeta.EventTypeUnset:
			tagInfos = append(tagInfos, &TagInfo{
				Source:       fmt.Sprintf("%s-%s", workloadmetaCollectorName, string(entityID.Kind)),
				Entity:       buildTaggerEntityID(entityID),
				DeleteEntity: true,
			})

		default:
			log.Errorf("cannot handle event of type %d", ev.Type)
		}

	}

	// NOTE: haha, this is still async and race conditions will still
	// happen :D since the workloadmeta will be the only collector in the
	// tagger in the end, this can be turned into a sync call to
	// processTagInfo
	c.out <- tagInfos

	close(evBundle.Ch)
}

func (c *WorkloadMetaCollector) handleContainer(ev workloadmeta.Event) []*TagInfo {
	tagInfos := []*TagInfo{}
	tags := utils.NewTagList()

	container := ev.Entity.(*workloadmeta.Container)

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
	c.extractFromMapNormalizedWithFn(container.EnvVars, c.containerEnvAsTags, tags.AddAuto)

	low, orch, high, standard := tags.Compute()
	tagInfos = append(tagInfos, &TagInfo{
		Source:               containerSource,
		Entity:               buildTaggerEntityID(container.EntityID),
		HighCardTags:         high,
		OrchestratorCardTags: orch,
		LowCardTags:          low,
		StandardTags:         standard,
	})

	return tagInfos
}

func (c *WorkloadMetaCollector) handleKubePod(ev workloadmeta.Event) []*TagInfo {
	tagInfos := []*TagInfo{}
	tags := utils.NewTagList()

	pod := ev.Entity.(*workloadmeta.KubernetesPod)

	tags.AddOrchestrator(kubernetes.PodTagName, pod.Name)
	tags.AddLow(kubernetes.NamespaceTagName, pod.Namespace)
	tags.AddLow("pod_phase", strings.ToLower(pod.Phase))
	tags.AddLow("kube_priority_class", pod.PriorityClass)

	c.extractTagsFromPodLabels(pod, tags)

	for name, value := range pod.Annotations {
		utils.AddMetadataAsTags(name, value, c.annotationsAsTags, c.globAnnotations, tags)
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
	tagInfos = append(tagInfos, &TagInfo{
		Source:               podSource,
		Entity:               buildTaggerEntityID(pod.EntityID),
		HighCardTags:         high,
		OrchestratorCardTags: orch,
		LowCardTags:          low,
		StandardTags:         standard,
	})

	for _, containerID := range pod.Containers {
		container, err := c.store.GetContainer(containerID)
		if err != nil {
			log.Debugf("pod %q has reference to non-existing container %q", pod.Name, containerID)
			continue
		}

		cTags := tags.Copy()
		c.extractTagsFromPodContainer(pod, container, cTags)

		low, orch, high, standard := cTags.Compute()
		tagInfos = append(tagInfos, &TagInfo{
			// podSource here is not a mistake. the source is
			// always from the parent resource.
			Source:               podSource,
			Entity:               buildTaggerEntityID(container.EntityID),
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
				tags.AddLow("persistentvolumeclaim", pvc)
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

func (c *WorkloadMetaCollector) extractTagsFromPodContainer(pod *workloadmeta.KubernetesPod, container *workloadmeta.Container, tags *utils.TagList) {
	tags.AddLow("kube_container_name", container.Name)
	tags.AddHigh("container_id", container.ID)

	if container.Name != "" && pod.Name != "" {
		tags.AddHigh("display_container_name", fmt.Sprintf("%s_%s", container.Name, pod.Name))
	}

	image := container.Image
	tags.AddLow("image_name", image.Name)
	tags.AddLow("short_image", image.ShortName)
	tags.AddLow("image_tag", image.Tag)
	tags.AddLow("image_id", image.ID)

	// enrich with standard tags from labels for this container if present
	standardTagKeys := map[string]string{
		fmt.Sprintf(podStandardLabelPrefix+"%s.%s", container.Name, tagKeyEnv):     tagKeyEnv,
		fmt.Sprintf(podStandardLabelPrefix+"%s.%s", container.Name, tagKeyVersion): tagKeyVersion,
		fmt.Sprintf(podStandardLabelPrefix+"%s.%s", container.Name, tagKeyService): tagKeyService,
	}
	c.extractFromMapWithFn(pod.Labels, standardTagKeys, tags.AddStandard)

	// enrich with standard tags from environment variables
	c.extractFromMapWithFn(container.EnvVars, standardEnvKeys, tags.AddStandard)

	// container-specific tags provided through pod annotation
	annotation := fmt.Sprintf(podContainerTagsAnnotationFormat, container.Name)
	c.extractTagsFromJSONInMap(annotation, pod.Annotations, tags)
}

func buildTaggerEntityID(entityID workloadmeta.EntityID) string {
	switch entityID.Kind {
	case workloadmeta.KindContainer:
		return containers.BuildTaggerEntityName(entityID.ID)
	case workloadmeta.KindKubernetesPod:
		return kubelet.PodUIDToTaggerEntityName(entityID.ID)
	default:
		log.Errorf("can't recognize entity %q with kind %q, but building a a tagger ID anyway", entityID.ID, entityID.Kind)
		return containers.BuildEntityName(string(entityID.Kind), entityID.ID)
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
