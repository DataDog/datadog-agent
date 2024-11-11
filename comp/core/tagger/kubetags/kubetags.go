// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package kubetags provides utilities to handle tags related to Kubernetes.
package kubetags

import (
	"fmt"

	"github.com/DataDog/datadog-agent/comp/core/tagger/tags"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes"
)

var kubernetesKindToTagName = map[string]string{
	kubernetes.PodKind:                   tags.KubePod,
	kubernetes.DeploymentKind:            tags.KubeDeployment,
	kubernetes.ReplicaSetKind:            tags.KubeReplicaSet,
	kubernetes.ReplicationControllerKind: tags.KubeReplicationController,
	kubernetes.StatefulSetKind:           tags.KubeStatefulSet,
	kubernetes.DaemonSetKind:             tags.KubeDaemonSet,
	kubernetes.JobKind:                   tags.KubeJob,
	kubernetes.CronJobKind:               tags.KubeCronjob,
	kubernetes.ServiceKind:               tags.KubeService,
	kubernetes.NamespaceKind:             tags.KubeNamespace,
}

// GetTagForKubernetesKind returns the tag name for the given kind
func GetTagForKubernetesKind(kind string) (string, error) {
	tagName, found := kubernetesKindToTagName[kind]
	if !found {
		return "", fmt.Errorf("no tag found for kind %s", kind)
	}

	return tagName, nil
}
