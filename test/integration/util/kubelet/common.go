// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018 Datadog, Inc.

// +build kubelet

package kubernetes

import (
	"github.com/DataDog/datadog-agent/test/integration/utils"
)

const emptyPodList = `{"kind":"PodList","apiVersion":"v1","metadata":{},"items":null}
`

// initOpenKubelet create a standalone kubelet open to http and https calls
func initOpenKubelet() (*utils.ComposeConf, error) {
	networkMode, err := utils.GetNetworkMode()
	if err != nil {
		return nil, err
	}

	compose := &utils.ComposeConf{
		ProjectName: "kubelet_kubeutil",
		FilePath:    "testdata/open-kubelet-compose.yaml",
		Variables:   map[string]string{"network_mode": networkMode},
	}
	return compose, nil
}
