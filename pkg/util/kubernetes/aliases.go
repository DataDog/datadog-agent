// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package kubernetes

import (
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes/helpers"
)

const (
	// KubeAllowedEncodeStringAlphaNums Alias via pkg/util/kubernetes/helpers
	KubeAllowedEncodeStringAlphaNums = helpers.KubeAllowedEncodeStringAlphaNums
	// Digits Alias via pkg/util/kubernetes/helpers
	Digits = helpers.Digits
)

var (
	// ParseDeploymentForReplicaSet Alias via pkg/util/kubernetes/helpers
	ParseDeploymentForReplicaSet = helpers.ParseDeploymentForReplicaSet
	// ParseCronJobForJob Alias via pkg/util/kubernetes/helpers
	ParseCronJobForJob = helpers.ParseCronJobForJob
)

// Aliases from const.go in pkg/util/kubernetes/helpers
const (
	// EnvTagLabelKey is the label key of the env standard tag
	EnvTagLabelKey = helpers.EnvTagLabelKey
	// ServiceTagLabelKey is the label key of the service standard tag
	ServiceTagLabelKey = helpers.ServiceTagLabelKey
	// VersionTagLabelKey is the label key of the version standard tag
	VersionTagLabelKey = helpers.VersionTagLabelKey

	// KubeAppNameLabelKey is the label key of the name of the application
	KubeAppNameLabelKey = helpers.KubeAppNameLabelKey
	// KubeAppInstanceLabelKey is the label key of unique name identifying the instance of an application
	KubeAppInstanceLabelKey = helpers.KubeAppInstanceLabelKey
	// KubeAppVersionLabelKey is the label key of the current version of the application
	KubeAppVersionLabelKey = helpers.KubeAppVersionLabelKey
	// KubeAppComponentLabelKey is the label key of the component within the architecture
	KubeAppComponentLabelKey = helpers.KubeAppComponentLabelKey
	// KubeAppPartOfLabelKey is the label key of the name of a higher level application one's part of
	KubeAppPartOfLabelKey = helpers.KubeAppPartOfLabelKey
	// KubeAppManagedByLabelKey is the label key of the tool being used to manage the operation of an application
	KubeAppManagedByLabelKey = helpers.KubeAppManagedByLabelKey

	// RcIDAnnotKey is the key of the RC ID annotation
	RcIDAnnotKey = helpers.RcIDAnnotKey

	// RcRevisionAnnotKey is the key of the RC revision annotation
	RcRevisionAnnotKey = helpers.RcRevisionAnnotKey

	// EnvTagEnvVar is the environment variable of the env standard tag
	EnvTagEnvVar = helpers.EnvTagEnvVar
	// ServiceTagEnvVar is the environment variable of the service standard tag
	ServiceTagEnvVar = helpers.ServiceTagEnvVar
	// VersionTagEnvVar is the environment variable of the version standard tag
	VersionTagEnvVar = helpers.VersionTagEnvVar

	// KubeNodeRoleTagName is the role label tag name
	KubeNodeRoleTagName = helpers.KubeNodeRoleTagName

	// PodKind represents the Pod object kind
	PodKind = helpers.PodKind
	// DeploymentKind represents the Deployment object kind
	DeploymentKind = helpers.DeploymentKind
	// ReplicaSetKind represents the ReplicaSet object kind
	ReplicaSetKind = helpers.ReplicaSetKind
	// ReplicationControllerKind represents the ReplicaSetController object kind
	ReplicationControllerKind = helpers.ReplicationControllerKind
	// StatefulSetKind represents the StatefulSet object kind
	StatefulSetKind = helpers.StatefulSetKind
	// DaemonSetKind represents the DaemonSet object kind
	DaemonSetKind = helpers.DaemonSetKind
	// JobKind represents the Job object kind
	JobKind = helpers.JobKind
	// CronJobKind represents the CronJob object kind
	CronJobKind = helpers.CronJobKind
	// ServiceKind represents the ServiceKind object kind
	ServiceKind = helpers.ServiceKind
	// NamespaceKind represents the NamespaceKind object kind
	NamespaceKind = helpers.NamespaceKind

	// CriContainerNamespaceLabel is the label set on containers by runtimes with Pod Namespace
	CriContainerNamespaceLabel = helpers.CriContainerNamespaceLabel
)
