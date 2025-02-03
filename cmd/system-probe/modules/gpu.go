// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

//go:build linux

package modules

import (
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"time"

	"github.com/NVIDIA/go-nvml/pkg/nvml"
	"go.uber.org/atomic"

	"github.com/DataDog/datadog-agent/cmd/system-probe/api/module"
	"github.com/DataDog/datadog-agent/cmd/system-probe/config"
	sysconfigtypes "github.com/DataDog/datadog-agent/cmd/system-probe/config/types"
	"github.com/DataDog/datadog-agent/cmd/system-probe/utils"
	"github.com/DataDog/datadog-agent/pkg/config/env"
	"github.com/DataDog/datadog-agent/pkg/ebpf/uprobes"
	"github.com/DataDog/datadog-agent/pkg/eventmonitor"
	"github.com/DataDog/datadog-agent/pkg/eventmonitor/consumers"
	"github.com/DataDog/datadog-agent/pkg/gpu"
	gpuconfig "github.com/DataDog/datadog-agent/pkg/gpu/config"
	"github.com/DataDog/datadog-agent/pkg/util/kernel"
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
			configureCgroupPermissions()
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

var agentProcessRegexp = regexp.MustCompile("datadog-agent/.*/agent")

func getAgentPID(procRoot string) (uint32, error) {
	pids, err := kernel.AllPidsProcs(procRoot)
	if err != nil {
		return 0, fmt.Errorf("failed to get all pids: %w", err)
	}

	for _, pid := range pids {
		proc := uprobes.NewProcInfo(procRoot, uint32(pid))
		exe, err := proc.Exe()
		if err != nil {
			// Ignore this process, we don't want to stop the search because of that
			continue
		}

		if agentProcessRegexp.MatchString(exe) {
			return uint32(pid), nil
		}
	}

	return 0, errors.New("agent process not found")
}

// configureCgroupPermissions configures the cgroup permissions to access NVIDIA
// devices for the system-probe and agent processes, as the NVIDIA device plugin
// sets them in a way that can be overwritten by SystemD cgroups.
func configureCgroupPermissions() {
	root := hostRoot()

	sysprobePID := uint32(os.Getpid())
	log.Infof("Configuring cgroup permissions for system-probe process with PID %d", sysprobePID)
	if err := gpu.ConfigureDeviceCgroups(sysprobePID, root); err != nil {
		log.Warnf("Failed to configure cgroup permissions for system-probe process: %v. gpu-monitoring module might not work properly", err)
	}

	procRoot := filepath.Join(root, "proc")
	agentPID, err := getAgentPID(procRoot)
	if err != nil {
		log.Warnf("Failed to get agent PID: %v. Cannot patch cgroup permissions, gpu-monitoring module might not work properly", err)
		return
	}

	log.Infof("Configuring cgroup permissions for agent process with PID %d", agentPID)
	if err := gpu.ConfigureDeviceCgroups(agentPID, root); err != nil {
		log.Warnf("Failed to configure cgroup permissions for agent process: %v. gpu-monitoring module might not work properly", err)
	}
}
