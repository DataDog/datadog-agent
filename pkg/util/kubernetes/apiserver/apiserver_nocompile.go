// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018 Datadog, Inc.

// +build !kubeapiserver

package apiserver

import (
	"errors"
	"sync"

	log "github.com/cihub/seelog"
)

var (
	// ErrNotCompiled is returned if kubernetes apiserver support is not compiled in.
	// User classes should handle that case as gracefully as possible.
	ErrNotCompiled = errors.New("kubernetes apiserver support not compiled in")
)

// APIClient provides authenticated access to the
type APIClient struct{}

// MetadataMapperBundle maps the podNames to the metadata they are associated with.
type MetadataMapperBundle struct {
	PodNameToService map[string][]string
	m                sync.RWMutex
}

// GetPodMetadataNames is used when the API endpoint of the DCA to get the services of a pod is hit.
func GetPodMetadataNames(nodeName string, podName string) ([]string, error) {
	log.Errorf("GetPodMetadataNames not implemented %s", ErrNotCompiled.Error())
	return nil, nil
}

// GetMetadataMapBundleOnNode is used for the CLI svcmap command to output given a nodeName
func GetMetadataMapBundleOnNode(nodeName string) (map[string]interface{}, error) {
	log.Errorf("GetMetadataMapBundleOnNode not implemented %s", ErrNotCompiled.Error())
	return nil, nil
}

// GetMetadataMapBundleOnAllNodes is used for the CLI svcmap command to run fetch the service map of all nodes.
func GetMetadataMapBundleOnAllNodes() (map[string]interface{}, error) {
	log.Errorf("GetMetadataMapBundleOnAllNodes not implemented %s", ErrNotCompiled.Error())
	return nil, nil
}

// StartMetadataMapping is only called once, when we have confirmed we could correctly connect to the API server.
func (c *APIClient) StartMetadataMapping() {
	log.Errorf("StartMetadataMapping not implemented %s", ErrNotCompiled.Error())
	return
}
