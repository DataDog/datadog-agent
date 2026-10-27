// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package libraryinjection

import (
	"golang.org/x/mod/semver"
	"k8s.io/apimachinery/pkg/version"

	"github.com/DataDog/datadog-agent/pkg/util/kubernetes/apiserver/common"
)

const minImageVolumeKubeVersion = "v1.33.0"

// IsImageVolumeSupported returns whether the Kubernetes API server version
// supports image volumes and subPath (Kubernetes v1.33.0+).
func IsImageVolumeSupported(serverVersion *version.Info) bool {
	sv, ok := common.NormalizeKubeSemver(serverVersion)
	if !ok {
		return false
	}
	return semver.Compare(sv, minImageVolumeKubeVersion) >= 0
}
