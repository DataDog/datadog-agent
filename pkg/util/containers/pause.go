// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

package containers

const containerNameLabel = "io.kubernetes.container.name"
const podNameLabel = "io.kubernetes.pod.name"
const pauseContainerNameValue = "POD"

// IsPauseContainer returns whether a container is a pause container based on the container labels
// This util can be used to exclude pause container in best-effort
// Note: Pause containers can still be excluded based on the image name via the container filtering module
func IsPauseContainer(labels map[string]string) bool {
	ctr, ctrFound := labels[containerNameLabel]
	if ctr == pauseContainerNameValue {
		return true
	}

	// Pause containers don't have a "io.kubernetes.container.name" label in containerd
	// they only have io.kubernetes.pod.name
	// See https://github.com/containerd/cri/issues/922
	_, podFound := labels[podNameLabel]
	return !ctrFound && podFound
}
