// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.
package proctracker

import (
	"debug/elf"
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"syscall"

	"github.com/DataDog/datadog-agent/pkg/util/log"

	"github.com/DataDog/datadog-agent/pkg/dynamicinstrumentation/ditypes"
	"github.com/cilium/ebpf"
	"github.com/cilium/ebpf/link"

	"github.com/DataDog/datadog-agent/pkg/network/go/bininspect"
	"github.com/DataDog/datadog-agent/pkg/network/go/binversion"
	"github.com/DataDog/datadog-agent/pkg/process/monitor"
	"github.com/DataDog/datadog-agent/pkg/security/secl/model"
	"github.com/DataDog/datadog-agent/pkg/security/utils"
	"github.com/DataDog/datadog-agent/pkg/util/kernel"
	"golang.org/x/sys/unix"
)

type processTrackerCallback func(ditypes.DIProcs)

// adapted from https://github.com/DataDog/datadog-agent/blob/main/pkg/network/protocols/http/ebpf_gotls.go
type ProcessTracker struct {
	procRoot    string
	lock        sync.RWMutex
	pm          *monitor.ProcessMonitor
	processes   processes
	binaries    binaries
	callback    processTrackerCallback
	unsubscribe []func()
}

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

func (pt *ProcessTracker) inspectBinary(exePath string, pid uint32) {
	serviceName := getServiceName(pid)
	if serviceName == "" {
		// if the expected env vars are not set we don't inspect the binary
		return
	}
	log.Info("Found instrumentation candidate", serviceName)
	// binPath, err := os.Readlink(exePath)
	// if err != nil {
	// 	// /proc could be slow to update so we retry a few times
	// 	end := time.Now().Add(10 * time.Millisecond)
	// 	for end.After(time.Now()) {
	// 		binPath, err = os.Readlink(exePath)
	// 		if err == nil {
	// 			break
	// 		}
	// 		time.Sleep(time.Millisecond)
	// 	}
	// }
	// if err != nil {
	// 	// we can't access the binary path here (pid probably ended already)
	// 	// there is not much we can do, and we don't want to flood the logs
	// 	log.Infof("cannot follow link %s -> %s, %s", exePath, binPath, err)
	// 	// in docker, following the symlink does not work, but we can open the file in /proc
	// 	// if we can't follow the symlink we try to open /proc directly
	// 	// TODO: validate this approach
	// 	binPath = exePath
	// }

	// TODO: switch to using exePath for the demo, use conditional logic above moving forward
	binPath := exePath
	f, err := os.Open(exePath)
	if err != nil {
		// this should be a debug log, but we want to know if this happens
		log.Infof("could not open file %s, %s", binPath, err)
		return
	}
	defer f.Close()

	elfFile, err := elf.NewFile(f)
	if err != nil {
		log.Infof("file %s could not be parsed as an ELF file: %s", binPath, err)
		return
	}

	noFuncs := make(map[string]bininspect.FunctionConfiguration)
	noStructs := make(map[bininspect.FieldIdentifier]bininspect.StructLookupFunction)
	_, err = bininspect.InspectNewProcessBinary(elfFile, noFuncs, noStructs)
	if errors.Is(err, binversion.ErrNotGoExe) {
		return
	}
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
	pt.registerProcess(binID, pid, stat.Mtim, binPath, serviceName)
}

func (pt *ProcessTracker) registerProcess(binID binaryID, pid pid, mTime syscall.Timespec, binaryPath string, serviceName string) {
	pt.lock.Lock()
	defer pt.lock.Unlock()

	pt.processes[pid] = binID
	if bin, ok := pt.binaries[binID]; ok {
		// proccess that uses this binary already exists
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

func getServiceName(pid uint32) string {
	envVars, _, err := utils.EnvVars([]string{"DD"}, pid, model.MaxArgsEnvsSize)
	if err != nil {
		return ""
	}

	serviceName := ""
	diEnabled := false
	for _, envVar := range envVars {
		parts := strings.SplitN(envVar, "=", 2)
		if len(parts) == 2 && parts[0] == "DD_SERVICE" {
			serviceName = parts[1]
		}
		if len(parts) == 2 && parts[0] == "DD_DYNAMIC_INSTRUMENTATION_ENABLED" {
			diEnabled = parts[1] == "true"
		}
	}

	if !diEnabled {
		return ""
	}
	return serviceName
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
	bin.processCount -= 1
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
			PID:         pid,
			BinaryPath:  bin.binaryPath,
			ServiceName: bin.serviceName,

			ProbesByID:             make(map[ditypes.ProbeID]*ditypes.Probe),
			InstrumentationUprobes: make(map[ditypes.ProbeID]*link.Link),
			InstrumentationObjects: make(map[ditypes.ProbeID]*ebpf.Collection),
		}
	}
	return state
}
