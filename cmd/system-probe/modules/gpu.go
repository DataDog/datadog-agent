// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

//go:build linux && bpf && nvml

package modules

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/NVIDIA/go-nvml/pkg/nvml"

	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/gpu/model"
	"github.com/DataDog/datadog-agent/pkg/config/env"
	"github.com/DataDog/datadog-agent/pkg/ebpf"
	"github.com/DataDog/datadog-agent/pkg/ebpf/uprobes"
	"github.com/DataDog/datadog-agent/pkg/eventmonitor"
	"github.com/DataDog/datadog-agent/pkg/eventmonitor/consumers"
	"github.com/DataDog/datadog-agent/pkg/gpu"
	gpuconfig "github.com/DataDog/datadog-agent/pkg/gpu/config"
	gpuconfigconsts "github.com/DataDog/datadog-agent/pkg/gpu/config/consts"
	"github.com/DataDog/datadog-agent/pkg/gpu/prm"
	ddnvml "github.com/DataDog/datadog-agent/pkg/gpu/safenvml"
	usm "github.com/DataDog/datadog-agent/pkg/network/usm/utils"
	"github.com/DataDog/datadog-agent/pkg/system-probe/api/module"
	sysconfigtypes "github.com/DataDog/datadog-agent/pkg/system-probe/config/types"
	"github.com/DataDog/datadog-agent/pkg/system-probe/utils"
	"github.com/DataDog/datadog-agent/pkg/util/kernel"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

func init() { registerModule(GPUMonitoring) }

var _ module.Module = &GPUMonitoringModule{}

// processEventConsumer is a global variable that holds the process event consumer, created in the eventmonitor module
// Note: In the future we should have a better way to handle dependencies between modules
var processEventConsumer *consumers.ProcessConsumer

const processConsumerID = "gpu"
const processConsumerChanSize = 100

const defaultCollectedDebugEvents = 100
const maxCollectedDebugEvents = 1000000
const driverEventQueueSize = 100

var processConsumerEventTypes = []consumers.ProcessConsumerEventTypes{consumers.ExecEventType, consumers.ExitEventType}

// GPUMonitoring Factory
var GPUMonitoring = &module.Factory{
	Name: sysconfigtypes.GPUMonitoringModule,
	Fn: func(_ *sysconfigtypes.Config, deps module.FactoryDependencies) (module.Module, error) {
		c := gpuconfig.New()
		if c.EnableEBPFProbes && processEventConsumer == nil {
			return nil, errors.New("process event consumer not initialized")
		}

		ctx, cancel := context.WithCancel(context.Background())

		if c.ConfigureCgroupPerms {
			configureCgroupPermissions(ctx, c.CgroupReapplyInterval, c.CgroupReapplyInfinitely)
		}

		deviceCache := ddnvml.NewDeviceCache() // note: deviceCache constructor does not allocate resources/do NVML calls
		var deviceCacheRefreshErr error
		if c.EnableEBPFProbes || c.DriverEventsEnabled {
			deviceCacheRefreshErr = deviceCache.Refresh()
			if deviceCacheRefreshErr != nil {
				log.Errorf("unable to refresh GPU device cache: %v", deviceCacheRefreshErr)
			} else {
				go refreshDeviceCache(ctx, deviceCache, c.DeviceCacheRefreshInterval)
			}
		}

		var p *gpu.Probe
		var driverEventSubscriber driverEventSubscriber
		var err error
		if c.EnableEBPFProbes {
			ebpfConfig := ebpf.NewConfig()
			probeDeps := gpu.ProbeDependencies{
				EBPFConfig:     ebpfConfig,
				Telemetry:      deps.Telemetry,
				ProcessMonitor: processEventConsumer,
				WorkloadMeta:   deps.WMeta,
				DeviceCache:    deviceCache,
			}
			p, err = gpu.NewProbe(c, probeDeps)
			if err != nil {
				cancel()
				return nil, fmt.Errorf("unable to start %s: %w", sysconfigtypes.GPUMonitoringModule, err)
			}
		}

		if c.DriverEventsEnabled && deviceCacheRefreshErr == nil {
			subscriber, err := gpu.NewDriverEventSubscriber(deps.Telemetry, deviceCache, gpu.DriverEventSubscriberConfig{
				QueueSize: driverEventQueueSize,
			})
			if err != nil {
				log.Errorf("unable to start GPU driver event subscriber: %v", err)
			} else {
				driverEventSubscriber = subscriber
			}
		} else if c.DriverEventsEnabled {
			log.Errorf("unable to start GPU driver event subscriber due to device cache refresh error %v", deviceCacheRefreshErr)
		}

		mod := &GPUMonitoringModule{
			Probe:                 p,
			driverEventSubscriber: driverEventSubscriber,
			prmHandler: prm.NewHandler(func(uuid string) (prm.Device, error) {
				// Gate the device access: the release monitor can shut NVML
				// down concurrently with an active PRM request.
				if err := ddnvml.BeginNVMLUse(); err != nil {
					return nil, err
				}
				defer ddnvml.EndNVMLUse()
				return deviceCache.GetByUUID(uuid)
			}),
			cfg:           c,
			contextCancel: cancel,
			context:       ctx,
			deviceCache:   deviceCache,
			leaseDone:     make(chan struct{}),
			migGrSamples:  map[string]migGrSample{},
		}
		// The release monitor lives on the module (not the probe) and runs
		// in every mode: PRM requests, driver events and the eBPF probe all
		// hold NVML, so all of them must participate in release windows.
		mod.startNvmlReleaseMonitor()
		return mod, nil
	},
	NeedsEBPF: func() bool {
		return gpuconfig.New().EnableEBPFProbes
	},
}

