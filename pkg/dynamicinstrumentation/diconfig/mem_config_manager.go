// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf

package diconfig

import (
	"encoding/json"
	"fmt"
	"io"
	"maps"
	"reflect"
	"sync"

	"github.com/DataDog/datadog-agent/pkg/dynamicinstrumentation/ditypes"
	"github.com/DataDog/datadog-agent/pkg/dynamicinstrumentation/proctracker"
	"github.com/DataDog/datadog-agent/pkg/ebpf/process"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// ReaderConfigManager is used to track updates to configurations
// which are read from memory
type ReaderConfigManager struct {
	sync.Mutex
	ConfigWriter *ConfigWriter
	ProcTracker  *proctracker.ProcessTracker

	callback configUpdateCallback
	configs  configsByService
	state    ditypes.DIProcs
}

type configsByService = map[ditypes.ServiceName]map[ditypes.ProbeID]rcConfig

// NewReaderConfigManager creates a new ReaderConfigManager
func NewReaderConfigManager(pm process.Subscriber) (*ReaderConfigManager, error) {
	cm := &ReaderConfigManager{
		callback: applyConfigUpdate,
		state:    ditypes.NewDIProcs(),
	}

	cm.ProcTracker = proctracker.NewProcessTracker(pm, cm.updateProcessInfo)
	err := cm.ProcTracker.Start()
	if err != nil {
		return nil, err
	}

	reader := NewConfigWriter(cm.updateServiceConfigs)
	err = reader.Start()
	if err != nil {
		return nil, err
	}
	cm.ConfigWriter = reader
	return cm, nil
}

// GetProcInfos returns the process info state
func (cm *ReaderConfigManager) GetProcInfos() ditypes.DIProcs {
	cm.Lock()
	defer cm.Unlock()
	return maps.Clone(cm.state)
}

// GetProcInfo returns the process info state for a specific PID
func (cm *ReaderConfigManager) GetProcInfo(pid ditypes.PID) *ditypes.ProcessInfo {
	cm.Lock()
	defer cm.Unlock()
	return cm.state[pid]
}

// Stop causes the ReaderConfigManager to stop processing data
func (cm *ReaderConfigManager) Stop() {
	cm.ConfigWriter.Stop()
	cm.ProcTracker.Stop()
}

func (cm *ReaderConfigManager) update() error {
	var updatedState = ditypes.NewDIProcs()
	for serviceName, configsByID := range cm.configs {
		for pid, proc := range cm.ConfigWriter.Processes() {
			// If a config exists relevant to this proc
			if proc.ServiceName == serviceName {
				updatedState[pid] = &ditypes.ProcessInfo{
					PID:                 proc.PID,
					ServiceName:         proc.ServiceName,
					RuntimeID:           proc.RuntimeID,
					BinaryPath:          proc.BinaryPath,
					TypeMap:             proc.TypeMap,
					ConfigurationUprobe: proc.ConfigurationUprobe,
					ProbesByID:          convert(serviceName, configsByID),
				}
			}
		}
	}

	if !reflect.DeepEqual(cm.state, updatedState) {
		statuses, err := inspectGoBinaries(updatedState)
		if err != nil {
			return err
		}

		for pid, procInfo := range cm.state {
			// cleanup dead procs
			if _, running := updatedState[pid]; !running {
				procInfo.CloseAllUprobeLinks()
				delete(cm.state, pid)
			}
		}

		for pid, procInfo := range updatedState {
			if !statuses[pid] {
				log.Info("Skipped the installation/deletion of probes for pid %d - failed to analyze its binary", pid)
				continue
			}

			if _, tracked := cm.state[pid]; !tracked {
				for _, probe := range procInfo.GetProbes() {
					// install all probes from new process
					cm.callback(procInfo, probe)
				}
			} else {
				currentStateProbes := cm.state[pid].GetProbes()
				for _, existingProbe := range currentStateProbes {
					cm.state[pid].DeleteProbe(existingProbe.ID)
				}
				for _, updatedProbe := range procInfo.GetProbes() {
					cm.callback(procInfo, updatedProbe)
				}
			}
		}
		cm.state = updatedState
	}
	return nil
}

func (cm *ReaderConfigManager) updateProcessInfo(procs ditypes.DIProcs) {
	cm.Lock()
	defer cm.Unlock()
	log.Info("Updating procs", procs)
	cm.ConfigWriter.UpdateProcesses(procs)
	err := cm.update()
	if err != nil {
		log.Info(err)
	}
}

func (cm *ReaderConfigManager) updateServiceConfigs(configs configsByService) {
	cm.Lock()
	defer cm.Unlock()

	cm.configs = configs
	err := cm.update()
	if err != nil {
		log.Info(err)
	}
}

// ConfigWriter handles writing configuration data
type ConfigWriter struct {
	io.Writer
	updateChannel  chan map[string]map[string]rcConfig
	processes      map[ditypes.PID]*ditypes.ProcessInfo
	configCallback ConfigWriterCallback
	stopChannel    chan (bool)
	mtx            sync.Mutex
}

// ConfigWriterCallback provides a callback interface for ConfigWriter
type ConfigWriterCallback func(configsByService)

// NewConfigWriter creates a new ConfigWriter
func NewConfigWriter(onConfigUpdate ConfigWriterCallback) *ConfigWriter {
	return &ConfigWriter{
		updateChannel:  make(chan map[string]map[string]rcConfig),
		configCallback: onConfigUpdate,
		stopChannel:    make(chan bool),
	}
}

// Processes returns a copy of the current processes
func (r *ConfigWriter) Processes() map[ditypes.PID]*ditypes.ProcessInfo {
	r.mtx.Lock()
	procs := maps.Clone(r.processes)
	r.mtx.Unlock()
	return procs
}

// WriteSync accepts the incoming RC config for processing (installation/deletion/editing of probes)
// used by Go DI testing infra
func (r *ConfigWriter) WriteSync(p []byte) error {
	conf, err := unmarshalToRcConfig(p)
	if err != nil {
		return err
	}
	r.configCallback(conf)
	return nil
}

func (r *ConfigWriter) Write(p []byte) (n int, e error) {
	conf, err := unmarshalToRcConfig(p)
	if err != nil {
		return 0, err
	}
	r.updateChannel <- conf
	return 0, nil
}

func unmarshalToRcConfig(p []byte) (map[string]map[string]rcConfig, error) {
	conf := map[string]map[string]rcConfig{}
	err := json.Unmarshal(p, &conf)
	if err != nil {
		return nil, log.Errorf("invalid config read from reader: %v", err)
	}
	return conf, nil
}

// Start initiates the ConfigWriter to start processing data
func (r *ConfigWriter) Start() error {
	go func() {
	configUpdateLoop:
		for {
			select {
			case conf := <-r.updateChannel:
				r.configCallback(conf)
			case <-r.stopChannel:
				break configUpdateLoop
			}
		}
	}()
	return nil
}

// Stop causes the ConfigWriter to stop processing data
func (r *ConfigWriter) Stop() {
	r.stopChannel <- true
}

// UpdateProcesses is the callback interface that ConfigWriter uses to consume the map of ProcessInfo's
// such that it's used whenever there's an update to the state of known service processes on the machine.
// It simply overwrites the previous state of known service processes with the new one
func (r *ConfigWriter) UpdateProcesses(procs ditypes.DIProcs) {
	r.mtx.Lock()
	defer r.mtx.Unlock()

	current := procs
	old := r.processes
	if !reflect.DeepEqual(current, old) {
		r.processes = current
	}
}

func convert(service string, configsByID map[ditypes.ProbeID]rcConfig) *ditypes.ProbesByID {
	probesByID := ditypes.NewProbesByID()
	for id, config := range configsByID {
		probesByID.Set(id, config.toProbe(service))
	}
	return probesByID
}

func (rc *rcConfig) toProbe(service string) *ditypes.Probe {
	return &ditypes.Probe{
		ID:          rc.ID,
		ServiceName: service,
		FuncName:    fmt.Sprintf("%s.%s", rc.Where.TypeName, rc.Where.MethodName),
		InstrumentationInfo: &ditypes.InstrumentationInfo{
			InstrumentationOptions: &ditypes.InstrumentationOptions{
				CaptureParameters: ditypes.CaptureParameters,
				ArgumentsMaxSize:  ditypes.ArgumentsMaxSize,
				StringMaxSize:     ditypes.StringMaxSize,
				SliceMaxLength:    ditypes.SliceMaxLength,
				MaxReferenceDepth: rc.Capture.MaxReferenceDepth,
				MaxFieldCount:     rc.Capture.MaxFieldCount,
			},
		},
	}
}
