// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// +build kubelet

package kubelet

import (
	"fmt"

	"github.com/DataDog/datadog-agent/pkg/util/hostname/validate"
)

// GetHostAlias uses the "kubelet" hostname provider to fetch the kubernetes alias
func GetHostAlias() (string, error) {
	name, err := HostnameProvider()
	if err == nil && validate.ValidHostname(name) == nil {
		return name, nil
	}
	return "", fmt.Errorf("Couldn't extract a host alias from the kubelet: %s", err)
}
