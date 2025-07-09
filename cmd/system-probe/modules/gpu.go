// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

//go:build linux && linux_bpf && nvml

package modules

import (
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strconv"

	"github.com/DataDog/datadog-agent/pkg/config/env"
	"github.com/DataDog/datadog-agent/pkg/ebpf/uprobes"
	"github.com/DataDog/datadog-agent/pkg/eventmonitor"
	"github.com/DataDog/datadog-agent/pkg/eventmonitor/consumers"
	"github.com/DataDog/datadog-agent/pkg/gpu"
	gpuconfig "github.com/DataDog/datadog-agent/pkg/gpu/config"
	gpuconfigconsts "github.com/DataDog/datadog-agent/pkg/gpu/config/consts"
	usm "github.com/DataDog/datadog-agent/pkg/network/usm/utils"
	"github.com/DataDog/datadog-agent/pkg/system-probe/api/module"
	"github.com/DataDog/datadog-agent/pkg/system-probe/config"
	sysconfigtypes "github.com/DataDog/datadog-agent/pkg/system-probe/config/types"
	"github.com/DataDog/datadog-agent/pkg/system-probe/utils"
	"github.com/DataDog/datadog-agent/pkg/util/kernel"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

func init() { registerModule(GPUMonitoring) }

var _ module.Module = &GPUMonitoringModule{}
var gpuMonitoringConfigNamespaces = []string{gpuconfigconsts.GPUNS}

// processEventConsumer is a global variable that holds the process event consumer, created in the eventmonitor module
// Note: In the future we should have a better way to handle dependencies between modules
var processEventConsumer *consumers.ProcessConsumer

const processConsumerID = "gpu"
const processConsumerChanSize = 100

const defaultCollectedDebugEvents = 100
const maxCollectedDebugEvents = 1000000

var processConsumerEventTypes = []consumers.ProcessConsumerEventTypes{consumers.ExecEventType, consumers.ExitEventType}

// GPUMonitoring Factory
var GPUMonitoring = &module.Factory{
	Name:             config.GPUMonitoringModule,
	ConfigNamespaces: gpuMonitoringConfigNamespaces,
	Fn: func(_ *sysconfigtypes.Config, deps module.FactoryDependencies) (module.Module, error) {
		if processEventConsumer == nil {
			return nil, errors.New("process event consumer not initialized")
		}

		c := gpuconfig.New()

		if c.ConfigureCgroupPerms {
			configureCgroupPermissions()
		}

		probeDeps := gpu.ProbeDependencies{
			Telemetry:      deps.Telemetry,
			ProcessMonitor: processEventConsumer,
			WorkloadMeta:   deps.WMeta,
		}
		p, err := gpu.NewProbe(c, probeDeps)
		if err != nil {
			return nil, fmt.Errorf("unable to start %s: %w", config.GPUMonitoringModule, err)
		}

		return &GPUMonitoringModule{
			Probe: p,
		}, nil
	},
	NeedsEBPF: func() bool {
		return true
	},
}

// GPUMonitoringModule is a module for GPU monitoring
type GPUMonitoringModule struct {
	*gpu.Probe
}

// Register registers the GPU monitoring module
func (t *GPUMonitoringModule) Register(httpMux *module.Router) error {
	httpMux.HandleFunc("/check", func(w http.ResponseWriter, _ *http.Request) {
		stats, err := t.Probe.GetAndFlush()
		if err != nil {
			log.Errorf("Error getting GPU stats: %v", err)
			w.WriteHeader(500)
			return
		}

		utils.WriteAsJSON(w, stats, utils.CompactOutput)
	})

	httpMux.HandleFunc("/debug/traced-programs", usm.GetTracedProgramsEndpoint(gpuconfigconsts.GpuModuleName))
	httpMux.HandleFunc("/debug/blocked-processes", usm.GetBlockedPathIDEndpoint(gpuconfigconsts.GpuModuleName))
	httpMux.HandleFunc("/debug/clear-blocked", usm.GetClearBlockedEndpoint(gpuconfigconsts.GpuModuleName))
	httpMux.HandleFunc("/debug/attach-pid", usm.GetAttachPIDEndpoint(gpuconfigconsts.GpuModuleName))
	httpMux.HandleFunc("/debug/detach-pid", usm.GetDetachPIDEndpoint(gpuconfigconsts.GpuModuleName))
	httpMux.HandleFunc("/debug/collect-events", t.collectEventsHandler)

	return nil
}

// GetStats returns the debug stats for the GPU monitoring module
func (t *GPUMonitoringModule) GetStats() map[string]interface{} {
	return t.Probe.GetDebugStats()
}

func (t *GPUMonitoringModule) collectEventsHandler(w http.ResponseWriter, r *http.Request) {
	count := defaultCollectedDebugEvents

	countStr := r.URL.Query().Get("count")
	if countStr != "" {
		var err error
		count, err = strconv.Atoi(countStr)
		if err != nil {
			w.Write([]byte(fmt.Sprintf("Invalid count: %s", countStr)))
			w.WriteHeader(http.StatusBadRequest)
			return
		}
	}

	if count > maxCollectedDebugEvents {
		log.Warnf("Count %d is too high, clamping to %d", count, maxCollectedDebugEvents)
		count = maxCollectedDebugEvents
	}

	log.Infof("Received request to collect %d GPU events, collecting...", count)

	data, err := t.Probe.CollectConsumedEvents(r.Context(), count)
	if err != nil {
		msg := fmt.Sprintf("Error collecting GPU events: %v", err)
		log.Warn(msg)
		w.Write([]byte(msg))
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	log.Info("Collection finished, writing response...")

	for _, row := range data {
		w.Write(row)
		w.Write([]byte("\n"))
	}

	w.WriteHeader(http.StatusOK)
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
