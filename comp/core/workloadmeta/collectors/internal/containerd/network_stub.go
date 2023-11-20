// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build containerd && !linux

package containerd

import (
	"errors"

	"github.com/containerd/containerd"

	cutil "github.com/DataDog/datadog-agent/pkg/util/containerd"
)

func extractIP(namespace string, container containerd.Container, containerdClient cutil.ContainerdItf) (string, error) {
	return "", errors.New("can't get the IPs on this OS")
}
