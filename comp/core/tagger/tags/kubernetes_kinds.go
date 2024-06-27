// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package tags

import (
	"fmt"

	"github.com/DataDog/datadog-agent/pkg/util/kubernetes"
)

var kindToTagName = map[string]string{
	kubernetes.PodKind:                   KubePod,
	kubernetes.DeploymentKind:            KubeDeployment,
	kubernetes.ReplicaSetKind:            KubeReplicaSet,
	kubernetes.ReplicationControllerKind: KubeReplicationController,
	kubernetes.StatefulSetKind:           KubeStatefulSet,
	kubernetes.DaemonSetKind:             KubeDaemonSet,
	kubernetes.JobKind:                   KubeJob,
	kubernetes.CronJobKind:               KubeCronjob,
	kubernetes.ServiceKind:               KubeService,
	kubernetes.NamespaceKind:             KubeNamespace,
}

// GetTagForKind returns the tag name for the given kind
func GetTagForKind(kind string) (string, error) {
	tagName, found := kindToTagName[kind]
	if !found {
		return "", fmt.Errorf("no tag found for kind %s", kind)
	}

	return tagName, nil
}
