// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !kubeapiserver
// +build !kubeapiserver

package flare

import (
	"errors"

	v1 "k8s.io/api/core/v1"

	"github.com/DataDog/datadog-agent/pkg/util/kubernetes/apiserver"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

var (
	// ErrNotCompiled is returned if kubernetes apiserver support is not compiled in.
	// User classes should handle that case as gracefully as possible.
	ErrNotCompiled = errors.New("kubernetes apiserver support not compiled in")
)

const (
	HELM_CHART_RELEASE_NAME       = "CHART_RELEASE_NAME"
	HELM_CHART_RELEASE_NAMESPACE  = "DD_KUBE_RESOURCES_NAMESPACE"
	HELM_AGENT_DAEMONSET          = "AGENT_DAEMONSET"
	HELM_CLUSTER_AGENT_DEPLOYMENT = "CLUSTER_AGENT_DEPLOYMENT"
)

// Retrieve a DaemonSet YAML from the API server for a given name and namespace, and returns the associated YAML manifest into a a byte array.
// Its purpose is to retrieve the Datadog Agent DaemonSet manifest when building a Cluster Agent flare.
func GetDaemonset(cl *apiserver.APIClient, name string, namespace string) ([]byte, error) {
	return nil, log.Errorf("GetDaemonset not implemented %s", ErrNotCompiled.Error())
}

// Retrieve a Deployment YAML from the API server for a given name and namespace, and returns the associated YAML manifest into a a byte array.
// Its purpose is to retrieve the Datadog Cluster Agent Deployment manifest when building a Cluster Agent flare.
func GetDeployment(cl *apiserver.APIClient, name string, namespace string) ([]byte, error) {
	return nil, log.Errorf("GetDeployment not implemented %s", ErrNotCompiled.Error())
}

// getDeployedHelmSecret returns the secret for a given release.
// Only a single release for a given name can be deployed at one time.
func getDeployedHelmSecret(cl *apiserver.APIClient, name string, namespace string) (*v1.Secret, error) {
	return nil, log.Errorf("getDeployedHelmSecret not implemented %s", ErrNotCompiled.Error())
}

// getDeployedHelmConfigmap returns the configmap for a given release.
// Only a single release for a given name can be deployed at one time.
func getDeployedHelmConfigmap(cl *apiserver.APIClient, name string, namespace string) (*v1.ConfigMap, error) {
	return nil, log.Errorf("getDeployedHelmConfigmap not implemented %s", ErrNotCompiled.Error())
}

// decodeChartValuesFromRelease returns a byte array with the user values from an encoded Helm chart release
func decodeChartValuesFromRelease(encodedRelease string) ([]byte, error) {
	return nil, log.Errorf("decodeChartValuesFromRelease not implemented %s", ErrNotCompiled.Error())
}
