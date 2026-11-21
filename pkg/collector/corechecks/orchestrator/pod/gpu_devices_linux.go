// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubelet && kubeapiserver && linux

package pod

import (
	model "github.com/DataDog/agent-payload/v5/process"

	"github.com/DataDog/datadog-agent/pkg/util/gpu"
	"github.com/DataDog/datadog-agent/pkg/util/kernel"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// collectGPUDevices returns the node's GPUs as listed by the NVIDIA kernel
// driver in procfs. Pod status reports index-based device IDs (nvidia0,
// nvidia0/vgpu3) for device plugins such as GKE's; this list lets the backend
// resolve them to GPU UUIDs without NVML or the PodResources socket.
func collectGPUDevices() []*model.NodeGPUDevice {
	gpus, err := gpu.ReadDriverGPUs(kernel.ProcFSRoot())
	if err != nil {
		log.Debugf("Unable to read some GPUs from the NVIDIA driver: %s", err)
	}
	if len(gpus) == 0 {
		return nil
	}

	devices := make([]*model.NodeGPUDevice, 0, len(gpus))
	for _, g := range gpus {
		devices = append(devices, &model.NodeGPUDevice{
			Minor:    int32(g.Minor),
			Uuid:     g.UUID,
			Model:    g.Model,
			PciBusId: g.BusLocation,
		})
	}
	return devices
}
