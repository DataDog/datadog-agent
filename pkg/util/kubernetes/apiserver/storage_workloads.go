// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2019 Datadog, Inc.

// +build kubeapiserver

package apiserver

import (
	coreV1 "k8s.io/api/core/v1"
	metaV1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// GetConfigMaps() retrieves all the ConfigMaps in the Kubernetes / OpenShift cluster across all namespaces.
func (c *APIClient) GetConfigMaps() ([]coreV1.ConfigMap, error) {
	cmList, err := c.Cl.CoreV1().ConfigMaps(metaV1.NamespaceAll).List(metaV1.ListOptions{})
	if err != nil {
		return []coreV1.ConfigMap{}, err
	}

	return cmList.Items, nil
}

// GetPersistentVolumes() retrieves all the PersistentVolumes in the Kubernetes / OpenShift cluster across all namespaces.
func (c *APIClient) GetPersistentVolumes() ([]coreV1.PersistentVolume, error) {
	pvList, err := c.Cl.CoreV1().PersistentVolumes().List(metaV1.ListOptions{})
	if err != nil {
		return []coreV1.PersistentVolume{}, err
	}

	return pvList.Items, nil
}
