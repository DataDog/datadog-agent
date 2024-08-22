// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.
package diconfig

import (
	"encoding/json"
	"fmt"
	"reflect"

	"github.com/DataDog/datadog-agent/pkg/util/log"

	"github.com/DataDog/datadog-agent/pkg/di/ditypes"
	"github.com/DataDog/datadog-agent/pkg/di/proctracker"
	"github.com/DataDog/datadog-agent/pkg/di/util"
)

type FileWatchingConfigManager struct {
	configTracker *configTracker
	procTracker   *proctracker.ProcessTracker

	callback configUpdateCallback
	configs  configsByService
	state    ditypes.DIProcs
}

type fileConfigCallback func(configsByService)

type configsByService = map[ditypes.ServiceName]map[ditypes.ProbeID]rcConfig

func NewFileConfigManager(configFile string) (*FileWatchingConfigManager, error) {
	cm := &FileWatchingConfigManager{
		callback: applyConfigUpdate,
	}

	cm.procTracker = proctracker.NewProcessTracker(cm.updateProcessInfo)
	err := cm.procTracker.Start()
	if err != nil {
		return nil, err
	}

	cm.configTracker = NewFileWatchingConfigTracker(configFile, cm.updateServiceConfigs)
	err = cm.configTracker.Start()
	if err != nil {
		return nil, err
	}
	return cm, nil
}

func (cm *FileWatchingConfigManager) GetProcInfos() ditypes.DIProcs {
	return cm.state
}

func (cm *FileWatchingConfigManager) Stop() {
	cm.configTracker.Stop()
	cm.procTracker.Stop()
}

func NewFileWatchingConfigTracker(configFile string, onConfigUpdate fileConfigCallback) *configTracker {
	ct := configTracker{
		ConfigPath:     configFile,
		configCallback: onConfigUpdate,
		stopChannel:    make(chan bool),
	}

	return &ct
}

// correlate this new configuration with a running service,
// and operate on the new global state of services/configs
// via cm.callback
func (cm *FileWatchingConfigManager) updateServiceConfigs(configs configsByService) {
	log.Info("Updating config from file:", configs)
	cm.configs = configs
	err := cm.update()
	if err != nil {
		log.Info(err)
	}
}

func (cm *FileWatchingConfigManager) updateProcessInfo(procs ditypes.DIProcs) {
	log.Info("Updating procs", procs)
	cm.configTracker.UpdateProcesses(procs)
	err := cm.update()
	if err != nil {
		log.Info(err)
	}
}

type configTracker struct {
	Processes      map[ditypes.PID]*ditypes.ProcessInfo
	ConfigPath     string
	configCallback fileConfigCallback
	stopChannel    chan bool
}

func (ct *configTracker) Start() error {
	fw := util.NewFileWatcher(ct.ConfigPath)
	updateChan, err := fw.Watch()
	if err != nil {
		return fmt.Errorf("failed to watch config file %s: %s", ct.ConfigPath, err)
	}

	go func(updateChan <-chan []byte) {
	configUpdateLoop:
		for {
			select {
			case rawConfigBytes := <-updateChan:
				conf := map[string]map[string]rcConfig{}
				err = json.Unmarshal(rawConfigBytes, &conf)
				if err != nil {
					log.Infof("invalid config read from %s: %s", ct.ConfigPath, err)
					continue
				}
				ct.configCallback(conf)
			case <-ct.stopChannel:
				break configUpdateLoop
			}
		}
	}(updateChan)
	return nil
}

func (ct *configTracker) Stop() {
	ct.stopChannel <- true
}

// UpdateProcesses is the callback interface that ConfigTracker uses to consume the map of ProcessInfo's
// such that it's used whenever there's an update to the state of known service processes on the machine.
// It simply overwrites the previous state of known service processes with the new one
func (ct *configTracker) UpdateProcesses(procs ditypes.DIProcs) {
	current := procs
	old := ct.Processes
	if !reflect.DeepEqual(current, old) {
		ct.Processes = current
	}
}

func (cm *FileWatchingConfigManager) update() error {
	var updatedState = ditypes.NewDIProcs()
	for serviceName, configsByID := range cm.configs {
		for pid, proc := range cm.configTracker.Processes {
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
				for _, existingProbe := range cm.state[pid].GetProbes() {
					updatedProbe := procInfo.GetProbe(existingProbe.ID)
					if updatedProbe == nil {
						// delete old probes
						cm.state[pid].DeleteProbe(existingProbe.ID)
					}
				}
				for _, updatedProbe := range procInfo.GetProbes() {
					existingProbe := cm.state[pid].GetProbe(updatedProbe.ID)
					if !reflect.DeepEqual(existingProbe, updatedProbe) {
						// update existing probes that changed
						cm.callback(procInfo, updatedProbe)
					}
				}
			}
		}
		cm.state = updatedState
	}
	return nil
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