// GPUMonitoringModule is a module for GPU monitoring
type GPUMonitoringModule struct {
	*gpu.Probe
	driverEventSubscriber driverEventSubscriber
	prmHandler            *prm.Handler
	cfg                   *gpuconfig.Config
	context               context.Context    // Context associated with the module
	contextCancel         context.CancelFunc // Cancel function associated with the context
	deviceCache           ddnvml.DeviceCache // deviceCache is the module's cache in every mode (the probe's in eBPF mode)

	// nvmlLease is the release lease held by the core agent over the
	// /nvml-release endpoint. It lives on the module — not the probe — so
	// driver-events-only mode (eBPF probes disabled, NVML still held by the
	// subscriber) participates in release windows too.
	nvmlLease gpu.NvmlReleaseLease
	leaseDone chan struct{}
	leaseWG   sync.WaitGroup

	// migGrSamples holds the previous GPM sample per MIG device UUID for /mig-gr
	migGrMu          sync.Mutex
	migGrSamples     map[string]migGrSample
	migGrLastRefresh time.Time
}

type migGrSample struct {
	sample nvml.GpmSample
	at     time.Time
}

const (
	// migGrMinWindow keeps a close second request (e.g. a manual check) from replacing the baseline with a tiny window
	migGrMinWindow = time.Second
	// migGrMaxAge treats an older baseline (agent restart, long outage) as missing rather than averaging over the gap.
	// It is well above the usual check intervals, so that a longer min_collection_interval still gets values.
	migGrMaxAge = 10 * time.Minute
	// migGrRefreshInterval limits the device cache refreshes triggered by unknown UUIDs
	migGrRefreshInterval = 30 * time.Second
)

// dropMigGrSamples forgets the stored samples, freeing them when NVML is still usable (before a release, at close).
func (t *GPUMonitoringModule) dropMigGrSamples(free bool) {
	t.migGrMu.Lock()
	defer t.migGrMu.Unlock()
	if len(t.migGrSamples) == 0 {
		// Nothing to free: don't initialize NVML just as it is being released
		return
	}
	if free {
		if lib, err := ddnvml.GetSafeNvmlLib(); err == nil {
			for _, s := range t.migGrSamples {
				_ = lib.GpmSampleFree(s.sample)
			}
		}
	}
	clear(t.migGrSamples)
}

type driverEventSubscriber interface {
	GetAndFlush() ([]model.DriverEvent, error)
	Stop()
}

