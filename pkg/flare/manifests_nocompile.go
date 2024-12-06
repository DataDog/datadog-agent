// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !kubeapiserver

package flare

import (
	"errors"

	"github.com/DataDog/datadog-agent/pkg/util/log"
)

var (
	// ErrNotCompiled is returned if kubernetes apiserver support is not compiled in.
	// User classes should handle that case as gracefully as possible.
	ErrNotCompiled = errors.New("kubernetes apiserver support not compiled in")
)

// getAgentDaemonSet retrieves the DaemonSet manifest of the Agent
func getAgentDaemonSet() ([]byte, error) {
	return nil, log.Errorf("getAgentDaemonSet not implemented %s", ErrNotCompiled.Error())
}

// getClusterAgentDeployment retrieves the Deployment manifest of the Cluster Agent
func getClusterAgentDeployment() ([]byte, error) {
	return nil, log.Errorf("getClusterAgentDeployment not implemented %s", ErrNotCompiled.Error())
}

// getHelmValues retrieves the user-defined values for the Datadog Helm chart
func getHelmValues() ([]byte, error) {
	return nil, log.Errorf("getHelmValues not implemented %s", ErrNotCompiled.Error())
}

// getDatadogAgentManifest retrieves the user-defined manifest for the Datadog Agent resource (managed by the Operator)
func getDatadogAgentManifest() ([]byte, error) {
	return nil, log.Errorf("getDatadogAgentManifest not implemented %s", ErrNotCompiled.Error())
}
