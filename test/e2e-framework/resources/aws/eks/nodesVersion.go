// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package eks

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"sync"
)

//go:embed nodes_versions.json
var nodesVersionJSON []byte

var (
	versionOnce sync.Once
	version     map[string]map[string]string
	versionErr  error
)

func getNodesVersion() (map[string]map[string]string, error) {
	versionOnce.Do(func() {
		version = make(map[string]map[string]string)
		versionErr = json.Unmarshal(nodesVersionJSON, &version)
	})
	return version, versionErr
}

func GetNodesVersion(amiType string, kubernetesVersion string) (string, error) {
	version, err := getNodesVersion()
	if err != nil {
		return "", err
	}
	if _, ok := version[amiType]; !ok {
		return "", fmt.Errorf("ami type %s not found in nodes_versions.json", amiType)
	}
	if _, ok := version[amiType][kubernetesVersion]; !ok {
		return "", fmt.Errorf("kubernetes version %s not found for ami type %s in nodes_versions.json", kubernetesVersion, amiType)
	}
	return version[amiType][kubernetesVersion], nil
}
