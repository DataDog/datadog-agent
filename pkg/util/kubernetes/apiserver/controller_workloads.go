// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2019 Datadog, Inc.

// +build kubeapiserver

package apiserver

import (
	appsV1 "k8s.io/api/apps/v1"
	batchV1 "k8s.io/api/batch/v1"
	batchV1B "k8s.io/api/batch/v1beta1"
	metaV1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// GetDaemonSets() retrieves all the DaemonSets in the Kubernetes / OpenShift cluster across all namespaces.
func (c *APIClient) GetDaemonSets() ([]appsV1.DaemonSet, error) {
	dsList, err := c.Cl.AppsV1().DaemonSets(metaV1.NamespaceAll).List(metaV1.ListOptions{})
	if err != nil {
		return []appsV1.DaemonSet{}, err
	}

	return dsList.Items, nil
}

// GetReplicaSets() retrieves all the ReplicaSets in the Kubernetes / OpenShift cluster across all namespaces.
func (c *APIClient) GetReplicaSets() ([]appsV1.ReplicaSet, error) {
	dsList, err := c.Cl.AppsV1().ReplicaSets(metaV1.NamespaceAll).List(metaV1.ListOptions{})
	if err != nil {
		return []appsV1.ReplicaSet{}, err
	}

	return dsList.Items, nil
}

// GetDeployments() retrieves all the Deployments in the Kubernetes / OpenShift cluster across all namespaces.
func (c *APIClient) GetDeployments() ([]appsV1.Deployment, error) {
	dmList, err := c.Cl.AppsV1().Deployments(metaV1.NamespaceAll).List(metaV1.ListOptions{})
	if err != nil {
		return []appsV1.Deployment{}, err
	}

	return dmList.Items, nil
}

// GetStatefulSets() retrieves all the StatefulSets in the Kubernetes / OpenShift cluster across all namespaces.
func (c *APIClient) GetStatefulSets() ([]appsV1.StatefulSet, error) {
	ssList, err := c.Cl.AppsV1().StatefulSets(metaV1.NamespaceAll).List(metaV1.ListOptions{})
	if err != nil {
		return []appsV1.StatefulSet{}, err
	}

	return ssList.Items, nil
}

// GetJobs() retrieves all the Jobs in the Kubernetes / OpenShift cluster across all namespaces.
func (c *APIClient) GetJobs() ([]batchV1.Job, error) {
	jList, err := c.Cl.BatchV1().Jobs(metaV1.NamespaceAll).List(metaV1.ListOptions{})
	if err != nil {
		return []batchV1.Job{}, err
	}

	return jList.Items, nil
}

// GetCronJobs() retrieves all the CronJobs in the Kubernetes / OpenShift cluster across all namespaces.
func (c *APIClient) GetCronJobs() ([]batchV1B.CronJob, error) {
	cjList, err := c.Cl.BatchV1beta1().CronJobs(metaV1.NamespaceAll).List(metaV1.ListOptions{})
	if err != nil {
		return []batchV1B.CronJob{}, err
	}

	return cjList.Items, nil
}
