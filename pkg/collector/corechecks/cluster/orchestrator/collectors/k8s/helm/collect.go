// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver && orchestrator

package helm

import (
	"context"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"

	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// helmStorageLabelSelector matches the ConfigMaps that Helm uses to store its
// release records. Helm labels every storage object (ConfigMap or Secret) with
// "owner=helm".
const helmStorageLabelSelector = "owner=helm"

// CollectReleases lists every Helm-managed ConfigMap across all namespaces and
// decodes each into a Release.
//
// A ConfigMap whose release data cannot be decoded is logged and skipped, so a
// single malformed object never prevents the rest from being collected.
//
// Only the ConfigMap storage backend is supported for now; Helm 3 defaults to
// Secrets, which use the same blob format and can be added later.
func CollectReleases(ctx context.Context, client kubernetes.Interface) ([]*Release, error) {
	configMaps, err := client.CoreV1().ConfigMaps(metav1.NamespaceAll).List(ctx, metav1.ListOptions{
		LabelSelector: helmStorageLabelSelector,
	})
	if err != nil {
		return nil, err
	}

	releases := make([]*Release, 0, len(configMaps.Items))
	for i := range configMaps.Items {
		cm := &configMaps.Items[i]
		release, err := ParseRelease(cm.Data["release"])
		if err != nil {
			log.Debugf("Skipping Helm ConfigMap %s/%s: %v", cm.Namespace, cm.Name, err)
			continue
		}
		releases = append(releases, release)
	}

	return releases, nil
}
