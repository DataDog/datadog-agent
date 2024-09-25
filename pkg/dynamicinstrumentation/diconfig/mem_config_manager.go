// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf && arm64

package diconfig

import (
	"encoding/json"
	"fmt"
	"io"
	"reflect"
	"sync"

	"github.com/DataDog/datadog-agent/pkg/dynamicinstrumentation/ditypes"
	"github.com/DataDog/datadog-agent/pkg/dynamicinstrumentation/proctracker"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// ReaderConfigManager is used to track updates to configurations
// which are read from memory
type ReaderConfigManager struct {
	sync.Mutex
	ConfigWriter *ConfigWriter
	procTracker  *proctracker.ProcessTracker

	callback configUpdateCallback
	configs  configsByService
	state    ditypes.DIProcs
}

type readerConfigCallback func(configsByService)
type configsByService = map[ditypes.ServiceName]map[ditypes.ProbeID]rcConfig

func NewReaderConfigManager() (*ReaderConfigManager, error) {
	cm := &ReaderConfigManager{
		callback: applyConfigUpdate,
		state:    ditypes.NewDIProcs(),
	}

	cm.procTracker = proctracker.NewProcessTracker(cm.updateProcessInfo)
	err := cm.procTracker.Start()
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

func (cm *ReaderConfigManager) GetProcInfos() ditypes.DIProcs {
	return cm.state
}

func (cm *ReaderConfigManager) Stop() {
	cm.ConfigWriter.Stop()
	cm.procTracker.Stop()
}

func (cm *ReaderConfigManager) update() error {
	var updatedState = ditypes.NewDIProcs()
	for serviceName, configsByID := range cm.configs {
		for pid, proc := range cm.ConfigWriter.Processes {
			// If a config exists relevant to this proc
			if proc.ServiceName == serviceName {
				procCopy := *proc
				updatedState[pid] = &procCopy
				updatedState[pid].ProbesByID = convert(serviceName, configsByID)
			}
		}
	}

	if !reflect.DeepEqual(cm.state, updatedState) {
		err := inspectGoBinaries(updatedState)
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
	cm.configs = configs
	err := cm.update()
	if err != nil {
		log.Info(err)
	}
}

type ConfigWriter struct {
	io.Writer
	updateChannel  chan ([]byte)
	Processes      map[ditypes.PID]*ditypes.ProcessInfo
	configCallback ConfigWriterCallback
	stopChannel    chan (bool)
}

type ConfigWriterCallback func(configsByService)

func NewConfigWriter(onConfigUpdate ConfigWriterCallback) *ConfigWriter {
	return &ConfigWriter{
		updateChannel:  make(chan []byte, 1),
		configCallback: onConfigUpdate,
		stopChannel:    make(chan bool),
	}
}

func (r *ConfigWriter) Write(p []byte) (n int, e error) {
	r.updateChannel <- p
	return 0, nil
}

func (r *ConfigWriter) Start() error {
	go func() {
	configUpdateLoop:
		for {
			select {
			case rawConfigBytes := <-r.updateChannel:
				conf := map[string]map[string]rcConfig{}
				err := json.Unmarshal(rawConfigBytes, &conf)
				if err != nil {
					log.Errorf("invalid config read from reader: %v", err)
					continue
				}
				r.configCallback(conf)
			case <-r.stopChannel:
				break configUpdateLoop
			}
		}
	}()
	return nil
}

func (cu *ConfigWriter) Stop() {
	cu.stopChannel <- true
}

// UpdateProcesses is the callback interface that ConfigWriter uses to consume the map of ProcessInfo's
// such that it's used whenever there's an update to the state of known service processes on the machine.
// It simply overwrites the previous state of known service processes with the new one
func (cu *ConfigWriter) UpdateProcesses(procs ditypes.DIProcs) {
	current := procs
	old := cu.Processes
	if !reflect.DeepEqual(current, old) {
		cu.Processes = current
	}
}

func convert(service string, configsByID map[ditypes.ProbeID]rcConfig) map[ditypes.ProbeID]*ditypes.Probe {
	probesByID := map[ditypes.ProbeID]*ditypes.Probe{}
	for id, config := range configsByID {
		probesByID[id] = config.toProbe(service)
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
				MaxReferenceDepth: rc.Capture.MaxReferenceDepth,
			},
		},
	}
}
