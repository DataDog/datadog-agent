// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2019 Datadog, Inc.
// +build kubeapiserver

package topologycollectors

import (
	"github.com/StackVista/stackstate-agent/pkg/topology"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestCollectorInterface(t *testing.T) {

	instance := topology.Instance{Type: "kubernetes", URL: "Test-Cluster-Name"}
	clusterTopologyCommon := NewClusterTopologyCommon(instance, nil)
	testCollector := NewTestCollector(NewClusterTopologyCollector(clusterTopologyCommon))

	actualClusterExternalID := testCollector.buildClusterExternalID()
	assert.Equal(t, "urn:cluster:/kubernetes:Test-Cluster-Name", actualClusterExternalID)

	podName := "test-pod-name"
	containerName := "test-container-name"
	actualContainerExternalID := testCollector.buildContainerExternalID(podName, containerName)
	assert.Equal(t, "urn:/kubernetes:Test-Cluster-Name:pod:test-pod-name:container:test-container-name", actualContainerExternalID)

	daemonSetName := "test-daemonset"
	actualDaemonSetExternalID := testCollector.buildDaemonSetExternalID(daemonSetName)
	assert.Equal(t, "urn:/kubernetes:Test-Cluster-Name:daemonset:test-daemonset", actualDaemonSetExternalID)

	deploymentNamespace := "test-namespace"
	deploymentName := "test-deployment"
	actualDeploymentExternalID := testCollector.buildDeploymentExternalID(deploymentNamespace, deploymentName)
	assert.Equal(t, "urn:/kubernetes:Test-Cluster-Name:deployment:test-namespace:test-deployment", actualDeploymentExternalID)

	nodeName := "test-node"
	actualNodeExternalID := testCollector.buildNodeExternalID(nodeName)
	assert.Equal(t, "urn:/kubernetes:Test-Cluster-Name:node:test-node", actualNodeExternalID)

	actualPodExternalID := testCollector.buildPodExternalID(podName)
	assert.Equal(t, "urn:/kubernetes:Test-Cluster-Name:pod:test-pod-name", actualPodExternalID)

	replicaSetName := "test-replicaset"
	actualReplicaSetExternalID := testCollector.buildReplicaSetExternalID(replicaSetName)
	assert.Equal(t, "urn:/kubernetes:Test-Cluster-Name:replicaset:test-replicaset", actualReplicaSetExternalID)

	serviceID := "test-service"
	actualServiceExternalID := testCollector.buildServiceExternalID(serviceID)
	assert.Equal(t, "urn:/kubernetes:Test-Cluster-Name:service:test-service", actualServiceExternalID)

	statefulSetName := "test-statefulset"
	actualStatefulSetExternalID := testCollector.buildStatefulSetExternalID(statefulSetName)
	assert.Equal(t, "urn:/kubernetes:Test-Cluster-Name:statefulset:test-statefulset", actualStatefulSetExternalID)

	configMapNamespace := "test-namespace"
	configMapName := "test-configmap"
	actualConfigMapExternalID := testCollector.buildConfigMapExternalID(configMapNamespace, configMapName)
	assert.Equal(t, "urn:/kubernetes:Test-Cluster-Name:configmap:test-namespace:test-configmap", actualConfigMapExternalID)

	cronJobName := "test-cronjob"
	actualCronJobExternalID := testCollector.buildCronJobExternalID(cronJobName)
	assert.Equal(t, "urn:/kubernetes:Test-Cluster-Name:cronjob:test-cronjob", actualCronJobExternalID)

	jobName := "test-job"
	actualJobExternalID := testCollector.buildJobExternalID(jobName)
	assert.Equal(t, "urn:/kubernetes:Test-Cluster-Name:job:test-job", actualJobExternalID)

	ingressName := "test-ingress"
	actualIngressExternalID := testCollector.buildIngressExternalID(ingressName)
	assert.Equal(t, "urn:/kubernetes:Test-Cluster-Name:ingress:test-ingress", actualIngressExternalID)

	volumeName := "test-volume"
	actualVolumeExternalID := testCollector.buildVolumeExternalID(volumeName)
	assert.Equal(t, "urn:/kubernetes:Test-Cluster-Name:volume:test-volume", actualVolumeExternalID)

	endpointName := "test-url"
	actualEndpointExternalID := testCollector.buildEndpointExternalID(endpointName)
	assert.Equal(t, "urn:endpoint:/Test-Cluster-Name:test-url", actualEndpointExternalID)

	expectedCollectorName := "Test Collector"
	actualCollectorName := testCollector.GetName()
	assert.Equal(t, expectedCollectorName, actualCollectorName)

	expectedErrorMessage := "CollectorFunction NotImplemented"
	actualResult := testCollector.CollectorFunction()
	if actualResult != nil && actualResult.Error() != expectedErrorMessage {
		t.Errorf("Error actual = %v, and Expected = %v.", actualResult, expectedErrorMessage)
	}

	actualCollectorInstanceURL := testCollector.GetInstance().URL
	assert.Equal(t, instance.URL, actualCollectorInstanceURL)
	actualCollectorInstanceType := testCollector.GetInstance().Type
	assert.Equal(t, instance.Type, actualCollectorInstanceType)
}

// TestCollector implements the ClusterTopologyCollector interface.
type TestCollector struct {
	ClusterTopologyCollector
}

// NewTestCollector
func NewTestCollector(clusterTopologyCollector ClusterTopologyCollector) ClusterTopologyCollector {
	return &TestCollector{
		ClusterTopologyCollector: clusterTopologyCollector,
	}
}

// GetName returns the name of the TestCollector
func (_ *TestCollector) GetName() string {
	return "Test Collector"
}
