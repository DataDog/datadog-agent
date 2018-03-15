// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018 Datadog, Inc.

// +build kubelet

package collectors

import (
	"fmt"
	"strings"

	log "github.com/cihub/seelog"

	"github.com/DataDog/datadog-agent/pkg/tagger/utils"
	"github.com/DataDog/datadog-agent/pkg/util/docker"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes/kubelet"
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
		tags.AddHigh("pod_name", pod.Metadata.Name)
		tags.AddLow("kube_namespace", pod.Metadata.Namespace)

		// Pod labels
		for name, value := range pod.Metadata.Labels {
			if tagName, found := c.labelsAsTags[strings.ToLower(name)]; found {
				tags.AddAuto(tagName, value)
			}
		}

		// Pod annotations
		for name, value := range pod.Metadata.Annotations {
			if tagName, found := c.annotationsAsTags[strings.ToLower(name)]; found {
				tags.AddAuto(tagName, value)
			}
		}

		// OpenShift pod annotations
		if dc_name, found := pod.Metadata.Annotations["openshift.io/deployment-config.name"]; found {
			tags.AddLow("oshift_deployment_config", dc_name)
		}
		if deploy_name, found := pod.Metadata.Annotations["openshift.io/deployment.name"]; found {
			tags.AddHigh("oshift_deployment", deploy_name)
		}

		// Creator
		for _, owner := range pod.Metadata.Owners {
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
				tags.AddHigh("kube_job", owner.Name) // TODO detect if no from cronjob, then low card
			case "ReplicaSet":
				deployment := c.parseDeploymentForReplicaset(owner.Name)
				if len(deployment) > 0 {
					tags.AddHigh("kube_replica_set", owner.Name)
					tags.AddLow("kube_deployment", deployment)
				} else {
					tags.AddLow("kube_replica_set", owner.Name)
				}
			default:
				log.Debugf("unknown owner kind %s for pod %s", owner.Kind, pod.Metadata.Name)
			}
		}

		low, high := tags.Compute()
		if pod.Metadata.UID != "" {
			podInfo := &TagInfo{
				Source:       kubeletCollectorName,
				Entity:       kubelet.PodUIDToEntityName(pod.Metadata.UID),
				HighCardTags: high,
				LowCardTags:  low,
			}
			output = append(output, podInfo)
		}

		// container tags
		for _, container := range pod.Status.Containers {
			lowC := append(low, fmt.Sprintf("kube_container_name:%s", container.Name))
			// check image tag in spec
			for _, containerSpec := range pod.Spec.Containers {
				if containerSpec.Name == container.Name {
					imageName, shortImage, imageTag, err := docker.SplitImageName(containerSpec.Image)
					if err != nil {
						log.Debugf("Cannot split %s: %s", containerSpec.Image, err)
						break
					}
					lowC = append(lowC, fmt.Sprintf("image_name:%s", imageName))
					lowC = append(lowC, fmt.Sprintf("short_image:%s", shortImage))
					if imageTag == "" {
						// k8s default to latest if tag is omitted
						imageTag = "latest"
					}
					lowC = append(lowC, fmt.Sprintf("image_tag:%s", imageTag))
					break
				}
			}

			info := &TagInfo{
				Source:       kubeletCollectorName,
				Entity:       container.ID,
				HighCardTags: high,
				LowCardTags:  lowC,
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
