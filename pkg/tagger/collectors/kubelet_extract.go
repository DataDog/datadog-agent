// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// +build kubelet

package collectors

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/DataDog/datadog-agent/pkg/util/kubernetes"
	"github.com/DataDog/datadog-agent/pkg/util/log"

	"github.com/DataDog/datadog-agent/pkg/tagger/utils"
	"github.com/DataDog/datadog-agent/pkg/util/containers"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes/kubelet"

	"k8s.io/kubernetes/third_party/forked/golang/expansion"
)

const (
	podAnnotationPrefix              = "ad.datadoghq.com/"
	podContainerTagsAnnotationFormat = podAnnotationPrefix + "%s.tags"
	podTagsAnnotation                = podAnnotationPrefix + "tags"
	podStandardLabelPrefix           = "tags.datadoghq.com/"
)

// parsePods convert Pods from the PodWatcher to TagInfo objects
func (c *KubeletCollector) parsePods(pods []*kubelet.Pod) ([]*TagInfo, error) {
	var output []*TagInfo
	for _, pod := range pods {
		// pod tags
		tags := utils.NewTagList()

		// Pod name
		tags.AddOrchestrator(kubernetes.PodTagName, pod.Metadata.Name)
		tags.AddLow(kubernetes.NamespaceTagName, pod.Metadata.Namespace)

		// Pod labels
		for name, value := range pod.Metadata.Labels {
			// Standard pod labels
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

			// Pod labels as tags
			utils.AddMetadataAsTags(name, value, c.labelsAsTags, c.globLabels, tags)
		}

		// Pod annotations as tags
		for name, value := range pod.Metadata.Annotations {
			utils.AddMetadataAsTags(name, value, c.annotationsAsTags, c.globAnnotations, tags)
		}

		if podTags, found := extractTagsFromMap(podTagsAnnotation, pod.Metadata.Annotations); found {
			for tagName, values := range podTags {
				for _, val := range values {
					tags.AddAuto(tagName, val)
				}
			}
		}

		// Pod phase
		tags.AddLow("pod_phase", strings.ToLower(pod.Status.Phase))

		// OpenShift pod annotations
		if dcName, found := pod.Metadata.Annotations["openshift.io/deployment-config.name"]; found {
			tags.AddLow("oshift_deployment_config", dcName)
		}
		if deployName, found := pod.Metadata.Annotations["openshift.io/deployment.name"]; found {
			tags.AddOrchestrator("oshift_deployment", deployName)
		}

		// Creator
		for _, owner := range pod.Owners() {
			tags.AddLow(kubernetes.OwnerRefKindTagName, strings.ToLower(owner.Kind))
			tags.AddOrchestrator(kubernetes.OwnerRefNameTagName, owner.Name)

			switch owner.Kind {
			case "":
				continue
			case kubernetes.DeploymentKind:
				tags.AddLow(kubernetes.DeploymentTagName, owner.Name)
			case kubernetes.DaemonSetKind:
				tags.AddLow(kubernetes.DaemonSetTagName, owner.Name)
			case kubernetes.ReplicationControllerKind:
				tags.AddLow(kubernetes.ReplicationControllerTagName, owner.Name)
			case kubernetes.StatefulSetKind:
				tags.AddLow(kubernetes.StatefulSetTagName, owner.Name)
				pvcs := pod.GetPersistentVolumeClaimNames()
				for _, pvc := range pvcs {
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
			default:
				log.Debugf("unknown owner kind %s for pod %s", owner.Kind, pod.Metadata.Name)
			}
		}

		low, orch, high, standard := tags.Compute()
		if pod.Metadata.UID != "" {
			podInfo := &TagInfo{
				Source:               kubeletCollectorName,
				Entity:               kubelet.PodUIDToTaggerEntityName(pod.Metadata.UID),
				HighCardTags:         high,
				OrchestratorCardTags: orch,
				LowCardTags:          low,
				StandardTags:         standard,
			}
			output = append(output, podInfo)
		}

		// container tags
		for _, container := range pod.Status.GetAllContainers() {
			if container.ID == "" {
				log.Debugf("Cannot collect container tags for container '%s' in pod '%s': Empty container ID", container.Name, pod.Metadata.Name)
				// This can happen due to kubelet latency
				// Ignore the container as early as possible to avoid unnecessary processing and warn logging
				// The container will be detected once again when container.ID is set
				continue
			}
			cTags := tags.Copy()
			cTags.AddLow("kube_container_name", container.Name)
			cTags.AddHigh("container_id", kubelet.TrimRuntimeFromCID(container.ID))
			if container.Name != "" && pod.Metadata.Name != "" {
				cTags.AddHigh("display_container_name", fmt.Sprintf("%s_%s", container.Name, pod.Metadata.Name))
			}

			// Enrich with standard tags from labels for this container if present
			labelTags := []string{
				tagKeyEnv,
				tagKeyVersion,
				tagKeyService,
			}

			for _, tag := range labelTags {
				label := fmt.Sprintf(podStandardLabelPrefix+"%s.%s", container.Name, tag)
				if value, ok := pod.Metadata.Labels[label]; ok {
					cTags.AddStandard(tag, value)
				}
			}

			// container-specific tags provided through pod annotation
			containerTags, found := extractTagsFromMap(
				fmt.Sprintf(podContainerTagsAnnotationFormat, container.Name),
				pod.Metadata.Annotations,
			)
			if found {
				for tagName, values := range containerTags {
					for _, val := range values {
						cTags.AddAuto(tagName, val)
					}
				}
			}

			// check env vars and image tag in spec
			// TODO: Implement support of environment variables set from ConfigMap, Secret, DownwardAPI.
			// See https://github.com/kubernetes/kubernetes/blob/d20fd4088476ec39c5ae2151b8fffaf0f4834418/pkg/kubelet/kubelet_pods.go#L566
			// for the complete environment variable resolution process that is done by the kubelet.
			for _, containerSpec := range pod.Spec.Containers {
				if containerSpec.Name == container.Name {
					tmpEnv := make(map[string]string)
					mappingFunc := expansion.MappingFuncFor(tmpEnv)

					for _, env := range containerSpec.Env {
						if env.Value != "" {
							runtimeVal := expansion.Expand(env.Value, mappingFunc)
							tmpEnv[env.Name] = runtimeVal

							switch env.Name {
							case envVarEnv:
								cTags.AddStandard(tagKeyEnv, runtimeVal)
							case envVarVersion:
								cTags.AddStandard(tagKeyVersion, runtimeVal)
							case envVarService:
								cTags.AddStandard(tagKeyService, runtimeVal)
							}
						} else if env.Name == envVarEnv || env.Name == envVarVersion || env.Name == envVarService {
							log.Warnf("Reading %s from a ConfigMap, Secret or anything but a literal value is not implemented yet.", env.Name)
						}
					}
					imageName, shortImage, imageTag, err := containers.SplitImageName(containerSpec.Image)
					if err != nil {
						log.Debugf("Cannot split %s: %s", containerSpec.Image, err)
						break
					}
					cTags.AddLow("image_name", imageName)
					cTags.AddLow("short_image", shortImage)
					if imageTag == "" {
						// k8s default to latest if tag is omitted
						imageTag = "latest"
					}
					cTags.AddLow("image_tag", imageTag)
					break
				}
			}

			cLow, cOrch, cHigh, standard := cTags.Compute()
			entityID, err := kubelet.KubeContainerIDToTaggerEntityID(container.ID)
			if err != nil {
				log.Warnf("Unable to parse container pName: %s / cName: %s / cId: %s / err: %s", pod.Metadata.Name, container.Name, container.ID, err)
				continue
			}
			info := &TagInfo{
				Source:               kubeletCollectorName,
				Entity:               entityID,
				HighCardTags:         cHigh,
				OrchestratorCardTags: cOrch,
				LowCardTags:          cLow,
				StandardTags:         standard,
			}
			output = append(output, info)
		}
	}
	return output, nil
}

// extractTagsFromMap extracts tags contained in a JSON string stored at the
// given key. If no valid tag definition is found at this key, it will return
// false. Otherwise it returns a map containing extracted tags.
// The map values are string slices to support tag keys with multiple values.
func extractTagsFromMap(key string, input map[string]string) (map[string][]string, bool) {
	jsonTags, found := input[key]
	if !found {
		return nil, false
	}

	tags, err := parseJSONValue(jsonTags)
	if err != nil {
		log.Errorf("can't parse value for annotation %s: %s", key, err)
		return nil, false
	}

	return tags, true
}

// parseJSONValue returns a map from the given JSON string.
func parseJSONValue(value string) (map[string][]string, error) {
	if value == "" {
		return nil, errors.New("value is empty")
	}

	result := map[string]interface{}{}
	if err := json.Unmarshal([]byte(value), &result); err != nil {
		return nil, fmt.Errorf("failed to unmarshal JSON: %s", err)
	}

	tags := map[string][]string{}
	for key, value := range result {
		switch v := value.(type) {
		case string:
			tags[key] = append(tags[key], v)
		case []interface{}:
			for _, tag := range v {
				tags[key] = append(tags[key], fmt.Sprint(tag))
			}
		default:
			log.Debugf("Tag value %s is not valid, must be a string or an array, skipping", v)
		}
	}

	return tags, nil
}
