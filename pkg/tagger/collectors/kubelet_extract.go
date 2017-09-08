// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2017 Datadog, Inc.

package collectors

import (
	//"fmt"
	"regexp"
	//log "github.com/cihub/seelog"

	"github.com/DataDog/datadog-agent/pkg/util/kubernetes"
)

// KubeReplicaSetRegexp allows to extract parent deployment name from replicaset
var KubeReplicaSetRegexp = regexp.MustCompile("^(\\S+)-[0-9]+$")

// parsePods convert Pods from the PodWatcher to TagInfo objects
func (c *KubeletCollector) parsePods(pods []*kubernetes.Pod) ([]*TagInfo, error) {
	var output []*TagInfo
	for _, pod := range pods {
		for _, container := range pod.Status.Containers {
			tags := newTagList()

			// Pod name
			tags.addHigh("pod_name", pod.Metadata.Name)
			tags.addLow("kube_namespace", pod.Metadata.Namespace)
			tags.addLow("kube_container_name", container.Name)

			// TODO labels with configurable list

			// Creator
			for _, owner := range pod.Metadata.Owners {
				if len(owner.Name) < 1 {
					continue
				}
				switch owner.Kind {
				case "Deployment":
					tags.addLow("kube_deployment", owner.Name)
				case "DaemonSet":
					tags.addLow("kube_daemon_set", owner.Name)
				case "ReplicationController":
					tags.addLow("kube_replication_controller", owner.Name)
				case "StatefulSet":
					tags.addLow("kube_stateful_set", owner.Name)
				case "Job":
					tags.addHigh("kube_job", owner.Name) // TODO detect if no from cronjob, then low card
				case "ReplicaSet":
					deployment := c.parseDeploymentForReplicaset(owner.Name)
					if len(deployment) > 0 {
						tags.addHigh("kube_replica_set", owner.Name)
						tags.addLow("kube_deployment", deployment)
					} else {
						tags.addLow("kube_replica_set", owner.Name)
					}
				}
			}

			low, high := tags.compute()
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

// parseDeploymentForReplicaset gets the deployment name fron a replicaset,
// or returns an empty string if no parent deployment is found.
func (c *KubeletCollector) parseDeploymentForReplicaset(name string) string {
	// TODO optionnaly query the cluster agent instead of assuming?
	match := KubeReplicaSetRegexp.FindStringSubmatch(name)
	if len(match) < 2 {
		return ""
	}
	return match[1]
}
