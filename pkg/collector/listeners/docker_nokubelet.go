// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2017 Datadog, Inc.

// +build docker
// +build !kubelet

package listeners

// DockerKubeletService overrides some methods when a container is
// running in kubernetes. The overrides are defined in docker_kubelet.go
// when the kubelet build tag is provided.
type DockerKubeletService struct {
	DockerService
}
