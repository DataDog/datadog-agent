// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2017 Datadog, Inc.

package collectors

import (
	"fmt"
	log "github.com/cihub/seelog"
	"regexp"
	"strings"

	"github.com/DataDog/datadog-agent/pkg/tagger/utils"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes/kubelet"
)

/* Deltas from agent5:

   Moved to cluster agent:
     - kube_service
	 - node tags
   To deprecate:
     - kube_replicate_controller added everytime, just keep if real owner
     - pod_name does not include the namespace anymore, redundant with kube_namespace
     - removing the no_pod tags
     - container_alias tags
*/

// KubeReplicaSetRegexp allows to extract parent deployment name from replicaset
var KubeReplicaSetRegexp = regexp.MustCompile("^(\\S+)-[0-9]+$")

// parsePods convert Pods from the PodWatcher to TagInfo objects
func (c *KubeletCollector) parsePods(pods []*kubelet.Pod) ([]*TagInfo, error) {
	var output []*TagInfo
	for _, pod := range pods {
		for _, container := range pod.Status.Containers {
			tags := utils.NewTagList()

			// Pod name
			tags.AddHigh("pod_name", pod.Metadata.Name)
			tags.AddLow("kube_namespace", pod.Metadata.Namespace)
			tags.AddLow("kube_container_name", container.Name)

			// Pod labels
			for key, value := range pod.Metadata.Labels {
				var tagName string
				if c.labelTagPrefix == "" {
					tagName = key
				} else {
					tagName = fmt.Sprintf("%s%s", c.labelTagPrefix, key)
				}
				switch {
				case strings.HasSuffix(key, "-template-generation"):
					tags.AddHigh(tagName, value)
				case strings.HasSuffix(key, "-template-hash"):
					tags.AddHigh(tagName, value)
				default:
					tags.AddLow(tagName, value)
				}
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
			info := &TagInfo{
				Source:       kubeletCollectorName,
				Entity:       container.ID,
				HighCardTags: high,
				LowCardTags:  low,
			}
			output = append(output, info)
		}
	}
	return output, nil
}

// parseDeploymentForReplicaset gets the deployment name from a replicaset,
// or returns an empty string if no parent deployment is found.
func (c *KubeletCollector) parseDeploymentForReplicaset(name string) string {
	// TODO optionaly query the cluster agent instead of assuming?
	match := KubeReplicaSetRegexp.FindStringSubmatch(name)
	if len(match) < 2 {
		return ""
	}
	return match[1]
}
