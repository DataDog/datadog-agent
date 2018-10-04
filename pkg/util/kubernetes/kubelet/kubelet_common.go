// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018 Datadog, Inc.

package kubelet

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"strings"
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

// TrimRuntimeFromCID takes a full containerID with runtime prefix
// and only returns the short cID, compatible with a docker container ID
func TrimRuntimeFromCID(cid string) string {
	parts := strings.SplitN(cid, "://", 2)
	return parts[len(parts)-1]
}
