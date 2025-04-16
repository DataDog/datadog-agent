// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf

// Package proctracker provides a facility for Dynamic Instrumentation to discover
// and track the lifecycle of processes running on the same host
package proctracker

import (
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"syscall"

	"golang.org/x/sys/unix"

	"github.com/DataDog/datadog-agent/pkg/dynamicinstrumentation/ditypes"
	"github.com/DataDog/datadog-agent/pkg/network/go/bininspect"
	"github.com/DataDog/datadog-agent/pkg/process/monitor"
	"github.com/DataDog/datadog-agent/pkg/security/secl/model/sharedconsts"
	"github.com/DataDog/datadog-agent/pkg/security/utils"
	"github.com/DataDog/datadog-agent/pkg/util/kernel"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/datadog-agent/pkg/util/safeelf"
	delve "github.com/go-delve/delve/pkg/goversion"
)

type processTrackerCallback func(ditypes.DIProcs)

// ProcessTracker is adapted from https://github.com/DataDog/datadog-agent/blob/main/pkg/network/protocols/http/ebpf_gotls.go
type ProcessTracker struct {
	procRoot    string
	lock        sync.RWMutex
	pm          *monitor.ProcessMonitor
	processes   processes
	binaries    binaries
	callback    processTrackerCallback
	unsubscribe []func()
}

// NewProcessTracker creates a new ProcessTracer
func NewProcessTracker(callback processTrackerCallback) *ProcessTracker {
	pt := ProcessTracker{
		pm:        monitor.GetProcessMonitor(),
		procRoot:  kernel.ProcFSRoot(),
		callback:  callback,
		binaries:  make(map[binaryID]*runningBinary),
		processes: make(map[pid]binaryID),
	}
	return &pt
}

// Start subscribes to exec and exit events so dynamic instrumentation can be made
// aware of new processes that may need to be instrumented or instrumented processes
// that should no longer be instrumented
func (pt *ProcessTracker) Start() error {

	unsubscribeExec := pt.pm.SubscribeExec(pt.handleProcessStart)
	unsubscribeExit := pt.pm.SubscribeExit(pt.handleProcessStop)

	pt.unsubscribe = append(pt.unsubscribe, unsubscribeExec)
	pt.unsubscribe = append(pt.unsubscribe, unsubscribeExit)

	err := pt.pm.Initialize(false)
	if err != nil {
		return err
	}

	return nil
}

// Stop unsubscribes from exec and exit events
func (pt *ProcessTracker) Stop() {
	for _, unsubscribe := range pt.unsubscribe {
		unsubscribe()
	}
}

func (pt *ProcessTracker) handleProcessStart(pid uint32) {
	exePath := filepath.Join(pt.procRoot, strconv.FormatUint(uint64(pid), 10), "exe")

	go pt.inspectBinary(exePath, pid)
}

func (pt *ProcessTracker) handleProcessStop(pid uint32) {
	pt.unregisterProcess(pid)
}

func remoteConfigCallback(_ delve.GoVersion, goarch string) ([]bininspect.ParameterMetadata, error) {
	if goarch != "arm64" && goarch != "amd64" {
		return nil, errors.New("invalid arch")
	}
	return []bininspect.ParameterMetadata{
		{
			TotalSize: 16,
			Kind:      0x18,
			Pieces: []bininspect.ParameterPiece{
				{Size: 8, InReg: true, StackOffset: 0, Register: 0},
				{Size: 8, InReg: true, StackOffset: 0, Register: 1},
			},
		},
		{
			TotalSize: 16,
			Kind:      0x18,
			Pieces: []bininspect.ParameterPiece{
				{Size: 8, InReg: true, StackOffset: 0, Register: 2},
				{Size: 8, InReg: true, StackOffset: 0, Register: 3},
			},
		},
		{
			TotalSize: 16,
			Kind:      0x18,
			Pieces: []bininspect.ParameterPiece{
				{Size: 8, InReg: true, StackOffset: 0, Register: 4},
				{Size: 8, InReg: true, StackOffset: 0, Register: 5},
			},
		}}, nil
}

