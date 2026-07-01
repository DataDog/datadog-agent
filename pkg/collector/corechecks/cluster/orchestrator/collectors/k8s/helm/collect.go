// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver && orchestrator

package helm

import (
	corev1 "k8s.io/api/core/v1"

	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// Helm labels storage objects with "owner=helm".
const StorageLabelSelector = "owner=helm"

// ReleasesFromConfigMaps decodes the given Helm-managed ConfigMaps into Releases.
//
// NOTE: this is currently called independently by both the Helm release and Helm
// chart collectors, so every ConfigMap is decoded twice
// per collection cycle. Future optimization: decode releases once and share the
// result between the two collectors.
func ReleasesFromConfigMaps(configMaps []*corev1.ConfigMap) []*Release {
	releases := make([]*Release, 0, len(configMaps))
	for _, cm := range configMaps {
		if cm == nil {
			continue
		}
		release, err := ParseRelease(cm.Data["release"])
		if err != nil {
			log.Debugf("Skipping Helm ConfigMap %s/%s: %v", cm.Namespace, cm.Name, err)
			continue
		}
		release.ResourceVersion = cm.ResourceVersion
		releases = append(releases, release)
	}
	return releases
}
