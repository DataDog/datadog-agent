// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018 Datadog, Inc.

package kubelet

import (
	"errors"
	"fmt"
)

var (
	// ErrNotCompiled is returned if kubelet support is not compiled in.
	// User classes should handle that case as gracefully as possible.
	ErrNotCompiled = errors.New("kubelet support not compiled in")

	// KubePodPrefix is the entity prefix for Kubernetes pods
	KubePodPrefix = "kubernetes_pod://"
)

// PodUIDToEntityName returns a prefixed entity name from a pod UID
func PodUIDToEntityName(uid string) string {
	if uid == "" {
		return ""
	}
	return fmt.Sprintf("%s%s", KubePodPrefix, uid)
}
