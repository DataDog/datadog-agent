// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package libraryinjection

import (
	"fmt"
	"strings"

	"golang.org/x/mod/semver"
	"k8s.io/apimachinery/pkg/version"
)

const minImageVolumeKubeVersion = "v1.33.0"

// IsImageVolumeSupported returns whether the Kubernetes API server version
// supports image volumes and subPath (Kubernetes v1.33.0+).
func IsImageVolumeSupported(serverVersion *version.Info) bool {
	sv, ok := normalizeKubeSemver(serverVersion)
	if !ok {
		return false
	}
	return semver.Compare(sv, minImageVolumeKubeVersion) >= 0
}

// TODO: Determine whether we can standardize Kubernetes Server version parsing across the codebase.
func normalizeKubeSemver(v *version.Info) (string, bool) {
	if v == nil {
		return "", false
	}

	// Prefer GitVersion (returned by v.String()) and strip build metadata like "+k3s1".
	s := v.String()
	if idx := strings.IndexByte(s, '+'); idx >= 0 {
		s = s[:idx]
	}
	if semver.IsValid(s) {
		return s, true
	}

	// Fallback to major/minor parsing.
	major := strings.TrimPrefix(v.Major, "v")
	minorDigits := digitsPrefix(v.Minor)
	if major == "" || minorDigits == "" {
		return "", false
	}

	s = fmt.Sprintf("v%s.%s.0", major, minorDigits)
	if semver.IsValid(s) {
		return s, true
	}

	return "", false
}

func digitsPrefix(s string) string {
	var b strings.Builder
	for _, r := range s {
		if r < '0' || r > '9' {
			break
		}
		b.WriteRune(r)
	}
	return b.String()
}
