// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2019 Datadog, Inc.

// +build docker,!kubelet

package listeners

// DockerKubeletService is not compiled if the kubelet tag is not here.
// Revert to all DockerService methods, that might probably fail though.
type DockerKubeletService struct {
	DockerService
}
