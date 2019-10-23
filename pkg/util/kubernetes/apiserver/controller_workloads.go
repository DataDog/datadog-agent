// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2019 Datadog, Inc.

// +build kubeapiserver

package apiserver

import (
	appsV1 "k8s.io/api/apps/v1"
	batchV1 "k8s.io/api/batch/v1beta1"
	coreV1 "k8s.io/api/core/v1"
	storageV1 "k8s.io/api/storage/v1beta1"
	metaV1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// GetDaemonSets() retrieves all the DaemonSets in the Kubernetes cluster across all namespaces.
func (c *APIClient) GetDaemonSets() ([]appsV1.DaemonSet, error) {
	dsList, err := c.Cl.AppsV1().DaemonSets(metaV1.NamespaceAll).List(metaV1.ListOptions{})
	if err != nil {
		return []appsV1.DaemonSet{}, err
	}

	return dsList.Items, nil
}

// GetReplicaSets() retrieves all the ReplicaSets in the Kubernetes cluster across all namespaces.
func (c *APIClient) GetReplicaSets() ([]appsV1.ReplicaSet, error) {
	dsList, err := c.Cl.AppsV1().ReplicaSets(metaV1.NamespaceAll).List(metaV1.ListOptions{})
	if err != nil {
		return []appsV1.ReplicaSet{}, err
	}

	return dsList.Items, nil
}

// GetDeployments() retrieves all the Deployments in the Kubernetes cluster across all namespaces.
func (c *APIClient) GetDeployments() ([]appsV1.Deployment, error) {
	dmList, err := c.Cl.AppsV1().Deployments(metaV1.NamespaceAll).List(metaV1.ListOptions{})
	if err != nil {
		return []appsV1.Deployment{}, err
	}

	return dmList.Items, nil
}

// GetStatefulSets() retrieves all the StatefulSets in the Kubernetes cluster across all namespaces.
func (c *APIClient) GetStatefulSets() ([]appsV1.StatefulSet, error) {
	ssList, err := c.Cl.AppsV1().StatefulSets(metaV1.NamespaceAll).List(metaV1.ListOptions{})
	if err != nil {
		return []appsV1.StatefulSet{}, err
	}

	return ssList.Items, nil
}

// GetCronJobs() retrieves all the CronJobs in the Kubernetes cluster across all namespaces.
func (c *APIClient) GetCronJobs() ([]batchV1.CronJob, error) {
	cjList, err := c.Cl.BatchV1beta1().CronJobs(metaV1.NamespaceAll).List(metaV1.ListOptions{})
	if err != nil {
		return []batchV1.CronJob{}, err
	}

	return cjList.Items, nil
}

// GetPersistentVolumeClaims() retrieves all the PersistentVolumeClaims in the Kubernetes cluster across all namespaces.
func (c *APIClient) GetPersistentVolumeClaims() ([]coreV1.PersistentVolumeClaim, error) {
	pvList, err := c.Cl.CoreV1().PersistentVolumeClaims(metaV1.NamespaceAll).List(metaV1.ListOptions{})
	if err != nil {
		return []coreV1.PersistentVolumeClaim{}, err
	}

	return pvList.Items, nil
}

// GetPersistentVolumes() retrieves all the PersistentVolumes in the Kubernetes cluster across all namespaces.
func (c *APIClient) GetPersistentVolumes() ([]coreV1.PersistentVolume, error) {
	pvList, err := c.Cl.CoreV1().PersistentVolumes().List(metaV1.ListOptions{})
	if err != nil {
		return []coreV1.PersistentVolume{}, err
	}

	return pvList.Items, nil
}

// GetVolumeAttachments() retrieves all the VolumeAttachment in the Kubernetes cluster across all namespaces.
func (c *APIClient) GetVolumeAttachments() ([]storageV1.VolumeAttachment, error) {
	cjList, err := c.Cl.StorageV1beta1().VolumeAttachments().List(metaV1.ListOptions{})
	if err != nil {
		return []storageV1.VolumeAttachment{}, err
	}

	return cjList.Items, nil
}