// Register registers the GPU monitoring module
func (t *GPUMonitoringModule) Register(httpMux *module.Router) error {
	// Data sources disabled in the configuration answer 501 and sources that
	// are enabled but not working answer 503, so that the Agent can tell both
	// apart from a 404, which means the module is not loaded.
	//
	// Ensure only one concurrent check is allowed, as the GetAndFlush method is not thread safe.
	httpMux.HandleFunc("/check", utils.WithConcurrencyLimit(1, func(w http.ResponseWriter, req *http.Request) {
		if t.Probe == nil {
			http.Error(w, "GPU eBPF probes are disabled", http.StatusNotImplemented)
			return
		}

		stats, err := t.Probe.GetAndFlush()
		if err != nil {
			log.Errorf("Error getting GPU stats: %v", err)
			w.WriteHeader(500)
			return
		}

		utils.WriteAsJSON(req, w, stats, utils.CompactOutput)
	}))

	httpMux.HandleFunc("/driver-events", utils.WithConcurrencyLimit(1, func(w http.ResponseWriter, req *http.Request) {
		if !t.cfg.DriverEventsEnabled {
			http.Error(w, "GPU driver events are disabled", http.StatusNotImplemented)
			return
		}
		if t.driverEventSubscriber == nil {
			http.Error(w, "GPU driver event subscriber failed to start, check the system-probe logs", http.StatusServiceUnavailable)
			return
		}
		events, err := t.driverEventSubscriber.GetAndFlush()
		if err != nil {
			log.Errorf("Error getting GPU driver events: %v", err)
			http.Error(w, "GPU driver event subscriber stopped", http.StatusServiceUnavailable)
			return
		}
		utils.WriteAsJSON(req, w, events, utils.CompactOutput)
	}))

	httpMux.HandleFunc("/prm-metrics", utils.WithConcurrencyLimit(1, func(w http.ResponseWriter, req *http.Request) {
		if t.cfg == nil || !t.cfg.PRMEndpointEnabled || t.prmHandler == nil {
			http.Error(w, "GPU PRM endpoint is disabled", http.StatusNotImplemented)
			return
		}
		// Gate the whole PRM operation: the handler performs device calls
		// (architecture, port counters) after the device lookup, and the
		// release monitor must not shut NVML down mid-request.
		if err := ddnvml.BeginNVMLUse(); err != nil {
			http.Error(w, fmt.Sprintf("NVML unavailable (release window active): %v", err), http.StatusServiceUnavailable)
			return
		}
		defer ddnvml.EndNVMLUse()
		t.prmHandler.HandlePRMMetrics(w, req)
	}))

	httpMux.HandleFunc("/mig-gr", t.migGrHandler)

	httpMux.HandleFunc("/debug/traced-programs", usm.GetTracedProgramsEndpoint(gpuconfigconsts.GpuModuleName))
	httpMux.HandleFunc("/debug/blocked-processes", usm.GetBlockedPathIDEndpoint(gpuconfigconsts.GpuModuleName))
	httpMux.HandleFunc("/debug/clear-blocked", usm.GetClearBlockedEndpoint(gpuconfigconsts.GpuModuleName))
	httpMux.HandleFunc("/debug/attach-pid", usm.GetAttachPIDEndpoint(gpuconfigconsts.GpuModuleName))
	httpMux.HandleFunc("/debug/detach-pid", usm.GetDetachPIDEndpoint(gpuconfigconsts.GpuModuleName))
	httpMux.HandleFunc("/debug/collect-events", t.collectEventsHandler)

	// The core agent holds a lease on this probe's NVML release through
	// this endpoint, so a GPU reset is not blocked by system-probe either.
	httpMux.HandleFunc("/nvml-release", t.nvmlReleaseHandler)

	return nil
}

// startNvmlReleaseMonitor follows the release lease: while the core agent
// holds it (a GPU reset window is open) the module releases its NVML; when it
// clears or expires, NVML is re-acquired and the caches re-enumerate lazily.
func (t *GPUMonitoringModule) startNvmlReleaseMonitor() {
	t.leaseWG.Add(1)
	go func() {
		defer t.leaseWG.Done()

		ticker := time.NewTicker(5 * time.Second)
		defer ticker.Stop()

		for {
			select {
			case <-t.leaseDone:
				return
			case <-ticker.C:
				window := t.nvmlLease.Held()
				released := ddnvml.IsNVMLReleased()
				switch {
				case window && !released:
					t.releaseNVMLForLease()
				case !window && released:
					t.reacquireNVMLForLease()
				}
			}
		}
	}()
}

