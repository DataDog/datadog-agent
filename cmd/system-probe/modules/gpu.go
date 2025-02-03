// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

//go:build linux

package modules

import (
	"fmt"
	"net/http"
	"os"
	"time"

	"github.com/NVIDIA/go-nvml/pkg/nvml"
	"go.uber.org/atomic"

	"github.com/DataDog/datadog-agent/cmd/system-probe/api/module"
	"github.com/DataDog/datadog-agent/cmd/system-probe/config"
	sysconfigtypes "github.com/DataDog/datadog-agent/cmd/system-probe/config/types"
	"github.com/DataDog/datadog-agent/cmd/system-probe/utils"
	"github.com/DataDog/datadog-agent/pkg/config/env"
	"github.com/DataDog/datadog-agent/pkg/eventmonitor"
	"github.com/DataDog/datadog-agent/pkg/eventmonitor/consumers"
	"github.com/DataDog/datadog-agent/pkg/gpu"
	gpuconfig "github.com/DataDog/datadog-agent/pkg/gpu/config"
	usm "github.com/DataDog/datadog-agent/pkg/network/usm/utils"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

var _ module.Module = &GPUMonitoringModule{}
var gpuMonitoringConfigNamespaces = []string{gpuconfig.GPUNS}

// processEventConsumer is a global variable that holds the process event consumer, created in the eventmonitor module
// Note: In the future we should have a better way to handle dependencies between modules
var processEventConsumer *consumers.ProcessConsumer

const processConsumerID = "gpu"
const processConsumerChanSize = 100

var processConsumerEventTypes = []consumers.ProcessConsumerEventTypes{consumers.ExecEventType, consumers.ExitEventType}

// GPUMonitoring Factory
var GPUMonitoring = module.Factory{
	Name:             config.GPUMonitoringModule,
	ConfigNamespaces: gpuMonitoringConfigNamespaces,
	Fn: func(_ *sysconfigtypes.Config, deps module.FactoryDependencies) (module.Module, error) {

		if processEventConsumer == nil {
			return nil, fmt.Errorf("process event consumer not initialized")
		}

		c := gpuconfig.New()

		if c.ConfigureCgroupPerms {
			log.Info("Configuring GPU device cgroup permissions for system-probe")
			err := gpu.ConfigureDeviceCgroups(uint32(os.Getpid()), hostRoot())
			if err != nil {
				log.Warnf("Failed to configure device cgroups for process: %v, gpu-monitoring module might not work properly", err)
			}
		}

		probeDeps := gpu.ProbeDependencies{
			Telemetry: deps.Telemetry,
			//if the config parameter doesn't exist or is empty string, the default value is used as defined in go-nvml library
			//(https://github.com/NVIDIA/go-nvml/blob/main/pkg/nvml/lib.go#L30)
			NvmlLib:        nvml.New(nvml.WithLibraryPath(c.NVMLLibraryPath)),
			ProcessMonitor: processEventConsumer,
			WorkloadMeta:   deps.WMeta,
		}

		ret := probeDeps.NvmlLib.Init()
		if ret != nvml.SUCCESS && ret != nvml.ERROR_ALREADY_INITIALIZED {
			return nil, fmt.Errorf("unable to initialize NVML library: %v", ret)
		}

		p, err := gpu.NewProbe(c, probeDeps)
		if err != nil {
			return nil, fmt.Errorf("unable to start %s: %w", config.GPUMonitoringModule, err)
		}

		return &GPUMonitoringModule{
			Probe:     p,
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

	httpMux.HandleFunc("/debug/traced-programs", usm.GetTracedProgramsEndpoint(gpu.GpuModuleName))
	httpMux.HandleFunc("/debug/blocked-processes", usm.GetBlockedPathIDEndpoint(gpu.GpuModuleName))
	httpMux.HandleFunc("/debug/clear-blocked", usm.GetClearBlockedEndpoint(gpu.GpuModuleName))
	httpMux.HandleFunc("/debug/attach-pid", usm.GetAttachPIDEndpoint(gpu.GpuModuleName))
	httpMux.HandleFunc("/debug/detach-pid", usm.GetDetachPIDEndpoint(gpu.GpuModuleName))

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

// createGPUProcessEventConsumer creates the process event consumer for the GPU module. Should be called from the event monitor module
func createGPUProcessEventConsumer(evm *eventmonitor.EventMonitor) error {
	var err error
	processEventConsumer, err = consumers.NewProcessConsumer(processConsumerID, processConsumerChanSize, processConsumerEventTypes, evm)
	if err != nil {
		return err
	}

	return nil
}

func hostRoot() string {
	envHostRoot := os.Getenv("HOST_ROOT")
	if envHostRoot != "" {
		return envHostRoot
	}

	if env.IsContainerized() {
		return "/host"
	}

	return "/"
}
