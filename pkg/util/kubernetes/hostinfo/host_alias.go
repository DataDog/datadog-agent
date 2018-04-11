// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018 Datadog, Inc.

// +build kubelet,kubeapiserver

package hostinfo

import (
	"fmt"

	"github.com/DataDog/datadog-agent/pkg/util"
	"github.com/DataDog/datadog-agent/pkg/util/hostname"
)

// GetHostAlias uses the "kubelet" hostname provider to fetch the kubernetes alias
func GetHostAlias() (string, error) {
	if getKubeletHostname, found := hostname.ProviderCatalog["kubelet"]; found {
		name, err := getKubeletHostname("")
		if err == nil && util.ValidHostname(name) == nil {
			return name, nil
		}
	}
	return "", fmt.Errorf("Couldn't extract a host alias from the kubelet")
}