func (pt *ProcessTracker) inspectBinary(exePath string, pid uint32) {
	// Avoid self-inspection.
	if int(pid) == os.Getpid() {
		return
	}

	serviceName, diEnabled := getEnvVars(pid)
	if serviceName == "" || !diEnabled {
		// if the expected env vars are not set we don't inspect the binary
		return
	}
	// TODO: switch to using exePath for the demo, use conditional logic above moving forward
	binPath := exePath
	f, err := os.Open(exePath)
	if err != nil {
		// this should be a debug log, but we want to know if this happens
		log.Infof("could not open file %s, %s", binPath, err)
		return
	}
	defer f.Close()

	elfFile, err := safeelf.NewFile(f)
	if err != nil {
		log.Infof("file %s could not be parsed as an ELF file: %s", binPath, err)
		return
	}
	noStructs := make(map[bininspect.FieldIdentifier]bininspect.StructLookupFunction)
	var functionsConfig = map[string]bininspect.FunctionConfiguration{
		ditypes.RemoteConfigCallback: {
			IncludeReturnLocations: false,
			ParamLookupFunction:    remoteConfigCallback,
		},
	}
	_, err = bininspect.InspectNewProcessBinary(elfFile, functionsConfig, noStructs)
	if err != nil {
		log.Infof("error reading exe: %s", err)
		return
	}

	var stat syscall.Stat_t
	if err = syscall.Stat(binPath, &stat); err != nil {
		log.Infof("could not stat binary path %s: %s", binPath, err)
		return
	}
	binID := binaryID{
		Id_major: unix.Major(stat.Dev),
		Id_minor: unix.Minor(stat.Dev),
		Ino:      stat.Ino,
	}
	log.Info("Found instrumentation candidate", serviceName)
	pt.registerProcess(binID, pid, stat.Mtim, binPath, serviceName)
}

func (pt *ProcessTracker) registerProcess(binID binaryID, pid pid, mTime syscall.Timespec, binaryPath string, serviceName string) {
	pt.lock.Lock()
	defer pt.lock.Unlock()

	pt.processes[pid] = binID
	if bin, ok := pt.binaries[binID]; ok {
		// process that uses this binary already exists
		bin.processCount++
	} else {

		pt.binaries[binID] = &runningBinary{
			binID:        binID,
			mTime:        mTime,
			processCount: 1,
			binaryPath:   binaryPath,
			serviceName:  serviceName,
		}
	}
	state := pt.currentState()
	pt.callback(state)
}

func getEnvVars(pid uint32) (serviceName string, diEnabled bool) {
	envVars, _, err := utils.EnvVars([]string{"DD"}, pid, sharedconsts.MaxArgsEnvsSize)
	if err != nil {
		return "", false
	}

	for _, envVar := range envVars {
		parts := strings.SplitN(envVar, "=", 2)
		if len(parts) == 2 && parts[0] == "DD_SERVICE" {
			serviceName = parts[1]
		}
		if len(parts) == 2 && parts[0] == "DD_DYNAMIC_INSTRUMENTATION_ENABLED" {
			diEnabled = parts[1] == "true"
		}
	}
	return serviceName, diEnabled
}

func (pt *ProcessTracker) unregisterProcess(pid pid) {
	pt.lock.Lock()
	defer pt.lock.Unlock()

	binID, ok := pt.processes[pid]
	if !ok {
		return
	}
	delete(pt.processes, pid)

	bin, ok := pt.binaries[binID]
	if !ok {
		return
	}
	bin.processCount--
	if bin.processCount == 0 {
		delete(pt.binaries, binID)
		state := pt.currentState()
		pt.callback(state)
	}
}

func (pt *ProcessTracker) currentState() map[ditypes.PID]*ditypes.ProcessInfo {
	state := make(map[ditypes.PID]*ditypes.ProcessInfo)

	for pid, binID := range pt.processes {
		bin := pt.binaries[binID]
		state[pid] = &ditypes.ProcessInfo{
			PID:                    pid,
			BinaryPath:             bin.binaryPath,
			ServiceName:            bin.serviceName,
			ProbesByID:             ditypes.NewProbesByID(),
			InstrumentationUprobes: ditypes.NewInstrumentationUprobesMap(),
			InstrumentationObjects: ditypes.NewInstrumentationObjectsMap(),
		}
	}
	return state
}