// releaseNVMLForLease dispatches the release on the mode: eBPF mode goes
// through the probe's system context (device cache + per-process caches +
// library); driver-events-only mode releases the library directly. The device
// cache is NOT dropped here: the driver-event consumer keeps running during
// the window, and an empty cache would leave events without a mapping, so
// they would be discarded. The pre-reset mapping is kept instead (best-effort
// attribution for the events in flight) and invalidated at reacquire time.
func (t *GPUMonitoringModule) releaseNVMLForLease() {
	t.dropMigGrSamples(true)
	if t.Probe != nil {
		t.Probe.ReleaseForNvmlLease()
		return
	}
	// Driver-events-only mode: no system context — release NVML directly.
	if err := ddnvml.ReleaseNVML(); err != nil {
		log.Warnf("error shutting down NVML for the release in the GPU monitoring module (will retry next tick): %v", err)
		return
	}
	log.Warnf("NVML release window active (GPU reset in progress); GPU monitoring module releasing NVML until it completes")
}

// reacquireNVMLForLease ends the release window: NVML is re-initialized and
// the device caches are dropped so the next use re-enumerates the (possibly
// changed) device layout. The cache refresh loop re-populates the
// driver-events-only cache; eBPF mode re-enumerates lazily on next use.
func (t *GPUMonitoringModule) reacquireNVMLForLease() {
	// samples stored between the release and here belong to the previous NVML session: drop them without freeing
	t.dropMigGrSamples(false)
	if t.Probe != nil {
		t.Probe.ReacquireForNvmlLease()
		return
	}
	// Invalidate while new users are still rejected, clear the gate last (see
	// systemContext.reacquireNVML for why the order matters).
	t.deviceCache.Invalidate()
	ddnvml.ReacquireNVML()
}

// nvmlReleaseHandler receives the core agent's NVML release push: a push
// with released=true renews the release lease, released=false ends the
// window. If the core agent stops renewing (crash, check reload), the lease
// simply expires.
func (t *GPUMonitoringModule) nvmlReleaseHandler(w http.ResponseWriter, r *http.Request) {
	var req model.NvmlReleaseRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}

	if req.Released == model.NvmlStateReleased {
		t.nvmlLease.Hold(time.Duration(req.TTLSeconds) * time.Second)
	} else {
		t.nvmlLease.Clear()
	}

	// The client helper always JSON-unmarshals the response body; an empty
	// body would surface as an error on every successful renewal.
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte("{}"))
}

