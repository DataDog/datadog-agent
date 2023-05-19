// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !kubeapiserver

package apiserver

import (
	"context"
	"errors"
	"time"

	"k8s.io/client-go/kubernetes"

	apiv1 "github.com/DataDog/datadog-agent/pkg/clusteragent/api/v1"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

var (
	// ErrNotCompiled is returned if kubernetes apiserver support is not compiled in.
	// User classes should handle that case as gracefully as possible.
	ErrNotCompiled = errors.New("kubernetes apiserver support not compiled in")
)

// APIClient provides authenticated access to the
type APIClient struct {
	Cl interface{}
}

// GetAPIClient returns the shared ApiClient instance.
func GetAPIClient() (*APIClient, error) {
	log.Errorf("GetAPIClient not implemented %s", ErrNotCompiled.Error())
	return &APIClient{}, nil
}

// WaitForAPIClient returns the shared ApiClient instance.
func WaitForAPIClient(ctx context.Context) (*APIClient, error) {
	log.Errorf("WaitForAPIClient not implemented %s", ErrNotCompiled.Error())
	return &APIClient{}, nil
}

// GetPodMetadataNames is used when the API endpoint of the DCA to get the services of a pod is hit.
func GetPodMetadataNames(nodeName, ns, podName string) ([]string, error) {
	log.Errorf("GetPodMetadataNames not implemented %s", ErrNotCompiled.Error())
	return nil, nil
}

// GetMetadataMapBundleOnNode is used for the CLI svcmap command to output given a nodeName
func GetMetadataMapBundleOnNode(nodeName string) (*apiv1.MetadataResponse, error) {
	log.Errorf("GetMetadataMapBundleOnNode not implemented %s", ErrNotCompiled.Error())
	return nil, nil
}

// GetMetadataMapBundleOnAllNodes is used for the CLI svcmap command to run fetch the service map of all nodes.
func GetMetadataMapBundleOnAllNodes(_ *APIClient) (*apiv1.MetadataResponse, error) {
	log.Errorf("GetMetadataMapBundleOnAllNodes not implemented %s", ErrNotCompiled.Error())
	return nil, nil
}

// GetNodeLabels retrieves the labels of the queried node from the cache of the shared informer.
func GetNodeLabels(_ *APIClient, nodeName string) (map[string]string, error) {
	log.Errorf("GetNodeLabels not implemented %s", ErrNotCompiled.Error())
	return nil, nil
}

// GetKubeClient returns a Kubernetes client.
func GetKubeClient(timeout time.Duration) (kubernetes.Interface, error) {
	return nil, ErrNotCompiled
}
