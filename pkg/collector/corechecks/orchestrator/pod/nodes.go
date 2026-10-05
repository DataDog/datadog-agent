// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build orchestrator && kubeapiserver

package pod

import (
	"context"
	"errors"
	"slices"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/client-go/kubernetes"

	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/cluster"
	"github.com/DataDog/datadog-agent/pkg/orchestrator"
	"github.com/DataDog/datadog-agent/pkg/util/flavor"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes/apiserver"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// getKubeClient returns the API server client, tests replace it.
var getKubeClient = func() (kubernetes.Interface, error) {
	apiClient, err := apiserver.GetAPIClient()
	if err != nil {
		return nil, err
	}
	return apiClient.Cl, nil
}

// runForSelectedNodes reports the pods of the nodes selected by node_selector, e.g. nodes without a node agent.
// The pods of each node are sent as if the node agent of that node had sent them:
// with the node as host, as the backend matches nodes and pods to hosts by "<node name>-<cluster name>".
// As no host metadata exists for that host, the payloads also carry the cluster name tags.
//
// The check runs in the Cluster Agent, where only the leader reports, or as a cluster check.
func (c *Check) runForSelectedNodes() error {
	if flavor.GetFlavor() == flavor.ClusterAgent {
		leader, err := cluster.RunLeaderElection()
		if err != nil {
			if errors.Is(err, apiserver.ErrNotLeader) {
				log.Debugf("Not leader (leader is %q). Skipping the orchestrator_pod check", leader)
				return nil
			}
			return err
		}
	}

	podsByNode, err := listSelectedNodePods(context.TODO(), c.nodeSelector)
	if err != nil {
		return err
	}

	nodeNames := make([]string, 0, len(podsByNode))
	for nodeName := range podsByNode {
		nodeNames = append(nodeNames, nodeName)
	}
	slices.Sort(nodeNames)

	var totalListed, totalProcessed int
	for _, nodeName := range nodeNames {
		hostName := nodeName
		if c.clusterName != "" {
			hostName = nodeName + "-" + c.clusterName
		}

		// System info describes the host running the check, not the selected node.
		listed, processed := c.processPods(podsByNode[nodeName], hostName, nil, c.clusterNameTags)
		if processed == -1 {
			return errors.New("unable to process pods: a panic occurred")
		}
		totalListed += listed
		totalProcessed += processed
	}

	orchestrator.SetCacheStats(totalListed, totalProcessed, orchestrator.K8sPod)

	return nil
}

// listSelectedNodePods returns the pods of the nodes matching the selector, by node name.
// Lists are served from the API server cache.
func listSelectedNodePods(ctx context.Context, selector labels.Selector) (map[string][]*corev1.Pod, error) {
	client, err := getKubeClient()
	if err != nil {
		return nil, err
	}

	nodes, err := client.CoreV1().Nodes().List(ctx, metav1.ListOptions{LabelSelector: selector.String(), ResourceVersion: "0"})
	if err != nil {
		return nil, err
	}

	podsByNode := make(map[string][]*corev1.Pod, len(nodes.Items))
	for _, node := range nodes.Items {
		pods, err := client.CoreV1().Pods(metav1.NamespaceAll).List(ctx, metav1.ListOptions{
			FieldSelector:   fields.OneTermEqualSelector("spec.nodeName", node.Name).String(),
			ResourceVersion: "0",
		})
		if err != nil {
			return nil, err
		}

		nodePods := make([]*corev1.Pod, 0, len(pods.Items))
		for i := range pods.Items {
			if pods.Items[i].Spec.NodeName == node.Name {
				nodePods = append(nodePods, &pods.Items[i])
			}
		}
		podsByNode[node.Name] = nodePods
	}

	return podsByNode, nil
}