// migGrHandler serves GET /mig-gr?uuid=<MIG device UUID>: the GPM GRAPHICS_UTIL of a
// MIG device between this request and the previous one for the same UUID. On MIG,
// GPM reads need CAP_SYS_ADMIN (or the MIG config capability), which system-probe
// may hold but the core agent does not. Without a usable previous sample (first
// request, too close, too old) it stores one and answers 503.
func (t *GPUMonitoringModule) migGrHandler(w http.ResponseWriter, req *http.Request) {
	if t.cfg == nil || !t.cfg.MigGrEndpointEnabled {
		http.Error(w, "GPU MIG gr_engine_active endpoint is disabled", http.StatusNotImplemented)
		return
	}
	uuid := req.URL.Query().Get("uuid")
	if req.Method != http.MethodGet || uuid == "" {
		http.Error(w, "expected GET with a uuid parameter", http.StatusBadRequest)
		return
	}
	// Queue on the module lock before holding NVML, and skip requests whose caller gave up meanwhile
	t.migGrMu.Lock()
	defer t.migGrMu.Unlock()
	if req.Context().Err() != nil {
		return
	}
	if err := ddnvml.BeginNVMLUse(); err != nil {
		http.Error(w, fmt.Sprintf("NVML unavailable (release window active): %v", err), http.StatusServiceUnavailable)
		return
	}
	defer ddnvml.EndNVMLUse()

	dev, err := t.deviceCache.GetByUUID(uuid)
	if err != nil && time.Since(t.migGrLastRefresh) >= migGrRefreshInterval {
		t.migGrLastRefresh = time.Now()
		if refreshErr := t.deviceCache.Refresh(); refreshErr != nil {
			http.Error(w, fmt.Sprintf("cannot list GPU devices: %v", refreshErr), http.StatusServiceUnavailable)
			return
		}
		dev, err = t.deviceCache.GetByUUID(uuid)
	}
	mig, ok := dev.(*ddnvml.MIGDevice)
	if err != nil || !ok || mig.Parent == nil {
		http.Error(w, fmt.Sprintf("MIG device %q not found", uuid), http.StatusNotFound)
		return
	}
	lib, err := ddnvml.GetSafeNvmlLib()
	if err != nil {
		http.Error(w, err.Error(), http.StatusServiceUnavailable)
		return
	}

	now := time.Now()
	prev, hasPrev := t.migGrSamples[uuid]
	if hasPrev && now.Sub(prev.at) < migGrMinWindow {
		http.Error(w, "previous GPM sample too recent", http.StatusServiceUnavailable)
		return
	}
	sample, err := lib.GpmSampleAlloc()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if hasPrev {
		delete(t.migGrSamples, uuid)
		defer func() { _ = lib.GpmSampleFree(prev.sample) }()
	}
	if err := mig.Parent.GpmMigSampleGet(mig.MIGInstanceID, sample); err != nil {
		_ = lib.GpmSampleFree(sample)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	t.migGrSamples[uuid] = migGrSample{sample: sample, at: now}
	if !hasPrev || now.Sub(prev.at) > migGrMaxAge {
		http.Error(w, "GPM baseline stored, no value yet", http.StatusServiceUnavailable)
		return
	}

	get := &nvml.GpmMetricsGetType{NumMetrics: 1, Version: nvml.GPM_METRICS_GET_VERSION, Sample1: prev.sample, Sample2: sample}
	get.Metrics[0] = nvml.GpmMetric{MetricId: uint32(nvml.GPM_METRIC_GRAPHICS_UTIL), NvmlReturn: uint32(nvml.ERROR_UNKNOWN)}
	if err := lib.GpmMetricsGet(get); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if ret := nvml.Return(get.Metrics[0].NvmlReturn); ret != nvml.SUCCESS {
		http.Error(w, nvml.ErrorString(ret), http.StatusInternalServerError)
		return
	}
	utils.WriteAsJSON(req, w, get.Metrics[0].Value, utils.CompactOutput)
}

// GetStats returns the debug stats for the GPU monitoring module
func (t *GPUMonitoringModule) GetStats() map[string]interface{} {
	if t.Probe == nil {
		return map[string]interface{}{"ebpf_probes_enabled": false}
	}

	return t.Probe.GetDebugStats()
}

func (t *GPUMonitoringModule) collectEventsHandler(w http.ResponseWriter, r *http.Request) {
	if t.Probe == nil {
		http.Error(w, "GPU eBPF probes are disabled", http.StatusNotImplemented)
		return
	}

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
	if t.leaseDone != nil {
		close(t.leaseDone)
		t.leaseWG.Wait()
	}
	t.dropMigGrSamples(true)
	t.contextCancel()
	if t.driverEventSubscriber != nil {
		t.driverEventSubscriber.Stop()
	}
	if t.Probe != nil {
		t.Probe.Close()
	}
}

func refreshDeviceCache(ctx context.Context, deviceCache ddnvml.DeviceCache, interval time.Duration) {
	if interval <= 0 {
		return
	}

	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if err := deviceCache.Refresh(); err != nil && !ddnvml.IsNVMLReleased() {
				// Quiet while NVML is deliberately released for a GPU reset
				// window: the refresh is expected to fail (skip quietly,
				// like the other NVML users) and the cache keeps serving the
				// pre-reset mapping until the reacquire.
				log.Warnf("failed to refresh GPU device cache: %v", err)
			}
		}
	}
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

