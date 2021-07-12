// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2019 Datadog, Inc.

// +build docker

package hostname

import (
	"github.com/StackVista/stackstate-agent/pkg/config"
	"github.com/StackVista/stackstate-agent/pkg/util/kubernetes/clustername"
)

func init() {
	if config.IsDockerSwarm() == true {
		RegisterHostnameProvider("dockerswarm", func() (string, error) {
			return clustername.GetClusterName(), nil
		})
	}
}
