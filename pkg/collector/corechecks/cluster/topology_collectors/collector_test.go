// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2019 Datadog, Inc.
// +build kubeapiserver

package topology_collectors

import (
	"github.com/StackVista/stackstate-agent/pkg/collector/corechecks/cluster/kubeapi"
	"github.com/StackVista/stackstate-agent/pkg/topology"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestExternalIDBuilders(t *testing.T) {

	instance := topology.Instance{Type: string(kubeapi.Kubernetes), URL: "Test-Cluster-Name"}
	clusterTopologyCollector := NewClusterTopologyCollector(instance, nil)

	clusterTopologyCollector.buildClusterExternalID()
	assert.Equal(t, "urn:cluster:%s/%s", "")

	podName := "test-pod-name"
	containerName := "test-container-name"
	actualContainerExternalID := clusterTopologyCollector.buildContainerExternalID(podName, containerName)
	assert.Equal(t, "urn:/kubernetes:Test-Cluster-Name:pod:test-pod-name:container:test-container-name", actualContainerExternalID)

	daemonSetName := "test-daemonset"
	actualDaemonSetExternalID := clusterTopologyCollector.buildDaemonSetExternalID(daemonSetName)
	assert.Equal(t, "urn:/kubernetes:Test-Cluster-Name:daemonset:test-daemonset", actualDaemonSetExternalID)

	deploymentName := "test-deployment"
	actualDeploymentExternalID := clusterTopologyCollector.buildDeploymentExternalID(deploymentName)
	assert.Equal(t, "urn:/kubernetes:Test-Cluster-Name:deployment:test-deployment", actualDeploymentExternalID)

	nodeName := "test-node"
	actualNodeExternalID := clusterTopologyCollector.buildNodeExternalID(nodeName)
	assert.Equal(t, "urn:/kubernetes:Test-Cluster-Name:node:test-node", actualNodeExternalID)

	actualPodExternalID := clusterTopologyCollector.buildPodExternalID(podName)
	assert.Equal(t, "urn:/kubernetes:Test-Cluster-Name:pod:test-pod-name", actualPodExternalID)

	replicaSetName := "test-replicaset"
	actualReplicaSetExternalID := clusterTopologyCollector.buildReplicaSetExternalID(replicaSetName)
	assert.Equal(t, "urn:/kubernetes:Test-Cluster-Name:replicaset:test-replicaset", actualReplicaSetExternalID)

	serviceID := "test-service"
	actualServiceExternalID := clusterTopologyCollector.buildServiceExternalID(serviceID)
	assert.Equal(t, "urn:/kubernetes:Test-Cluster-Name:service:test-service", actualServiceExternalID)

	statefulSetName := "test-statefulset"
	actualStatefulSetExternalID := clusterTopologyCollector.buildStatefulSetExternalID(statefulSetName)
	assert.Equal(t, "urn:/kubernetes:Test-Cluster-Name:statefulset:test-statefulset", actualStatefulSetExternalID)
}
