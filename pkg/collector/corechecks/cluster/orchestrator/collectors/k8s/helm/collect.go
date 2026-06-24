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

// Helm labels every storage object (ConfigMap or Secret) with
// "owner=helm".
const StorageLabelSelector = "owner=helm"

// ReleasesFromConfigMaps decodes the given Helm-managed ConfigMaps into Releases.
// The ConfigMaps are expected to already be filtered to Helm's storage objects
// (see StorageLabelSelector), for example by an informer's list options.
//
// A ConfigMap whose release data cannot be decoded is logged and skipped, so a
// single malformed object never prevents the rest from being collected.
//
// Only the ConfigMap storage backend is supported for now; Helm 3 defaults to
// Secrets, which use the same blob format and can be added later.
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
		releases = append(releases, release)
	}
	return releases
}
