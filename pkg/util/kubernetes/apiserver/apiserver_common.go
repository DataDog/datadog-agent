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
	coreV1 "k8s.io/api/core/v1"
	extensionsV1B "k8s.io/api/extensions/v1beta1"
)

type APICollectorClient interface {
	GetDaemonSets() ([]appsV1.DaemonSet, error)
	GetReplicaSets() ([]appsV1.ReplicaSet, error)
	GetDeployments() ([]appsV1.Deployment, error)
	GetStatefulSets() ([]appsV1.StatefulSet, error)
	GetJobs() ([]batchV1.Job, error)
	GetCronJobs() ([]batchV1B.CronJob, error)
	GetEndpoints() ([]coreV1.Endpoints, error)
	GetNodes() ([]coreV1.Node, error)
	GetPods() ([]coreV1.Pod, error)
	GetServices() ([]coreV1.Service, error)
	GetIngresses() ([]extensionsV1B.Ingress, error)
	GetConfigMaps() ([]coreV1.ConfigMap, error)
	GetPersistentVolumes() ([]coreV1.PersistentVolume, error)
}
