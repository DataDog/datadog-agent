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
	"time"

	"golang.org/x/mod/semver"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/apimachinery/pkg/version"

	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes/apiserver/common"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

const (
	// minSupportedKubeVersion is the oldest officially supported Kubernetes version.
	minSupportedKubeVersion = "v1.23"
	// supportDropRelease is the first release without official support for
	// Kubernetes versions older than minSupportedKubeVersion.
	supportDropRelease = "7.86.0"
)

// versionPollInterval is the interval to check whether workloadmeta collected the server version.
var versionPollInterval = 5 * time.Second

// WarnOnUnsupportedVersion waits for the Kubernetes server version collected by
// workloadmeta and logs a warning if it is not supported.
func WarnOnUnsupportedVersion(ctx context.Context, wmeta workloadmeta.Component) {
	_ = wait.PollUntilContextCancel(ctx, versionPollInterval, true, func(context.Context) (bool, error) {
		kubeCapabilities, err := wmeta.GetKubeCapabilities()
		if err != nil || kubeCapabilities.Version == nil {
			return false, nil
		}
		if detected, unsupported := versionStatus(kubeCapabilities.Version); unsupported {
			log.Warnf("[DEPRECATION WARNING] %s", warningMessage(detected))
		}
		return true, nil
	})
}

// versionStatus returns the server version to display and whether it is unsupported.
func versionStatus(serverVersion *version.Info) (detected string, unsupported bool) {
	if serverVersion == nil || (serverVersion.String() == "" && serverVersion.Major == "" && serverVersion.Minor == "") {
		return "", false
	}

	detected = serverVersion.String()
	if detected == "" {
		// The server only reported Major/Minor.
		detected = fmt.Sprintf("v%s.%s", serverVersion.Major, serverVersion.Minor)
	}

	sv, ok := common.NormalizeKubeSemver(serverVersion)
	if !ok {
		log.Debugf("Cannot parse the Kubernetes server version %q, skipping the supported version check", detected)
		return detected, false
	}
	// Compare major.minor only: vendor suffixes like "-eks-abc" are semver
	// pre-releases and would otherwise sort below the stable release.
	return detected, semver.Compare(semver.MajorMinor(sv), minSupportedKubeVersion) < 0
}

func warningMessage(detected string) string {
	return fmt.Sprintf("Kubernetes %s is older than %s, the minimum supported version. "+
		"Starting with Datadog Cluster Agent %s, Kubernetes versions older than %s are no longer officially supported: "+
		"features are supported on a best-effort basis and might not work as expected. "+
		"Upgrade your cluster to Kubernetes %s or later.",
		detected, minSupportedKubeVersion, supportDropRelease, minSupportedKubeVersion, minSupportedKubeVersion)
}
