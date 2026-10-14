// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

// Package kubeversion warns when the cluster agent runs on an unsupported Kubernetes version.
package kubeversion

import (
	"fmt"
	"sync"
	"time"

	"k8s.io/apimachinery/pkg/version"
	"k8s.io/client-go/discovery"

	"github.com/DataDog/datadog-agent/pkg/util/kubernetes/apiserver/common"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// supportDropRelease is the first release without official support for
// Kubernetes versions older than common.MinSupportedKubeVersion.
const supportDropRelease = "7.86.0"

var (
	mu sync.RWMutex
	// unsupportedVersion is the server version when it is not supported, empty otherwise.
	unsupportedVersion string
)

// Check gets the Kubernetes server version and logs a warning if it is not supported.
func Check(discoveryCl discovery.DiscoveryInterface) {
	serverVersion, err := common.KubeServerVersion(discoveryCl, 10*time.Second)
	if err != nil {
		log.Infof("Cannot get the Kubernetes server version, skipping the supported version check: %v", err)
		return
	}
	setServerVersion(serverVersion)
}

func setServerVersion(serverVersion *version.Info) {
	supported, ok := common.IsKubeVersionSupported(serverVersion)
	if !ok {
		log.Debugf("Cannot parse the Kubernetes server version %q, skipping the supported version check", serverVersion)
		return
	}

	detected := ""
	if !supported {
		detected = serverVersion.String()
		log.Warnf("[DEPRECATION WARNING] %s", warningMessage(detected))
	}

	mu.Lock()
	defer mu.Unlock()
	unsupportedVersion = detected
}

func getUnsupportedVersion() string {
	mu.RLock()
	defer mu.RUnlock()
	return unsupportedVersion
}

func warningMessage(detected string) string {
	return fmt.Sprintf("Kubernetes %s is older than %s, the minimum supported version. "+
		"Starting with Datadog Cluster Agent %s, Kubernetes versions older than %s are no longer officially supported: "+
		"features are supported on a best-effort basis and might not work as expected. "+
		"Upgrade your cluster to Kubernetes %s or later.",
		detected, common.MinSupportedKubeVersion, supportDropRelease, common.MinSupportedKubeVersion, common.MinSupportedKubeVersion)
}
