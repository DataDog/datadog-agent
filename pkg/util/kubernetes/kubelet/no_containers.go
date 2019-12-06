// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2019 Datadog, Inc.

// +build kubelet,!linux

package kubelet

import (
	"errors"

	"github.com/DataDog/datadog-agent/pkg/util/containers"
)

var (
	// ErrLinuxNotCompiled is returned if linux build tag is not defined.
	ErrLinuxNotCompiled = errors.New("linux build tag not defined")
)

// ListContainers lists all non-excluded running containers, and retrieves their performance metrics
func (ku *KubeUtil) ListContainers() ([]*containers.Container, error) {
	return nil, ErrLinuxNotCompiled
}

// UpdateContainerMetrics updates cgroup / network performance metrics for
// a provided list of Container objects
func (ku *KubeUtil) UpdateContainerMetrics(ctrList []*containers.Container) error {
	return ErrLinuxNotCompiled
}
