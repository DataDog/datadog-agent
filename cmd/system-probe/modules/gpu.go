// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

//go:build linux

package modules

import (
	"fmt"
	"net/http"
	"time"

	"github.com/NVIDIA/go-nvml/pkg/nvml"
	"go.uber.org/atomic"

	"github.com/DataDog/datadog-agent/cmd/system-probe/api/module"
	"github.com/DataDog/datadog-agent/cmd/system-probe/config"
	sysconfigtypes "github.com/DataDog/datadog-agent/cmd/system-probe/config/types"
	"github.com/DataDog/datadog-agent/cmd/system-probe/utils"
	"github.com/DataDog/datadog-agent/pkg/gpu"
	gpuconfig "github.com/DataDog/datadog-agent/pkg/gpu/config"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

var _ module.Module = &GPUMonitoringModule{}
var gpuMonitoringConfigNamespaces = []string{gpuconfig.GPUNS}

// GPUMonitoring Factory
var GPUMonitoring = module.Factory{
	Name:             config.GPUMonitoringModule,
	ConfigNamespaces: gpuMonitoringConfigNamespaces,
	Fn: func(_ *sysconfigtypes.Config, deps module.FactoryDependencies) (module.Module, error) {

		c := gpuconfig.NewConfig()
		probeDeps := gpu.ProbeDependencies{
			Telemetry: deps.Telemetry,
			//if the config parameter doesn't exist or is empty string, the default value is used as defined in go-nvml library
			//(https://github.com/NVIDIA/go-nvml/blob/main/pkg/nvml/lib.go#L30)
			NvmlLib: nvml.New(nvml.WithLibraryPath(c.NVMLLibraryPath)),
		}

		ret := probeDeps.NvmlLib.Init()
		if ret != nvml.SUCCESS && ret != nvml.ERROR_ALREADY_INITIALIZED {
			return nil, fmt.Errorf("unable to initialize NVML library: %v", ret)
		}

		t, err := gpu.NewProbe(c, probeDeps)
		if err != nil {
			return nil, fmt.Errorf("unable to start GPU monitoring: %w", err)
		}

		return &GPUMonitoringModule{
			Probe:     t,
			lastCheck: atomic.NewInt64(0),
		}, nil
	},
	NeedsEBPF: func() bool {
		return true
	},
}

// GPUMonitoringModule is a module for GPU monitoring
type GPUMonitoringModule struct {
	*gpu.Probe
	lastCheck *atomic.Int64
}

// Register registers the GPU monitoring module
func (t *GPUMonitoringModule) Register(httpMux *module.Router) error {
	httpMux.HandleFunc("/check", func(w http.ResponseWriter, _ *http.Request) {
		t.lastCheck.Store(time.Now().Unix())
		stats, err := t.Probe.GetAndFlush()
		if err != nil {
			log.Errorf("Error getting GPU stats: %v", err)
			w.WriteHeader(500)
			return
		}

		utils.WriteAsJSON(w, stats)
	})

	return nil
}

// GetStats returns the last check time
func (t *GPUMonitoringModule) GetStats() map[string]interface{} {
	return map[string]interface{}{
		"last_check": t.lastCheck.Load(),
	}
}

// Close closes the GPU monitoring module
func (t *GPUMonitoringModule) Close() {
	t.Probe.Close()
}
