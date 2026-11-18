// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubelet && kubeapiserver && !linux

package pod

import model "github.com/DataDog/agent-payload/v5/process"

// collectGPUDevices returns nothing: the NVIDIA driver's procfs files only exist on Linux.
func collectGPUDevices() []*model.NodeGPUDevice {
	return nil
}
