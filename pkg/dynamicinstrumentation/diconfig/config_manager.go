// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf

// Package diconfig provides utilities that allows dynamic instrumentation to receive and
// manage probe configurations from users
package diconfig

import (
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"sync"

	"github.com/cilium/ebpf/ringbuf"
	"github.com/google/uuid"

	"github.com/DataDog/datadog-agent/pkg/dynamicinstrumentation/codegen"
	"github.com/DataDog/datadog-agent/pkg/dynamicinstrumentation/diagnostics"
	"github.com/DataDog/datadog-agent/pkg/dynamicinstrumentation/ditypes"
	"github.com/DataDog/datadog-agent/pkg/dynamicinstrumentation/ebpf"
	"github.com/DataDog/datadog-agent/pkg/dynamicinstrumentation/eventparser"
	"github.com/DataDog/datadog-agent/pkg/dynamicinstrumentation/proctracker"
	"github.com/DataDog/datadog-agent/pkg/dynamicinstrumentation/ratelimiter"
	"github.com/DataDog/datadog-agent/pkg/ebpf/process"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// errReferenceDepthExhausted is returned by tryGenerateAndAttach when all retries by decrementing reference depth are exhausted.
var errReferenceDepthExhausted = errors.New("reference depth exhausted during BPF program generation/attachment")

type rcConfig struct {
	ID        string
	Version   int
	ProbeType string `json:"type"`
	Language  string
	Where     struct {
		TypeName   string `json:"typeName"`
		MethodName string `json:"methodName"`
		SourceFile string
		Lines      []string
	}
	Tags            []string
	Template        string
	CaptureSnapshot bool
	EvaluatedAt     string
	Capture         struct {
		MaxReferenceDepth int `json:"maxReferenceDepth"`
		MaxFieldCount     int `json:"maxFieldCount"`
	}
}

type configUpdateCallback func(*ditypes.ProcessInfo, *ditypes.Probe)

// ConfigManager is a facility to track probe configurations for
// instrumenting tracked processes
type ConfigManager interface {
	GetProcInfos() ditypes.DIProcs
	GetProcInfo(ditypes.PID) *ditypes.ProcessInfo
	Stop()
}

// RCConfigManager is the configuration manager which utilizes remote-config
type RCConfigManager struct {
	procTracker *proctracker.ProcessTracker

	mu struct {
		sync.RWMutex
		diProcs  ditypes.DIProcs
		callback configUpdateCallback
	}
}

// NewRCConfigManager creates a new configuration manager which utilizes remote-config
func NewRCConfigManager(pm process.Subscriber) (*RCConfigManager, error) {
	log.Info("Creating new RC config manager")
	cm := &RCConfigManager{}
	cm.mu.callback = applyConfigUpdate
	cm.mu.diProcs = ditypes.NewDIProcs()

	cm.procTracker = proctracker.NewProcessTracker(pm, cm.updateProcesses)
	err := cm.procTracker.Start()
	if err != nil {
		return nil, fmt.Errorf("could not start process tracker: %w", err)
	}
	return cm, nil
}

// GetProcInfos returns a copy of the state of the RCConfigManager.
func (cm *RCConfigManager) GetProcInfos() ditypes.DIProcs {
	cm.mu.RLock()
	defer cm.mu.RUnlock()
	return maps.Clone(cm.mu.diProcs)
}

// GetProcInfo returns the ProcessInfo for the given PID.
func (cm *RCConfigManager) GetProcInfo(pid ditypes.PID) *ditypes.ProcessInfo {
	cm.mu.RLock()
	defer cm.mu.RUnlock()
	return cm.mu.diProcs[pid]
}

// Stop closes the config and proc trackers used by the RCConfigManager
func (cm *RCConfigManager) Stop() {
	cm.mu.Lock()
	defer cm.mu.Unlock()
	cm.procTracker.Stop()
	for _, procInfo := range cm.mu.diProcs {
		procInfo.CloseAllUprobeLinks()
	}
	log.Infof("Closed all uprobe links, stopped process tracker")
}

// updateProcesses is the callback interface that ConfigManager uses to consume the map of `ProcessInfo`s
// It is called whenever there's an update to the state of known processes of services on the machine.
//
// It compares the previously known state of services on the machine and creates a hook on the remote-config
// callback for configurations on new ones, and deletes the hook on old ones.
func (cm *RCConfigManager) updateProcesses(runningProcs ditypes.DIProcs) {
	cm.mu.Lock()
	defer cm.mu.Unlock()
	// Remove processes that are no longer running from state and close their uprobe links
	for pid, procInfo := range cm.mu.diProcs {
		_, ok := runningProcs[pid]
		if !ok {
			procInfo.CloseAllUprobeLinks()
			delete(cm.mu.diProcs, pid)
		}
	}
	for pid, runningProcInfo := range runningProcs {
		_, ok := cm.mu.diProcs[pid]
		if !ok {
			cm.mu.diProcs[pid] = runningProcInfo
			err := cm.installConfigProbe(runningProcInfo)
			if err != nil {
				log.Errorf("could not install config probe for service %s (pid %d): %s", runningProcInfo.ServiceName, runningProcInfo.PID, err)
			}
		} else {
			log.Infof("config probe already installed for %d %s", pid, runningProcInfo.ServiceName)
		}
	}
}

func (cm *RCConfigManager) installConfigProbe(procInfo *ditypes.ProcessInfo) error {
	var err error
	configProbe := newConfigProbe(procInfo.DDTracegoVersion)

	svcConfigProbe := *configProbe
	svcConfigProbe.ServiceName = procInfo.ServiceName
	procInfo.ProbesByID.Set(configProbe.ID, &svcConfigProbe)

	log.Infof("Installing config probe for service: %d %s %s", procInfo.PID, svcConfigProbe.ServiceName, procInfo.BinaryPath)
	procInfo.TypeMap = &ditypes.TypeMap{
		Functions: make(map[string][]*ditypes.Parameter),
	}
	procInfo.TypeMap.Functions[ditypes.RemoteConfigCallbackV2] = remoteConfigCallbackTypeMapEntry
	procInfo.TypeMap.Functions[ditypes.RemoteConfigCallback] = remoteConfigCallbackTypeMapEntry

	err = codegen.GenerateBPFParamsCode(procInfo, configProbe)
	if err != nil {
		return fmt.Errorf("could not generate bpf code for config probe: %d %s %w", procInfo.PID, procInfo.ServiceName, err)
	}

	err = ebpf.CompileBPFProgram(configProbe)
	if err != nil {
		return fmt.Errorf("could not compile bpf code for config probe: %d %s %w", procInfo.PID, procInfo.ServiceName, err)
	}

	err = ebpf.AttachBPFUprobe(procInfo, configProbe)
	if err != nil {
		return fmt.Errorf("could not attach bpf code for config probe: %d %s %w", procInfo.PID, procInfo.ServiceName, err)
	}

	m, err := procInfo.SetupConfigUprobe()
	if err != nil {
		return fmt.Errorf("could not setup config probe for service %d %s: %w", procInfo.PID, procInfo.ServiceName, err)
	}

	r, err := ringbuf.NewReader(m)
	if err != nil {
		return fmt.Errorf("could not read from config probe %d %s: %w", procInfo.PID, procInfo.ServiceName, err)
	}

	go cm.readConfigs(r, procInfo)

	return nil
}

func (cm *RCConfigManager) readConfigs(r *ringbuf.Reader, procInfo *ditypes.ProcessInfo) {
	log.Tracef("Waiting for configs for service: %d %s", procInfo.PID, procInfo.ServiceName)
	configRateLimiter := ratelimiter.NewMultiProbeRateLimiter(0.0)
	configRateLimiter.SetRate(ditypes.ConfigBPFProbeID, 0)

	for {
		record, err := r.Read()
		if err != nil {
			log.Errorf("error reading raw configuration from bpf: %d %s %v", procInfo.PID, procInfo.ServiceName, err)
			continue
		}

		configEvent, err := eventparser.ParseEvent(record.RawSample, configRateLimiter)
		if err != nil {
			log.Errorf("error parsing configuration for PID %d %s: %v", procInfo.PID, procInfo.ServiceName, err)
			continue
		}
		configEventParams := configEvent.Argdata
		if len(configEventParams) != 3 {
			log.Errorf("error parsing configuration for PID %d %s: not enough arguments (got %d): %s", procInfo.PID, procInfo.ServiceName, len(configEventParams), string(record.RawSample))
			continue
		}

		runtimeID, err := uuid.ParseBytes([]byte(configEventParams[0].ValueStr))
		if err != nil {
			log.Errorf("error parsing event uuid: \"%s\" is not a uuid: %d %s %v", configEventParams[0].ValueStr, procInfo.PID, procInfo.ServiceName, err)
			continue
		}

		configPath, err := ditypes.ParseConfigPath(string(configEventParams[1].ValueStr))
		if err != nil {
			log.Errorf("couldn't parse config path (%s): %d %s %v", string(configEventParams[1].ValueStr), procInfo.PID, procInfo.ServiceName, err)
			continue
		}

		// An empty config means that this probe has been removed for this process
		if configEventParams[2].ValueStr == "" {
			cm.mu.Lock()
			cm.mu.diProcs.DeleteProbe(procInfo.PID, configPath.ProbeUUID.String())
			cm.mu.Unlock()
			continue
		}

		conf := rcConfig{}
		err = json.Unmarshal([]byte(configEventParams[2].ValueStr), &conf)
		if err != nil {
			diagnostics.Diagnostics.SetError(procInfo.ServiceName, procInfo.RuntimeID, configPath.ProbeUUID.String(), "ATTACH_ERROR", err.Error())
			log.Errorf("could not unmarshal configuration, cannot apply: %d %s %v (Probe-ID: %s)\n", procInfo.PID, procInfo.ServiceName, err, configPath.ProbeUUID)
			continue
		}

		if conf.Capture.MaxReferenceDepth == 0 {
			conf.Capture.MaxReferenceDepth = int(ditypes.MaxReferenceDepth)
		}
		if conf.Capture.MaxFieldCount == 0 {
			conf.Capture.MaxFieldCount = int(ditypes.MaxFieldCount)
		}
		opts := &ditypes.InstrumentationOptions{
			CaptureParameters: ditypes.CaptureParameters,
			ArgumentsMaxSize:  ditypes.ArgumentsMaxSize,
			StringMaxSize:     ditypes.StringMaxSize,
			MaxReferenceDepth: conf.Capture.MaxReferenceDepth,
			MaxFieldCount:     conf.Capture.MaxFieldCount,
		}

		cm.mu.Lock()
		probe := procInfo.ProbesByID.Get(configPath.ProbeUUID.String())
		if probe == nil {
			cm.mu.diProcs.SetProbe(procInfo.PID, procInfo.ServiceName, conf.Where.TypeName, conf.Where.MethodName, configPath.ProbeUUID, runtimeID, opts)
			diagnostics.Diagnostics.SetStatus(procInfo.ServiceName, runtimeID.String(), configPath.ProbeUUID.String(), ditypes.StatusReceived)
			probe = procInfo.ProbesByID.Get(configPath.ProbeUUID.String())
			if probe != nil {
				log.Infof("Received config for %d %s %s", procInfo.PID, procInfo.ServiceName, probe.FuncName)
			}
		}

		// Check hash to see if the configuration changed
		if configPath.Hash != probe.InstrumentationInfo.ConfigurationHash {
			log.Infof("Configuration hash changed for %d %s %s", procInfo.PID, procInfo.ServiceName, probe.FuncName)
			err := AnalyzeBinary(procInfo)
			if err != nil {
				log.Errorf("couldn't inspect binary (%s): %v\n", procInfo.BinaryPath, err)
				cm.mu.Unlock()
				continue
			}
			probe.InstrumentationInfo.ConfigurationHash = configPath.Hash
			applyConfigUpdate(procInfo, probe)
		}
		cm.mu.Unlock()
	}
}

func applyConfigUpdate(procInfo *ditypes.ProcessInfo, probe *ditypes.Probe) {
	log.Infof("Applying config update for: %d %s %s (ID: %s)\n", procInfo.PID, procInfo.ServiceName, probe.FuncName, probe.ID)
	for {
		err := tryGenerateAndAttach(procInfo, probe)
		if err == nil {
			log.Infof("Successfully generated and attached BPF program for %d %s %s (ID: %s)", procInfo.PID, procInfo.ServiceName, probe.FuncName, probe.ID)
			return
		}
		if errors.Is(err, errReferenceDepthExhausted) {
			log.Infof("Exhausted retries (reference depth reached zero) while attempting to generate and attach BPF program for %d %s %s (ID: %s). Parameter capturing may be disabled.", procInfo.PID, procInfo.ServiceName, probe.FuncName, probe.ID)
			return
		}
		log.Warnf("Failed to generate and attach BPF program for %d %s %s (ID: %s), will retry if possible: %v", procInfo.PID, procInfo.ServiceName, probe.FuncName, probe.ID, err)
	}
}

// tryGenerateAndAttach attempts to generate and attach the BPF program for the probe
// it will decrement the reference depth of the probe if it fails to generate and attach
// the BPF program and try again until the reference depth is 0.
// It returns nil on success, errReferenceDepthExhausted if retries are exhausted,
// or the underlying error if a failure occurs and retries are still available.
func tryGenerateAndAttach(procInfo *ditypes.ProcessInfo, probe *ditypes.Probe) error {
	log.Infof("Attempting to generate and attach BPF program for %d %s %s (ID: %s)", procInfo.PID, procInfo.ServiceName, probe.FuncName, probe.ID)
	err := codegen.GenerateBPFParamsCode(procInfo, probe)
	if err != nil {
		log.Errorf("Couldn't generate BPF programs for %d %s %s: %v", procInfo.PID, procInfo.ServiceName, probe.FuncName, err)
		if isReferenceDepthExhaustedAfterDecrementing(probe) {
			return errReferenceDepthExhausted
		}
		return err
	}
	err = ebpf.CompileBPFProgram(probe)
	if err != nil {
		log.Errorf("Couldn't compile BPF object for function %d %s %s (ID: %s): %v", procInfo.PID, procInfo.ServiceName, probe.FuncName, probe.ID, err)
		if isReferenceDepthExhaustedAfterDecrementing(probe) {
			return errReferenceDepthExhausted
		}
		return err
	}
	err = ebpf.AttachBPFUprobe(procInfo, probe)
	if err != nil {
		log.Errorf("Couldn't load and attach bpf programs for function %d %s %s (ID: %s): %v", procInfo.PID, procInfo.ServiceName, probe.FuncName, probe.ID, err)
		if isReferenceDepthExhaustedAfterDecrementing(probe) {
			return errReferenceDepthExhausted
		}
		return err
	}
	log.Infof("Successfully generated and attached BPF program for %d %s %s (ID: %s)", procInfo.PID, procInfo.ServiceName, probe.FuncName, probe.ID)
	return nil
}

// isReferenceDepthExhaustedAfterDecrementing decrements the reference depth of the probe
// and returns true if the reference depth has been exhausted.
// If exhausted, it also sets CaptureParameters to false.
func isReferenceDepthExhaustedAfterDecrementing(probe *ditypes.Probe) bool {
	if !probe.InstrumentationInfo.InstrumentationOptions.CaptureParameters ||
		probe.InstrumentationInfo.InstrumentationOptions.MaxReferenceDepth <= 0 {
		// Already exhausted or not capturing parameters, so nothing to decrement.
		// Mark as exhausted to be safe, which might set CaptureParameters to false if it wasn't already.
		probe.InstrumentationInfo.InstrumentationOptions.CaptureParameters = false
		return true
	}

	probe.InstrumentationInfo.InstrumentationOptions.MaxReferenceDepth--
	log.Tracef("Decremented capture depth to: %d for %s %s", probe.InstrumentationInfo.InstrumentationOptions.MaxReferenceDepth, probe.ServiceName, probe.FuncName)

	if probe.InstrumentationInfo.InstrumentationOptions.MaxReferenceDepth <= 0 {
		log.Tracef("Reference depth exhausted for %s %s, disabling parameter capture.", probe.ServiceName, probe.FuncName)
		probe.InstrumentationInfo.InstrumentationOptions.CaptureParameters = false
		return true
	}
	return false
}

func newConfigProbe(ddtracegoVersion ditypes.DDTraceGoVersion) *ditypes.Probe {
	probe := &ditypes.Probe{
		ID:       ditypes.ConfigBPFProbeID,
		FuncName: ditypes.RemoteConfigCallback,
		InstrumentationInfo: &ditypes.InstrumentationInfo{
			InstrumentationOptions: &ditypes.InstrumentationOptions{
				ArgumentsMaxSize:  ConfigProbeArgumentsMaxSize,
				StringMaxSize:     ConfigProbeStringMaxSize,
				MaxFieldCount:     int(ditypes.MaxFieldCount),
				MaxReferenceDepth: 8,
				CaptureParameters: true,
			},
		},
		RateLimiter: ratelimiter.NewSingleEventRateLimiter(0),
	}
	if ddtracegoVersion == ditypes.DDTraceGoVersionV2 {
		probe.FuncName = ditypes.RemoteConfigCallbackV2
	}
	return probe
}

const (
	// ConfigProbeArgumentsMaxSize is the maximum size of the raw argument buffer
	ConfigProbeArgumentsMaxSize = 50000
	// ConfigProbeStringMaxSize is the maximum allowed size of instrumented string parameters/fields
	ConfigProbeStringMaxSize = 10000
)