// getAgentPIDs returns all matching Agent processes. In some configurations,
// such as when the OTel host profiler is running, multiple datadog-agent
// binaries can be present. Patch all of them until we have a reliable way to
// identify the process that needs GPU device permissions.
func getAgentPIDs(procRoot string) ([]uint32, error) {
	pids, err := kernel.AllPidsProcs(procRoot)
	if err != nil {
		return nil, fmt.Errorf("failed to get all pids: %w", err)
	}

	var agentPIDs []uint32
	for _, pid := range pids {
		proc := uprobes.NewProcInfo(procRoot, uint32(pid))
		exe, err := proc.Exe()
		if err != nil {
			// Ignore this process, we don't want to stop the search because of that
			continue
		}

		if agentProcessRegexp.MatchString(exe) {
			agentPIDs = append(agentPIDs, uint32(pid))
		}
	}

	if len(agentPIDs) == 0 {
		return nil, errors.New("agent process not found")
	}

	return agentPIDs, nil
}

// configureCgroupPermissions configures the cgroup permissions to access NVIDIA
// devices for the system-probe and agent processes, as the NVIDIA device plugin
// sets them in a way that can be overwritten by SystemD cgroups.
func configureCgroupPermissions(ctx context.Context, reapplyInterval time.Duration, reapplyInfinitely bool) {
	root := hostRoot()

	log.Infof("Configuring cgroup permissions for system-probe and agent processes")

	// Always run once immediately
	doConfigureCgroupPermissions(root)

	// Now, if reapplyInterval is greater than 0, schedule a background task to
	// run after that delay. If reapplyInfinitely is true, the task will run
	// indefinitely, otherwise it will run only once. There are two reasons to
	// enable this:
	//
	// 1. To fix race conditions between SystemD and the
	// system-probe permission patching. SystemD might read an old version of
	// the device permissions, then system-probe changes that configuration,
	// patches the cgroup permissions and then SystemD changes the cgroups based
	// on the old config.
	//
	// 2. If the agent container restarts, it will lose the permissions patch. For simplicity,
	// reapply the permissions instead of having the agent request a permission patch.
	if reapplyInterval > 0 {
		go func() {
			log.Infof("Scheduling background re-application of cgroup permissions for system-probe and agent processes in %v, infinite repeats: %t", reapplyInterval, reapplyInfinitely)
			ticker := time.NewTicker(reapplyInterval)
			defer ticker.Stop()

			for {
				select {
				case <-ctx.Done():
					return
				case <-ticker.C:
					// do not spam the logs with informational messages, switch to debug level
					doConfigureCgroupPermissions(root)
					if !reapplyInfinitely {
						return
					}
				}
			}
		}()
	}
}

// cgroupPermissionWarnings holds the cgroup permission failures already logged as warnings, so that a
// persistent failure is not logged on every re-application
var cgroupPermissionWarnings sync.Map

// warnCgroupPermissionsOnce logs the failure as a warning the first time it happens for the key, and at debug
// level afterwards.
func warnCgroupPermissionsOnce(key string, format string, args ...interface{}) {
	if _, warned := cgroupPermissionWarnings.LoadOrStore(key, struct{}{}); warned {
		log.Debugf(format, args...)
		return
	}
	log.Warnf(format, args...)
}

// forgetDepartedAgentWarnings drops the warning state of the agent processes that are gone, so that it does not
// grow with each restart of a failing agent
func forgetDepartedAgentWarnings(agentPIDs []uint32) {
	current := make(map[string]struct{}, 2*len(agentPIDs))
	for _, pid := range agentPIDs {
		current[agentWarningKey(pid)] = struct{}{}
		current[agentCgroupWarningKey(pid)] = struct{}{}
	}
	cgroupPermissionWarnings.Range(func(key, _ any) bool {
		if k := key.(string); strings.HasPrefix(k, "agent-") {
			if _, ok := current[k]; !ok {
				cgroupPermissionWarnings.Delete(k)
			}
		}
		return true
	})
}

// agentWarningKey is the warning key of a failure to configure the cgroup of an agent process
func agentWarningKey(pid uint32) string { return fmt.Sprintf("agent-%d", pid) }

// agentCgroupWarningKey is the warning key of a failure to read the cgroup of an agent process
func agentCgroupWarningKey(pid uint32) string { return fmt.Sprintf("agent-cgroup-%d", pid) }

