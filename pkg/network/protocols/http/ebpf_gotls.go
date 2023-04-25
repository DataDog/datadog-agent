// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf
// +build linux_bpf

package http

import (
	"debug/elf"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"sync"
	"syscall"
	"time"

	"github.com/cilium/ebpf"
	"golang.org/x/sys/unix"

	manager "github.com/DataDog/ebpf-manager"

	"github.com/DataDog/datadog-agent/pkg/network/config"
	"github.com/DataDog/datadog-agent/pkg/network/go/bininspect"
	"github.com/DataDog/datadog-agent/pkg/network/go/binversion"
	"github.com/DataDog/datadog-agent/pkg/network/protocols/http/gotls"
	"github.com/DataDog/datadog-agent/pkg/network/protocols/http/gotls/lookup"
	libtelemetry "github.com/DataDog/datadog-agent/pkg/network/protocols/telemetry"
	errtelemetry "github.com/DataDog/datadog-agent/pkg/network/telemetry"
	"github.com/DataDog/datadog-agent/pkg/process/monitor"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

const (
	offsetsDataMap            = "offsets_data"
	goTLSReadArgsMap          = "go_tls_read_args"
	goTLSWriteArgsMap         = "go_tls_write_args"
	connectionTupleByGoTLSMap = "conn_tup_by_go_tls_conn"
)

type uprobeInfo struct {
	ebpfFunctionName string
	ebpfSection      string
}

type uprobesInfo struct {
	functionInfo *uprobeInfo
	returnInfo   *uprobeInfo
}

var functionToProbes = map[string]uprobesInfo{
	bininspect.ReadGoTLSFunc: {
		functionInfo: &uprobeInfo{
			ebpfFunctionName: "uprobe__crypto_tls_Conn_Read",
			ebpfSection:      "uprobe/crypto/tls.(*Conn).Read",
		},
		returnInfo: &uprobeInfo{
			ebpfFunctionName: "uprobe__crypto_tls_Conn_Read__return",
			ebpfSection:      "uprobe/crypto/tls.(*Conn).Read/return",
		},
	},
	bininspect.WriteGoTLSFunc: {
		functionInfo: &uprobeInfo{
			ebpfFunctionName: "uprobe__crypto_tls_Conn_Write",
			ebpfSection:      "uprobe/crypto/tls.(*Conn).Write",
		},
		returnInfo: &uprobeInfo{
			ebpfFunctionName: "uprobe__crypto_tls_Conn_Write__return",
			ebpfSection:      "uprobe/crypto/tls.(*Conn).Write/return",
		},
	},
	bininspect.CloseGoTLSFunc: {
		functionInfo: &uprobeInfo{
			ebpfFunctionName: "uprobe__crypto_tls_Conn_Close",
			ebpfSection:      "uprobe/crypto/tls.(*Conn).Close",
		},
	},
}

var functionsConfig = map[string]bininspect.FunctionConfiguration{
	bininspect.WriteGoTLSFunc: {
		IncludeReturnLocations: true,
		ParamLookupFunction:    lookup.GetWriteParams,
	},
	bininspect.ReadGoTLSFunc: {
		IncludeReturnLocations: true,
		ParamLookupFunction:    lookup.GetReadParams,
	},
	bininspect.CloseGoTLSFunc: {
		IncludeReturnLocations: false,
		ParamLookupFunction:    lookup.GetCloseParams,
	},
}

var structFieldsLookupFunctions = map[bininspect.FieldIdentifier]bininspect.StructLookupFunction{
	bininspect.StructOffsetTLSConn:     lookup.GetTLSConnInnerConnOffset,
	bininspect.StructOffsetTCPConn:     lookup.GetTCPConnInnerConnOffset,
	bininspect.StructOffsetNetConnFd:   lookup.GetConnFDOffset,
	bininspect.StructOffsetNetFdPfd:    lookup.GetNetFD_PFDOffset,
	bininspect.StructOffsetPollFdSysfd: lookup.GetFD_SysfdOffset,
}

type pid = uint32

type binaryID = gotls.TlsBinaryId

// runningBinary represents a binary currently being hooked
type runningBinary struct {
	// Inode number of the binary
	binID binaryID

	// IDs of the probes currently attached on the binary
	probeIDs []manager.ProbeIdentificationPair

	// Modification time of the hooked binary, at the time of hooking.
	mTime syscall.Timespec

	// Reference counter for the number of currently running processes for
	// this binary.
	processCount int32
}

type GoTLSProgram struct {
	cfg     *config.Config
	manager *errtelemetry.Manager

	// Path to the process/container's procfs
	procRoot string

	// Process monitor channels
	procMonitor struct {
		cleanupExec func()
		cleanupExit func()
	}

	lock sync.RWMutex

	// eBPF map holding the result of binary analysis, indexed by binaries'
	// inodes.
	offsetsDataMap *ebpf.Map

	// binaries keeps track of the currently hooked binary.
	binaries map[binaryID]*runningBinary

	// processes keeps track of the inode numbers of the hooked binaries
	// associated with running processes.
	processes map[pid]binaryID

	// binAnalysisMetric handles telemetry on the time spent doing binary
	// analysis
	binAnalysisMetric *libtelemetry.Metric
}

// Static evaluation to make sure we are not breaking the interface.
var _ subprogram = &GoTLSProgram{}

func newGoTLSProgram(c *config.Config) *GoTLSProgram {
	if !c.EnableHTTPSMonitoring || !c.EnableGoTLSSupport {
		return nil
	}

	if !HTTPSSupported(c) {
		log.Errorf("goTLS not supported by this platform")
		return nil
	}

	if !c.EnableRuntimeCompiler && !c.EnableCORE {
		log.Errorf("goTLS support requires runtime-compilation or CO-RE to be enabled")
		return nil
	}

	p := &GoTLSProgram{
		cfg:       c,
		procRoot:  c.ProcRoot,
		binaries:  make(map[binaryID]*runningBinary),
		processes: make(map[pid]binaryID),
	}

	p.binAnalysisMetric = libtelemetry.NewMetric("gotls.analysis_time", libtelemetry.OptStatsd)

	return p
}

func (p *GoTLSProgram) ConfigureManager(m *errtelemetry.Manager) {
	p.manager = m
	p.manager.Maps = append(p.manager.Maps, []*manager.Map{
		{Name: offsetsDataMap},
		{Name: goTLSReadArgsMap},
		{Name: goTLSWriteArgsMap},
	}...)
	// Hooks will be added in runtime for each binary
}

func (p *GoTLSProgram) ConfigureOptions(options *manager.Options) {
	options.MapSpecEditors[connectionTupleByGoTLSMap] = manager.MapSpecEditor{
		Type:       ebpf.Hash,
		MaxEntries: uint32(p.cfg.MaxTrackedConnections),
		EditorFlag: manager.EditMaxEntries,
	}
}

func (*GoTLSProgram) GetAllUndefinedProbes() []manager.ProbeIdentificationPair {
	probeList := make([]manager.ProbeIdentificationPair, 0)
	for _, probeInfo := range functionToProbes {
		if probeInfo.functionInfo != nil {
			probeList = append(probeList, probeInfo.functionInfo.getIdentificationPair())
		}

		if probeInfo.returnInfo != nil {
			probeList = append(probeList, probeInfo.returnInfo.getIdentificationPair())
		}
	}

	return probeList
}

func (p *GoTLSProgram) Start() {
	var err error
	p.offsetsDataMap, _, err = p.manager.GetMap(offsetsDataMap)
	if err != nil {
		log.Errorf("could not get offsets_data map: %s", err)
		return
	}

	mon := monitor.GetProcessMonitor()
	p.procMonitor.cleanupExec, err = mon.Subscribe(&monitor.ProcessCallback{
		Event:    monitor.EXEC,
		Metadata: monitor.ANY,
		Callback: p.handleProcessStart,
	})
	if err != nil {
		log.Errorf("failed to subscribe Exec process monitor error: %s", err)
		return
	}
	p.procMonitor.cleanupExit, err = mon.Subscribe(&monitor.ProcessCallback{
		Event:    monitor.EXIT,
		Metadata: monitor.ANY,
		Callback: p.handleProcessStop,
	})
	if err != nil {
		log.Errorf("failed to subscribe Exit process monitor error: %s", err)

		if p.procMonitor.cleanupExec != nil {
			p.procMonitor.cleanupExec()
		}
		if p.procMonitor.cleanupExit != nil {
			p.procMonitor.cleanupExit()
		}
	}

}

func (p *GoTLSProgram) Stop() {
	p.procMonitor.cleanupExec()
	p.procMonitor.cleanupExit()
}

func (p *GoTLSProgram) handleProcessStart(pid pid) {
	exePath := filepath.Join(p.procRoot, strconv.FormatUint(uint64(pid), 10), "exe")

	binPath, err := os.Readlink(exePath)
	if err != nil {
		// We receive the Exec event, /proc could be slow to update
		end := time.Now().Add(10 * time.Millisecond)
		for end.After(time.Now()) {
			binPath, err = os.Readlink(exePath)
			if err == nil {
				break
			}
			time.Sleep(time.Millisecond)
		}
	}
	if err != nil {
		// we can't access to the binary path here (pid probably ended already)
		// there are not much we can do, and we don't want to flood the logs
		return
	}
	// Getting the full path in the process' namespace.
	binPath = filepath.Join(p.procRoot, strconv.FormatUint(uint64(pid), 10), "root", binPath)

	var stat syscall.Stat_t
	if err = syscall.Stat(binPath, &stat); err != nil {
		log.Debugf("could not stat binary path %s: %s", binPath, err)
		return
	}
	binID := binaryID{
		Id_major: unix.Major(stat.Dev),
		Id_minor: unix.Minor(stat.Dev),
		Ino:      stat.Ino,
	}

	oldProcCount, bin, err := p.registerProcess(binID, pid, stat.Mtim)
	if err != nil {
		log.Warnf("could not register new process (%d) with binary %q: %s", pid, binPath, err)
		return
	}

	if oldProcCount == 0 {
		// This is a slow process so let's not halt the watcher while we
		// are doing this.
		go p.hookNewBinary(binID, binPath, pid, bin)
	}
}

func (p *GoTLSProgram) handleProcessStop(pid pid) {
	p.unregisterProcess(pid)
}

func (p *GoTLSProgram) hookNewBinary(binID binaryID, binPath string, pid pid, bin *runningBinary) {
	var err error
	defer func() {
		if err != nil {
			// report hooking issue only if we detect properly a golang binary
			if !errors.Is(err, binversion.ErrNotGoExe) {
				log.Debugf("could not hook new binary %q for process %d: %s", binPath, pid, err)
			}
			p.unregisterProcess(pid)
			return
		}
	}()

	start := time.Now()

	f, err := os.Open(binPath)
	if err != nil {
		err = fmt.Errorf("could not open file %s, %w", binPath, err)
		return
	}
	defer f.Close()

	elfFile, err := elf.NewFile(f)
	if err != nil {
		err = fmt.Errorf("file %s could not be parsed as an ELF file: %w", binPath, err)
		return
	}

	inspectionResult, err := bininspect.InspectNewProcessBinary(elfFile, functionsConfig, structFieldsLookupFunctions)
	if err != nil {
		err = fmt.Errorf("error reading exe: %w", err)
		return
	}

	p.lock.Lock()
	defer p.lock.Unlock()

	if bin.processCount == 0 {
		err = fmt.Errorf("process exited before hooks could be attached")
		return
	}

	if err = p.addInspectionResultToMap(binID, inspectionResult); err != nil {
		return
	}

	probeIDs, err := p.attachHooks(inspectionResult, binPath)
	if err != nil {
		p.removeInspectionResultFromMap(binID)
		err = fmt.Errorf("error while attaching hooks: %w", err)
		return
	}

	bin.probeIDs = probeIDs

	elapsed := time.Since(start)

	p.binAnalysisMetric.Set(elapsed.Milliseconds())
	log.Debugf("attached hooks on %s (%v) in %s", binPath, binID, elapsed)
}

func (p *GoTLSProgram) registerProcess(binID binaryID, pid pid, mTime syscall.Timespec) (int32, *runningBinary, error) {
	p.lock.Lock()
	defer p.lock.Unlock()

	bin, found := p.binaries[binID]
	if !found {
		bin = &runningBinary{
			binID: binID,
			mTime: mTime,
		}
		p.binaries[binID] = bin
	} else if mTime != bin.mTime {
		return 0, nil, fmt.Errorf("binary has been modified since it has been hooked.")
	}

	old := bin.processCount
	bin.processCount += 1

	p.processes[pid] = binID

	return old, bin, nil
}

func (p *GoTLSProgram) unregisterProcess(pid pid) {
	p.lock.Lock()
	defer p.lock.Unlock()

	binID, found := p.processes[pid]
	if !found {
		return
	}
	delete(p.processes, pid)

	bin, found := p.binaries[binID]
	if !found {
		return
	}
	bin.processCount -= 1

	if bin.processCount == 0 {
		p.unhookBinary(bin)
		delete(p.binaries, binID)
	}
}

// addInspectionResultToMap runs a binary inspection and adds the result to the
// map that's being read by the probes, indexed by the binary's inode number `ino`.
func (p *GoTLSProgram) addInspectionResultToMap(binID binaryID, result *bininspect.Result) error {
	offsetsData, err := inspectionResultToProbeData(result)
	if err != nil {
		return fmt.Errorf("error while parsing inspection result: %w", err)
	}

	err = p.offsetsDataMap.Put(binID, offsetsData)
	if err != nil {
		return fmt.Errorf("could not write binary inspection result to map for binID %v: %w", binID, err)
	}

	return nil
}

func (p *GoTLSProgram) removeInspectionResultFromMap(binID binaryID) {
	err := p.offsetsDataMap.Delete(binID)
	if err != nil {
		log.Errorf("could not remove inspection result from map for ino %v: %s", binID, err)
	}
}

func (p *GoTLSProgram) attachHooks(result *bininspect.Result, binPath string) (probeIDs []manager.ProbeIdentificationPair, err error) {
	pathID, err := newPathIdentifier(binPath)
	if err != nil {
		return probeIDs, fmt.Errorf("can't create path identifier for path %s : %s", binPath, err)
	}
	uid := getUID(pathID)
	defer func() {
		if err != nil {
			p.detachHooks(probeIDs)
		}
	}()

	for function, uprobes := range functionToProbes {
		if functionsConfig[function].IncludeReturnLocations && uprobes.returnInfo == nil {
			err = fmt.Errorf("function %q configured to include return locations but no return uprobes found in config", function)
			return
		}
		if functionsConfig[function].IncludeReturnLocations && uprobes.returnInfo != nil {
			for i, offset := range result.Functions[function].ReturnLocations {
				returnProbeID := manager.ProbeIdentificationPair{
					EBPFFuncName: uprobes.returnInfo.ebpfFunctionName,
					UID:          makeReturnUID(uid, i),
				}
				err = p.manager.AddHook("", &manager.Probe{
					ProbeIdentificationPair: returnProbeID,
					BinaryPath:              binPath,
					// Each return probe needs to have a unique uid value,
					// so add the index to the binary UID to make an overall UID.
					UprobeOffset: offset,
				})
				if err != nil {
					err = fmt.Errorf("could not add return hook to function %q in offset %d due to: %w", function, offset, err)
					return
				}
				probeIDs = append(probeIDs, returnProbeID)
			}
		}

		if uprobes.functionInfo != nil {
			probeID := manager.ProbeIdentificationPair{
				EBPFFuncName: uprobes.functionInfo.ebpfFunctionName,
				UID:          uid,
			}

			err = p.manager.AddHook("", &manager.Probe{
				BinaryPath:              binPath,
				UprobeOffset:            result.Functions[function].EntryLocation,
				ProbeIdentificationPair: probeID,
			})
			if err != nil {
				err = fmt.Errorf("could not add hook for %q in offset %d due to: %w", uprobes.functionInfo.ebpfFunctionName, result.Functions[function].EntryLocation, err)
				return
			}
			probeIDs = append(probeIDs, probeID)
		}
	}

	return
}
func (p *GoTLSProgram) unhookBinary(bin *runningBinary) {
	if bin.probeIDs == nil {
		// This binary was not hooked in the first place
		return
	}

	p.detachHooks(bin.probeIDs)
	p.removeInspectionResultFromMap(bin.binID)

	log.Debugf("detached hooks on ino %v", bin.binID)
}

func (p *GoTLSProgram) detachHooks(probeIDs []manager.ProbeIdentificationPair) {
	for _, probeID := range probeIDs {
		err := p.manager.DetachHook(probeID)
		if err != nil {
			log.Errorf("failed detaching hook %s: %s", probeID.UID, err)
		}
	}
}

func (i *uprobeInfo) getIdentificationPair() manager.ProbeIdentificationPair {
	return manager.ProbeIdentificationPair{
		EBPFFuncName: i.ebpfFunctionName,
	}
}
