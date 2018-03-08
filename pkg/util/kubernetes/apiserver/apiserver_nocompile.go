// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018 Datadog, Inc.

// +build !kubeapiserver

package apiserver

import (
	"errors"

	log "github.com/cihub/seelog"
)

var (
	// ErrNotCompiled is returned if kubernetes apiserver support is not compiled in.
	// User classes should handle that case as gracefully as possible.
	ErrNotCompiled = errors.New("kubernetes apiserver support not compiled in")
)

// GetPodServiceNames is used when the API endpoint of the DCA to get the services of a pod is hit.
func GetPodServiceNames(nodeName string, podName string) ([]string, error) {
	log.Errorf("GetPodServiceNames not implemented %s", ErrNotCompiled.Error())
	return nil, nil
}

// GetServiceMapBundleOnNode is used for the CLI svcmap command to output given a
func GetServiceMapBundleOnNode(nodeName string) (map[string]interface{}, error) {
	log.Errorf("GetPodServiceNames not implemented %s", ErrNotCompiled.Error())
	return nil, nil
}