// agentCgroup is an agent process and its cgroup, as seen from the host
type agentCgroup struct {
	pid    uint32
	cgroup string
}

// agentsNextToSystemProbe returns the agent processes whose cgroup is a sibling of the system-probe one: the agent
// container of the same pod in Kubernetes, the agent service next to the system-probe service on a host. Other
// agents on the node (e.g. another agent DaemonSet or cluster check runners) are left alone, and so are the agent
// processes whose cgroup cannot be read, so that a lookup failure never targets another deployment.
func agentsNextToSystemProbe(sysprobeCgroup string, agentPIDs []uint32, cgroupOf func(uint32) (string, error)) []agentCgroup {
	parent := filepath.Dir(sysprobeCgroup)
	var kept []agentCgroup
	for _, pid := range agentPIDs {
		cgroup, err := cgroupOf(pid)
		if err != nil {
			warnCgroupPermissionsOnce(agentCgroupWarningKey(pid), "Not configuring cgroup permissions for agent process with PID %d: cannot get its cgroup: %v. gpu-monitoring module might not work properly", pid, err)
			continue
		}
		cgroupPermissionWarnings.Delete(agentCgroupWarningKey(pid))
		switch {
		case filepath.Dir(cgroup) != parent:
			log.Debugf("Not configuring cgroup permissions for agent process with PID %d: its cgroup %s is not next to the system-probe one %s", pid, cgroup, sysprobeCgroup)
		default:
			kept = append(kept, agentCgroup{pid: pid, cgroup: cgroup})
		}
	}
	if len(kept) == 0 {
		// Usually the agent of this pod is restarting: skip this round rather than touching the other agents
		log.Debugf("No agent process found next to the system-probe cgroup %s, not configuring any cgroup permissions this time", sysprobeCgroup)
	}
	return kept
}

// doConfigureCgroupPermissions configures the cgroup permissions to access NVIDIA
// devices for the system-probe and agent processes.
func doConfigureCgroupPermissions(root string) {
	sysprobePID := gpu.SelfHostPID()

	// Resolved once: it is the cgroup to patch for system-probe, and it tells the agent of this pod apart from the
	// other agents of the node
	sysprobeCgroup, err := gpu.CgroupPathForProcess(sysprobePID, root)
	if err != nil {
		warnCgroupPermissionsOnce("system-probe-cgroup", "Failed to get the system-probe cgroup: %v. Cannot patch cgroup permissions, gpu-monitoring module might not work properly", err)
		return
	}
	cgroupPermissionWarnings.Delete("system-probe-cgroup")

	log.Debugf("Configuring cgroup permissions for system-probe process with PID %d, cgroup %s", sysprobePID, sysprobeCgroup)
	if err := gpu.ConfigureCgroupDevices(sysprobeCgroup, root); err != nil {
		warnCgroupPermissionsOnce("system-probe", "Failed to configure cgroup permissions for system-probe process: %v. gpu-monitoring module might not work properly", err)
	} else {
		cgroupPermissionWarnings.Delete("system-probe")
	}

	procRoot := filepath.Join(root, "proc")
	agentPIDs, err := getAgentPIDs(procRoot)
	if err != nil {
		warnCgroupPermissionsOnce("agent-pids", "Failed to get agent PIDs: %v. Cannot patch cgroup permissions, gpu-monitoring module might not work properly", err)
		return
	}
	cgroupPermissionWarnings.Delete("agent-pids")

	forgetDepartedAgentWarnings(agentPIDs)
	agents := agentsNextToSystemProbe(sysprobeCgroup, agentPIDs, func(pid uint32) (string, error) {
		return gpu.CgroupPathForProcess(pid, root)
	})
	for _, agent := range agents {
		log.Debugf("Configuring cgroup permissions for agent process with PID %d, cgroup %s", agent.pid, agent.cgroup)
		key := agentWarningKey(agent.pid)
		if err := gpu.ConfigureCgroupDevices(agent.cgroup, root); err != nil {
			warnCgroupPermissionsOnce(key, "Failed to configure cgroup permissions for agent process with PID %d: %v. gpu-monitoring module might not work properly", agent.pid, err)
		} else {
			cgroupPermissionWarnings.Delete(key)
		}
	}
}
