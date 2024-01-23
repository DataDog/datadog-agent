// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package kubelet

import (
	"errors"

	"github.com/DataDog/datadog-agent/pkg/util/containers"
)

var (
	// ErrNotCompiled is returned if kubelet support is not compiled in.
	// User classes should handle that case as gracefully as possible.
	ErrNotCompiled = errors.New("kubelet support not compiled in")

	// KubePodEntityName is the entity name for Kubernetes pods.
	KubePodEntityName = "kubernetes_pod"

	// KubePodPrefix is the entity prefix for Kubernetes pods
	KubePodPrefix = KubePodEntityName + containers.EntitySeparator

	// KubePodTaggerEntityName is the tagger entity name for Kubernetes pods
	KubePodTaggerEntityName = "kubernetes_pod_uid"

	// KubePodTaggerEntityPrefix is the tagger entity prefix for Kubernetes pods
	KubePodTaggerEntityPrefix = KubePodTaggerEntityName + containers.EntitySeparator

	// KubeNodeTaggerEntityName is the tagger entity name for Kubernetes nodes
	KubeNodeTaggerEntityName = "kubernetes_node_uid"

	// KubeNodeTaggerEntityPrefix is the tagger entity prefix for Kubernetes pods
	KubeNodeTaggerEntityPrefix = KubeNodeTaggerEntityName + containers.EntitySeparator
)

// PodUIDToEntityName returns a prefixed entity name from a pod UID
func PodUIDToEntityName(uid string) string {
	panic("not called")
}

// PodUIDToTaggerEntityName returns a prefixed tagger entity name from a pod UID
func PodUIDToTaggerEntityName(uid string) string {
	panic("not called")
}

// NodeUIDToTaggerEntityName returns a prefixed tagger entity name from a node UID
func NodeUIDToTaggerEntityName(uid string) string {
	panic("not called")
}

// ParseMetricFromRaw parses a metric from raw prometheus text
func ParseMetricFromRaw(raw []byte, metric string) (string, error) {
	panic("not called")
}

// KubeContainerIDToTaggerEntityID builds an entity ID from a container ID coming from
// the pod status (i.e. including the <runtime>:// prefix).
func KubeContainerIDToTaggerEntityID(ctrID string) (string, error) {
	panic("not called")
}

// KubePodUIDToTaggerEntityID builds an entity ID from a pod UID coming from
// the pod status (i.e. including the <runtime>:// prefix).
func KubePodUIDToTaggerEntityID(podUID string) (string, error) {
	panic("not called")
}

// KubeIDToTaggerEntityID builds a tagger entity ID from an entity ID belonging to
// a container or pod.
func KubeIDToTaggerEntityID(entityName string) (string, error) {
	panic("not called")
}
