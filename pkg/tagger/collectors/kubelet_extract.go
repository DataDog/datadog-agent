// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2019 Datadog, Inc.

// +build kubelet

package collectors

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/DataDog/datadog-agent/pkg/util/log"

	"github.com/DataDog/datadog-agent/pkg/tagger/utils"
	"github.com/DataDog/datadog-agent/pkg/util/containers"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes/kubelet"
)

const (
	podAnnotationPrefix              = "ad.datadoghq.com/"
	podContainerTagsAnnotationFormat = podAnnotationPrefix + "%s.tags"
	podTagsAnnotation                = podAnnotationPrefix + "tags"
)

// KubeAllowedEncodeStringAlphaNums holds the charactes allowed in replicaset names from as parent deployment
// Taken from https://github.com/kow3ns/kubernetes/blob/96067e6d7b24a05a6a68a0d94db622957448b5ab/staging/src/k8s.io/apimachinery/pkg/util/rand/rand.go#L76
const KubeAllowedEncodeStringAlphaNums = "bcdfghjklmnpqrstvwxz2456789"

// Digits holds the digits used for naming replicasets in kubenetes < 1.8
const Digits = "1234567890"

// parsePods convert Pods from the PodWatcher to TagInfo objects
func (c *KubeletCollector) parsePods(pods []*kubelet.Pod) ([]*TagInfo, error) {
	var output []*TagInfo
	for _, pod := range pods {
		// pod tags
		tags := utils.NewTagList()

		// Pod name
		tags.AddOrchestrator("pod_name", pod.Metadata.Name)
		tags.AddLow("kube_namespace", pod.Metadata.Namespace)

		// Pod labels
		for name, value := range pod.Metadata.Labels {
			for pattern, tmpl := range c.labelsAsTags {
				if ok, _ := filepath.Match(pattern, strings.ToLower(name)); ok {
					tags.AddAuto(resolveTag(tmpl, name), value)
				}
			}
		}

		// Pod annotations
		for name, value := range pod.Metadata.Annotations {
			if tagName, found := c.annotationsAsTags[strings.ToLower(name)]; found {
				tags.AddAuto(tagName, value)
			}
		}
		if podTags, found := extractTagsFromMap(podTagsAnnotation, pod.Metadata.Annotations); found {
			for tagName, value := range podTags {
				tags.AddAuto(tagName, value)
			}
		}

		// Pod phase
		tags.AddLow("pod_phase", strings.ToLower(pod.Status.Phase))

		// OpenShift pod annotations
		if dc_name, found := pod.Metadata.Annotations["openshift.io/deployment-config.name"]; found {
			tags.AddLow("oshift_deployment_config", dc_name)
		}
		if deploy_name, found := pod.Metadata.Annotations["openshift.io/deployment.name"]; found {
			tags.AddOrchestrator("oshift_deployment", deploy_name)
		}

		// Creator
		for _, owner := range pod.Owners() {
			switch owner.Kind {
			case "":
				continue
			case "Deployment":
				tags.AddLow("kube_deployment", owner.Name)
			case "DaemonSet":
				tags.AddLow("kube_daemon_set", owner.Name)
			case "ReplicationController":
				tags.AddLow("kube_replication_controller", owner.Name)
			case "StatefulSet":
				tags.AddLow("kube_stateful_set", owner.Name)
			case "Job":
				tags.AddOrchestrator("kube_job", owner.Name) // TODO detect if no from cronjob, then low card
			case "ReplicaSet":
				deployment := c.parseDeploymentForReplicaset(owner.Name)
				if len(deployment) > 0 {
					tags.AddOrchestrator("kube_replica_set", owner.Name)
					tags.AddLow("kube_deployment", deployment)
				} else {
					tags.AddLow("kube_replica_set", owner.Name)
				}
			default:
				log.Debugf("unknown owner kind %s for pod %s", owner.Kind, pod.Metadata.Name)
			}
		}

		low, orch, high := tags.Compute()
		if pod.Metadata.UID != "" {
			podInfo := &TagInfo{
				Source:               kubeletCollectorName,
				Entity:               kubelet.PodUIDToEntityName(pod.Metadata.UID),
				HighCardTags:         high,
				OrchestratorCardTags: orch,
				LowCardTags:          low,
			}
			output = append(output, podInfo)
		}

		// container tags
		for _, container := range pod.Status.GetAllContainers() {
			cTags := tags.Copy()
			cTags.AddLow("kube_container_name", container.Name)
			cTags.AddHigh("container_id", kubelet.TrimRuntimeFromCID(container.ID))
			if container.Name != "" && pod.Metadata.Name != "" {
				cTags.AddHigh("display_container_name", fmt.Sprintf("%s_%s", container.Name, pod.Metadata.Name))
			}

			// check image tag in spec
			for _, containerSpec := range pod.Spec.Containers {
				if containerSpec.Name == container.Name {
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

			// container-specific tags provided through pod annotation
			containerTags, found := extractTagsFromMap(
				fmt.Sprintf(podContainerTagsAnnotationFormat, container.Name),
				pod.Metadata.Annotations,
			)
			if found {
				for tagName, value := range containerTags {
					cTags.AddAuto(tagName, value)
				}
			}

			cLow, cOrch, cHigh := cTags.Compute()
			info := &TagInfo{
				Source:               kubeletCollectorName,
				Entity:               container.ID,
				HighCardTags:         cHigh,
				OrchestratorCardTags: cOrch,
				LowCardTags:          cLow,
			}
			output = append(output, info)
		}
	}
	return output, nil
}

// parseDeploymentForReplicaset gets the deployment name from a replicaset,
// or returns an empty string if no parent deployment is found.
func (c *KubeletCollector) parseDeploymentForReplicaset(name string) string {
	lastDash := strings.LastIndexAny(name, "-")
	if lastDash == -1 {
		// No dash
		return ""
	}
	suffix := name[lastDash+1:]
	if len(suffix) < 3 {
		// Suffix is variable length but we cutoff at 3+ characters
		return ""
	}

	if !utils.StringInRuneset(suffix, Digits) && !utils.StringInRuneset(suffix, KubeAllowedEncodeStringAlphaNums) {
		// Invalid suffix
		return ""
	}

	return name[:lastDash]
}

// extractTagsFromMap extracts tags contained in a JSON string stored at the
// given key. If no valid tag definition is found at this key, it will return
// false. Otherwise it returns a map containing extracted tags.
func extractTagsFromMap(key string, input map[string]string) (map[string]string, bool) {
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
func parseJSONValue(value string) (map[string]string, error) {
	if value == "" {
		return nil, fmt.Errorf("value is empty")
	}

	var result map[string]string

	err := json.Unmarshal([]byte(value), &result)
	if err != nil {
		return nil, fmt.Errorf("failed to unmarshal JSON: %s", err)
	}

	return result, nil
}
