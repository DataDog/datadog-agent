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
	"fmt"
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
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

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
	Stop()
}

// RCConfigManager is the configuration manager which utilizes remote-config
type RCConfigManager struct {
	sync.RWMutex
	procTracker *proctracker.ProcessTracker

	diProcs  ditypes.DIProcs
	callback configUpdateCallback
}

// NewRCConfigManager creates a new configuration manager which utilizes remote-config
func NewRCConfigManager() (*RCConfigManager, error) {
	log.Info("Creating new RC config manager")
	cm := &RCConfigManager{
		callback: applyConfigUpdate,
	}

	cm.procTracker = proctracker.NewProcessTracker(cm.updateProcesses)
	err := cm.procTracker.Start()
	if err != nil {
		return nil, fmt.Errorf("could not start process tracker: %w", err)
	}
	cm.diProcs = ditypes.NewDIProcs()
	return cm, nil
}

// GetProcInfos returns a copy of the state of the RCConfigManager
func (cm *RCConfigManager) GetProcInfos() ditypes.DIProcs {
	cm.RLock()
	defer cm.RUnlock()
	return cm.diProcs
}

// Stop closes the config and proc trackers used by the RCConfigManager
func (cm *RCConfigManager) Stop() {
	cm.Lock()
	defer cm.Unlock()
	cm.procTracker.Stop()
	for _, procInfo := range cm.diProcs {
		procInfo.CloseAllUprobeLinks()
	}
}

// updateProcesses is the callback interface that ConfigManager uses to consume the map of `ProcessInfo`s
// It is called whenever there's an update to the state of known processes of services on the machine.
//
// It compares the previously known state of services on the machine and creates a hook on the remote-config
// callback for configurations on new ones, and deletes the hook on old ones.
func (cm *RCConfigManager) updateProcesses(runningProcs ditypes.DIProcs) {
	cm.Lock()
	defer cm.Unlock()
	// Remove processes that are no longer running from state and close their uprobe links
	for pid, procInfo := range cm.diProcs {
		_, ok := runningProcs[pid]
		if !ok {
			procInfo.CloseAllUprobeLinks()
			delete(cm.diProcs, pid)
		}
	}

	for pid, runningProcInfo := range runningProcs {
		_, ok := cm.diProcs[pid]
		if !ok {
			cm.diProcs[pid] = runningProcInfo
			err := cm.installConfigProbe(runningProcInfo)
			if err != nil {
				log.Infof("could not install config probe for service %s (pid %d): %s", runningProcInfo.ServiceName, runningProcInfo.PID, err)
			}
		}
	}
}

func (cm *RCConfigManager) installConfigProbe(procInfo *ditypes.ProcessInfo) error {
	var err error
	configProbe := newConfigProbe()

	svcConfigProbe := *configProbe
	svcConfigProbe.ServiceName = procInfo.ServiceName
	procInfo.ProbesByID.Set(configProbe.ID, &svcConfigProbe)

	log.Infof("Installing config probe for service: %s", svcConfigProbe.ServiceName)
	procInfo.TypeMap = &ditypes.TypeMap{
		Functions: make(map[string][]*ditypes.Parameter),
	}
	procInfo.TypeMap.Functions[ditypes.RemoteConfigCallback] = remoteConfigCallbackTypeMapEntry

	err = codegen.GenerateBPFParamsCode(procInfo, configProbe)
	if err != nil {
		return fmt.Errorf("could not generate bpf code for config probe: %w", err)
	}

	err = ebpf.CompileBPFProgram(configProbe)
	if err != nil {
		return fmt.Errorf("could not compile bpf code for config probe: %w", err)
	}

	err = ebpf.AttachBPFUprobe(procInfo, configProbe)
	if err != nil {
		return fmt.Errorf("could not attach bpf code for config probe: %w", err)
	}

	m, err := procInfo.SetupConfigUprobe()
	if err != nil {
		return fmt.Errorf("could not setup config probe for service %s: %w", procInfo.ServiceName, err)
	}

	r, err := ringbuf.NewReader(m)
	if err != nil {
		return fmt.Errorf("could not read from config probe %s", procInfo.ServiceName)
	}

	go cm.readConfigs(r, procInfo)

	return nil
}

