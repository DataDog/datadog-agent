// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package kubelet

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"strings"

	"github.com/DataDog/datadog-agent/comp/core/tagger/types"
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
)

// PodUIDToEntityName returns a prefixed entity name from a pod UID
func PodUIDToEntityName(uid string) string {
	if uid == "" {
		return ""
	}
	return KubePodPrefix + uid
}

// ParseMetricFromRaw parses a metric from raw prometheus text
func ParseMetricFromRaw(raw []byte, metric string) (string, error) {
	bytesReader := bytes.NewReader(raw)
	scanner := bufio.NewScanner(bytesReader)
	for scanner.Scan() {
		// skipping comments
		if string(scanner.Text()[0]) == "#" {
			continue
		}
		if strings.Contains(scanner.Text(), metric) {
			return scanner.Text(), nil
		}
	}
	return "", fmt.Errorf("%s metric not found in payload", metric)
}

// KubeContainerIDToTaggerEntityID builds an entity ID from a container ID coming from
// the pod status (i.e. including the <runtime>:// prefix).
func KubeContainerIDToTaggerEntityID(ctrID string) (types.EntityID, error) {
	sep := strings.LastIndex(ctrID, containers.EntitySeparator)
	if sep != -1 && len(ctrID) > sep+len(containers.EntitySeparator) {
		return types.NewEntityID(types.ContainerID, ctrID[sep+len(containers.EntitySeparator):]), nil
	}
	return types.EntityID{}, fmt.Errorf("can't extract an entity ID from container ID %s", ctrID)
}

// KubePodUIDToTaggerEntityID builds an entity ID from a pod UID coming from
// the pod status (i.e. including the <runtime>:// prefix).
func KubePodUIDToTaggerEntityID(podUID string) (types.EntityID, error) {
	sep := strings.LastIndex(podUID, containers.EntitySeparator)
	if sep != -1 && len(podUID) > sep+len(containers.EntitySeparator) {
		return types.NewEntityID(types.KubernetesPodUID, podUID[sep+len(containers.EntitySeparator):]), nil
	}
	return types.EntityID{}, fmt.Errorf("can't extract an entity ID from pod UID %s", podUID)
}

// KubeIDToTaggerEntityID builds a tagger entity ID from an entity ID belonging to
// a container or pod.
func KubeIDToTaggerEntityID(entityName string) (types.EntityID, error) {
	prefix, _ := containers.SplitEntityName(entityName)

	if prefix == KubePodEntityName {
		return KubePodUIDToTaggerEntityID(entityName)
	}

	return KubeContainerIDToTaggerEntityID(entityName)
}
