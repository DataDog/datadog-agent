// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

package kubernetes

const (
	// EnvTagLabelKey is the label key of the env standard tag
	EnvTagLabelKey = "tags.datadoghq.com/env"
	// ServiceTagLabelKey is the label key of the service standard tag
	ServiceTagLabelKey = "tags.datadoghq.com/service"
	// VersionTagLabelKey is the label key of the version standard tag
	VersionTagLabelKey = "tags.datadoghq.com/version"

	// KubeAppNameLabelKey is the label key of the name of the application
	KubeAppNameLabelKey = "app.kubernetes.io/name"
	// KubeAppInstanceLabelKey is the label key of unique name identifying the instance of an application
	KubeAppInstanceLabelKey = "app.kubernetes.io/instance"
	// KubeAppVersionLabelKey is the label key of the current version of the application
	KubeAppVersionLabelKey = "app.kubernetes.io/version"
	// KubeAppComponentLabelKey is the label key of the component within the architecture
	KubeAppComponentLabelKey = "app.kubernetes.io/component"
	// KubeAppPartOfLabelKey is the label key of the name of a higher level application one's part of
	KubeAppPartOfLabelKey = "app.kubernetes.io/part-of"
	// KubeAppManagedByLabelKey is the label key of the tool being used to manage the operation of an application
	KubeAppManagedByLabelKey = "app.kubernetes.io/managed-by"

	// EnvTagEnvVar is the environment variable of the env standard tag
	EnvTagEnvVar = "DD_ENV"
	// ServiceTagEnvVar is the environment variable of the service standard tag
	ServiceTagEnvVar = "DD_SERVICE"
	// VersionTagEnvVar is the environment variable of the version standard tag
	VersionTagEnvVar = "DD_VERSION"

	// KubeNodeRoleTagName is the role label tag name
	KubeNodeRoleTagName = "kube_node_role"

	// PodKind represents the Pod object kind
	PodKind = "Pod"
	// DeploymentKind represents the Deployment object kind
	DeploymentKind = "Deployment"
	// ReplicaSetKind represents the ReplicaSet object kind
	ReplicaSetKind = "ReplicaSet"
	// ReplicationControllerKind represents the ReplicaSetController object kind
	ReplicationControllerKind = "ReplicationController"
	// StatefulSetKind represents the StatefulSet object kind
	StatefulSetKind = "StatefulSet"
	// DaemonSetKind represents the DaemonSet object kind
	DaemonSetKind = "DaemonSet"
	// JobKind represents the Job object kind
	JobKind = "Job"
	// CronJobKind represents the CronJob object kind
	CronJobKind = "CronJob"
	// ServiceKind represents the ServiceKind object kind
	ServiceKind = "Service"

	// PodTagName represents the pods tag name
	PodTagName = "pod_name"
	// DeploymentTagName represents the Deployment tag name
	DeploymentTagName = "kube_deployment"
	// ReplicaSetTagName represents the ReplicaSet tag name
	ReplicaSetTagName = "kube_replica_set"
	// ReplicationControllerTagName represents the ReplicationController tag name
	ReplicationControllerTagName = "kube_replication_controller"
	// StatefulSetTagName represents the StatefulSet tag name
	StatefulSetTagName = "kube_stateful_set"
	// DaemonSetTagName represents the DaemonSet tag name
	DaemonSetTagName = "kube_daemon_set"
	// JobTagName represents the Job tag name
	JobTagName = "kube_job"
	// CronJobTagName represents the CronJob tag name
	CronJobTagName = "kube_cronjob"
	// ServiceTagName represents the CronJob tag name
	ServiceTagName = "kube_service"
)

// KindToTagName returns the tag name for a given kubernetes object name
var KindToTagName = map[string]string{
	PodKind:                   PodTagName,
	DeploymentKind:            DeploymentTagName,
	ReplicaSetKind:            ReplicaSetTagName,
	ReplicationControllerKind: ReplicationControllerTagName,
	StatefulSetKind:           StatefulSetTagName,
	DaemonSetKind:             DaemonSetTagName,
	JobKind:                   JobTagName,
	CronJobKind:               CronJobTagName,
	ServiceKind:               ServiceTagName,
}
