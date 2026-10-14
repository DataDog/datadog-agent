// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

// Package kubeinfo collects information about the Kubernetes cluster the cluster agent runs on,
// and warns when it runs on an unsupported Kubernetes version.
package kubeinfo

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

// serverVersionTimeout bounds the retries to get the server version.
var serverVersionTimeout = 10 * time.Second

// info is the information collected at startup.
type info struct {
	clusterName  string
	clusterID    string
	distribution string
	// serverVersion is empty until the server version is known.
	serverVersion string
	// unsupported is true when the server version is older than common.MinSupportedKubeVersion.
	unsupported bool
}

var (
	mu    sync.RWMutex
	state info
)

// SetClusterInfo stores the cluster name, ID and distribution.
func SetClusterInfo(clusterName, clusterID, distribution string) {
	mu.Lock()
	defer mu.Unlock()
	state.clusterName = clusterName
	state.clusterID = clusterID
	state.distribution = distribution
}

// CheckServerVersion gets the Kubernetes server version and logs a warning if it is not supported.
func CheckServerVersion(discoveryCl discovery.DiscoveryInterface) {
	serverVersion, err := common.KubeServerVersion(discoveryCl, serverVersionTimeout)
	if err != nil {
		log.Infof("Cannot get the Kubernetes server version, skipping the supported version check: %v", err)
		return
	}
	setServerVersion(serverVersion)
}

func setServerVersion(serverVersion *version.Info) {
	if serverVersion == nil {
		return
	}

	detected := serverVersion.String()
	if detected == "" {
		// The server only reported Major/Minor.
		detected = fmt.Sprintf("v%s.%s", serverVersion.Major, serverVersion.Minor)
	}

	supported, ok := common.IsKubeVersionSupported(serverVersion)
	if !ok {
		log.Debugf("Cannot parse the Kubernetes server version %q, skipping the supported version check", detected)
	}
	unsupported := ok && !supported
	if unsupported {
		log.Warnf("[DEPRECATION WARNING] %s", warningMessage(detected))
	}

	mu.Lock()
	defer mu.Unlock()
	state.serverVersion = detected
	state.unsupported = unsupported
}

func getInfo() info {
	mu.RLock()
	defer mu.RUnlock()
	return state
}

func warningMessage(detected string) string {
	return fmt.Sprintf("Kubernetes %s is older than %s, the minimum supported version. "+
		"Starting with Datadog Cluster Agent %s, Kubernetes versions older than %s are no longer officially supported: "+
		"features are supported on a best-effort basis and might not work as expected. "+
		"Upgrade your cluster to Kubernetes %s or later.",
		detected, common.MinSupportedKubeVersion, supportDropRelease, common.MinSupportedKubeVersion, common.MinSupportedKubeVersion)
}