func (cm *RCConfigManager) readConfigs(r *ringbuf.Reader, procInfo *ditypes.ProcessInfo) {
	log.Tracef("Waiting for configs for service: %s", procInfo.ServiceName)
	configRateLimiter := ratelimiter.NewMultiProbeRateLimiter(0.0)
	configRateLimiter.SetRate(ditypes.ConfigBPFProbeID, 0)

	for {
		record, err := r.Read()
		if err != nil {
			log.Errorf("error reading raw configuration from bpf: %v", err)
			continue
		}

		configEvent, err := eventparser.ParseEvent(record.RawSample, configRateLimiter)
		if err != nil {
			log.Errorf("error parsing configuration for PID %d: %v", procInfo.PID, err)
			continue
		}
		configEventParams := configEvent.Argdata
		if len(configEventParams) != 3 {
			log.Errorf("error parsing configuration for PID: %d: not enough arguments (got %d): %s", procInfo.PID, len(configEventParams), string(record.RawSample))
			continue
		}

		runtimeID, err := uuid.ParseBytes([]byte(configEventParams[0].ValueStr))
		if err != nil {
			log.Errorf("Runtime ID \"%s\" is not a UUID: %v)", configEventParams[0].ValueStr, err)
			continue
		}

		configPath, err := ditypes.ParseConfigPath(string(configEventParams[1].ValueStr))
		if err != nil {
			log.Errorf("couldn't parse config path (%s): %v", string(configEventParams[1].ValueStr), err)
			continue
		}

		// An empty config means that this probe has been removed for this process
		if configEventParams[2].ValueStr == "" {
			cm.Lock()
			cm.diProcs.DeleteProbe(procInfo.PID, configPath.ProbeUUID.String())
			cm.Unlock()
			continue
		}

		conf := rcConfig{}
		err = json.Unmarshal([]byte(configEventParams[2].ValueStr), &conf)
		if err != nil {
			diagnostics.Diagnostics.SetError(procInfo.ServiceName, procInfo.RuntimeID, configPath.ProbeUUID.String(), "ATTACH_ERROR", err.Error())
			log.Errorf("could not unmarshal configuration, cannot apply: %v (Probe-ID: %s)\n", err, configPath.ProbeUUID)
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

		cm.Lock()
		probe := procInfo.ProbesByID.Get(configPath.ProbeUUID.String())
		if probe == nil {
			cm.diProcs.SetProbe(procInfo.PID, procInfo.ServiceName, conf.Where.TypeName, conf.Where.MethodName, configPath.ProbeUUID, runtimeID, opts)
			diagnostics.Diagnostics.SetStatus(procInfo.ServiceName, runtimeID.String(), configPath.ProbeUUID.String(), ditypes.StatusReceived)
			probe = procInfo.ProbesByID.Get(configPath.ProbeUUID.String())
		}

		// Check hash to see if the configuration changed
		if configPath.Hash != probe.InstrumentationInfo.ConfigurationHash {
			err := AnalyzeBinary(procInfo)
			if err != nil {
				log.Errorf("couldn't inspect binary (%s): %v\n", procInfo.BinaryPath, err)
				cm.Unlock()
				continue
			}

			probe.InstrumentationInfo.ConfigurationHash = configPath.Hash
			applyConfigUpdate(procInfo, probe)
		}
		cm.Unlock()
	}
}

func applyConfigUpdate(procInfo *ditypes.ProcessInfo, probe *ditypes.Probe) {
	log.Debugf("Applying config update for: %s in %s (ID: %s)\n", probe.FuncName, probe.ServiceName, probe.ID)
	for {
		if err := tryGenerateAndAttach(procInfo, probe); err == nil {
			return
		}
	}
}

// tryGenerateAndAttach attempts to generate and attach the BPF program for the probe
// it will decrement the reference depth of the probe if it fails to generate and attach
// the BPF program and try again until the reference depth is 0
func tryGenerateAndAttach(procInfo *ditypes.ProcessInfo, probe *ditypes.Probe) error {
	err := codegen.GenerateBPFParamsCode(procInfo, probe)
	if err != nil {
		log.Errorf("Couldn't generate BPF programs for %s: %v", probe.FuncName, err)
		if !haveExhaustedReferenceDepthDecrementing(probe) {
			return err
		}
		return nil
	}
	err = ebpf.CompileBPFProgram(probe)
	if err != nil {
		log.Errorf("Couldn't compile BPF object for function %s (ID: %s): %v", probe.FuncName, probe.ID, err)
		if !haveExhaustedReferenceDepthDecrementing(probe) {
			return err
		}
		return nil
	}
	err = ebpf.AttachBPFUprobe(procInfo, probe)
	if err != nil {
		log.Errorf("Couldn't load and attach bpf programs for function %s (ID: %s): %v", probe.FuncName, probe.ID, err)
		if !haveExhaustedReferenceDepthDecrementing(probe) {
			return err
		}
		return nil
	}
	return nil
}

// haveExhaustedReferenceDepthDecrementing checks if the reference depth has been exhausted
// in the process of decrementing itand if so, marks all parameters as not captured
func haveExhaustedReferenceDepthDecrementing(probe *ditypes.Probe) bool {
	if !checkAndDecrementReferenceDepth(probe) {
		probe.InstrumentationInfo.InstrumentationOptions.CaptureParameters = false
		return true
	}
	return false
}

// checkAndDecrementReferenceDepth decrements the reference depth of the probe
// and returns true if the reference depth is still greater than 0
func checkAndDecrementReferenceDepth(probe *ditypes.Probe) bool {
	if !probe.InstrumentationInfo.InstrumentationOptions.CaptureParameters ||
		probe.InstrumentationInfo.InstrumentationOptions.MaxReferenceDepth <= 0 {
		return false
	}
	probe.InstrumentationInfo.InstrumentationOptions.MaxReferenceDepth--
	log.Tracef("Retrying after decrementing capture depth to: %d", probe.InstrumentationInfo.InstrumentationOptions.MaxReferenceDepth)
	return true
}

func newConfigProbe() *ditypes.Probe {
	return &ditypes.Probe{
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
}

const (
	// ConfigProbeArgumentsMaxSize is the maximum size of the raw argument buffer
	ConfigProbeArgumentsMaxSize = 50000
	// ConfigProbeStringMaxSize is the maximum allowed size of instrumented string parameters/fields
	ConfigProbeStringMaxSize = 10000
)
