// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

// Package kubeinfo collects information about the Kubernetes cluster the cluster agent runs on,
// and warns when it runs on an unsupported Kubernetes version.
package kubeinfo

import (
	"context"
	"fmt"
	"sync"

	"k8s.io/apimachinery/pkg/version"

	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes/apiserver/common"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// supportDropRelease is the first release without official support for
// Kubernetes versions older than common.MinSupportedKubeVersion.
const supportDropRelease = "7.86.0"

// clusterInfo is the cluster information collected at startup.
type clusterInfo struct {
	clusterName  string
	clusterID    string
	distribution string
}

var (
	mu    sync.RWMutex
	state clusterInfo
)

// SetClusterInfo stores the cluster name, ID and distribution.
func SetClusterInfo(clusterName, clusterID, distribution string) {
	mu.Lock()
	defer mu.Unlock()
	state = clusterInfo{clusterName: clusterName, clusterID: clusterID, distribution: distribution}
}

func getClusterInfo() clusterInfo {
	mu.RLock()
	defer mu.RUnlock()
	return state
}

// WarnOnUnsupportedVersion waits for the Kubernetes server version collected by
// workloadmeta and logs a warning if it is not supported.
func WarnOnUnsupportedVersion(ctx context.Context, wmeta workloadmeta.Component) {
	filter := workloadmeta.NewFilterBuilder().
		AddKind(workloadmeta.KindKubeCapabilities).
		SetEventType(workloadmeta.EventTypeSet).
		Build()
	ch := wmeta.Subscribe("kubeinfo", workloadmeta.NormalPriority, filter)
	defer wmeta.Unsubscribe(ch)

	for {
		select {
		case <-ctx.Done():
			return
		case bundle, ok := <-ch:
			if !ok {
				return
			}
			bundle.Acknowledge()
			for _, event := range bundle.Events {
				kubeCapabilities, ok := event.Entity.(*workloadmeta.KubeCapabilities)
				if !ok || kubeCapabilities.Version == nil {
					continue
				}
				if detected, unsupported := versionStatus(kubeCapabilities.Version); unsupported {
					log.Warnf("[DEPRECATION WARNING] %s", warningMessage(detected))
				}
				return
			}
		}
	}
}

// versionStatus returns the server version to display and whether it is unsupported.
func versionStatus(serverVersion *version.Info) (detected string, unsupported bool) {
	if serverVersion == nil {
		return "", false
	}

	detected = serverVersion.String()
	if detected == "" {
		// The server only reported Major/Minor.
		detected = fmt.Sprintf("v%s.%s", serverVersion.Major, serverVersion.Minor)
	}

	supported, ok := common.IsKubeVersionSupported(serverVersion)
	if !ok {
		log.Debugf("Cannot parse the Kubernetes server version %q, skipping the supported version check", detected)
	}
	return detected, ok && !supported
}

func warningMessage(detected string) string {
	return fmt.Sprintf("Kubernetes %s is older than %s, the minimum supported version. "+
		"Starting with Datadog Cluster Agent %s, Kubernetes versions older than %s are no longer officially supported: "+
		"features are supported on a best-effort basis and might not work as expected. "+
		"Upgrade your cluster to Kubernetes %s or later.",
		detected, common.MinSupportedKubeVersion, supportDropRelease, common.MinSupportedKubeVersion, common.MinSupportedKubeVersion)
}
