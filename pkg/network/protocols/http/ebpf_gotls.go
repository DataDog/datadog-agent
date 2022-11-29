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
	"runtime"
	"strconv"
	"sync"
	"syscall"
	"time"

	"github.com/cilium/ebpf"
	"github.com/vishvananda/netlink"
	"golang.org/x/sys/unix"

	"github.com/DataDog/datadog-agent/pkg/network/config"
	"github.com/DataDog/datadog-agent/pkg/network/go/bininspect"
	"github.com/DataDog/datadog-agent/pkg/network/go/binversion"
	"github.com/DataDog/datadog-agent/pkg/network/protocols/http/gotls"
	"github.com/DataDog/datadog-agent/pkg/network/protocols/http/gotls/lookup"
	errtelemetry "github.com/DataDog/datadog-agent/pkg/network/telemetry"
	"github.com/DataDog/datadog-agent/pkg/process/util"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	manager "github.com/DataDog/ebpf-manager"
)

const (
	offsetsDataMap    = "offsets_data"
	goTLSReadArgsMap  = "go_tls_read_args"
	goTLSWriteArgsMap = "go_tls_write_args"
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
	manager *errtelemetry.Manager

	// Path to the process/container's procfs
	procRoot string

	// Process monitor channels
	procMonitor struct {
		done   chan struct{}
		events chan netlink.ProcEvent
		errors chan error
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
	binAnalysisMetric *errtelemetry.Metric
}

// Static evaluation to make sure we are not breaking the interface.
var _ subprogram = &GoTLSProgram{}

func supportedArch(arch string) bool {
	return arch == string(bininspect.GoArchX86_64)
}

func newGoTLSProgram(c *config.Config) *GoTLSProgram {
	if !c.EnableHTTPSMonitoring || !c.EnableGoTLSSupport {
		return nil
	}

	if !supportedArch(runtime.GOARCH) {
		log.Errorf("System arch %q is not supported for goTLS", runtime.GOARCH)
		return nil
	}

	if !c.EnableRuntimeCompiler {
		log.Errorf("goTLS support requires runtime-compilation to be enabled")
		return nil
	}

	p := &GoTLSProgram{
		procRoot:  c.ProcRoot,
		binaries:  make(map[binaryID]*runningBinary),
		processes: make(map[pid]binaryID),
	}

	p.procMonitor.done = make(chan struct{})
	p.procMonitor.events = make(chan netlink.ProcEvent, 1000)
	p.procMonitor.errors = make(chan error, 1)

	p.binAnalysisMetric = errtelemetry.NewMetric("gotls.analysis_time", errtelemetry.OptStatsd)

	return p
}

func (p *GoTLSProgram) ConfigureManager(m *errtelemetry.Manager) {
	if p == nil {
		return
	}

	p.manager = m
	p.manager.Maps = append(p.manager.Maps, []*manager.Map{
		{Name: offsetsDataMap},
		{Name: goTLSReadArgsMap},
		{Name: goTLSWriteArgsMap},
	}...)
	// Hooks will be added in runtime for each binary
}

func (p *GoTLSProgram) ConfigureOptions(options *manager.Options) {}

func (p *GoTLSProgram) GetAllUndefinedProbes() (probeList []manager.ProbeIdentificationPair) {
	for _, probeInfo := range functionToProbes {
		if probeInfo.functionInfo != nil {
			probeList = append(probeList, probeInfo.functionInfo.getIdentificationPair())
		}

		if probeInfo.returnInfo != nil {
			probeList = append(probeList, probeInfo.returnInfo.getIdentificationPair())
		}
	}

	return
}

func (p *GoTLSProgram) Start() {
	if p == nil {
		return
	}

	var err error
	p.offsetsDataMap, _, err = p.manager.GetMap(offsetsDataMap)
	if err != nil {
		log.Errorf("could not get offsets_data map: %s", err)
		return
	}

	if err := netlink.ProcEventMonitor(p.procMonitor.events, p.procMonitor.done, p.procMonitor.errors); err != nil {
		log.Errorf("could not create process monitor: %s", err)
		return
	}

	// This channel is used by the process watcher goroutine (just below) to
	// wait until we finished scanning for already running Go processes.
	// This is needed to avoid a race condition where an exit event is
	// processed during the registration of an already running process,
	// which would make the possible impossible to unregister afterwards,
	// causing a memory leak.
	startDone := make(chan interface{})

	// Process watcher events handling goroutine
	go func() {
		// Wait for the scanning of already running processes to complete
		<-startDone

		for {
			select {
			case <-p.procMonitor.done:
				return

			case event, ok := <-p.procMonitor.events:
				if !ok {
					return
				}

				switch ev := event.Msg.(type) {
				case *netlink.ExecProcEvent:
					p.handleProcessStart(ev.ProcessPid)
				case *netlink.ExitProcEvent:
					p.handleProcessStop(ev.ProcessPid)

					// No default case; the watcher has a
					// lot of event types, some of which
					// (e.g fork) happen all the time even
					// on an idle machine. Logging those
					// would flood our logs.
				}

			case err, ok := <-p.procMonitor.errors:
				if !ok {
					return
				}

				log.Errorf("process watcher error: %s", err)
			}
		}
	}()

	// Scan already running processes. We allow the process watcher to
	// process events afterwards.
	go func() {
		_ = util.WithAllProcs(p.procRoot, func(pid int) error {
			p.handleProcessStart(uint32(pid))
			return nil
		})
		close(startDone)
	}()
}

func (p *GoTLSProgram) Stop() {
	if p == nil {
		return
	}

	close(p.procMonitor.done)
}

func (p *GoTLSProgram) handleProcessStart(pid pid) {
	exePath := filepath.Join(p.procRoot, strconv.FormatUint(uint64(pid), 10), "exe")
	binPath, err := os.Readlink(exePath)
	if err != nil {
		log.Debugf(" could not read binary path for pid %d: %s", pid, err)
		return
	}

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
			log.Debugf("could not hook new binary %q for process %d: %s", binPath, pid, err)
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
		if !errors.Is(err, binversion.ErrNotGoExe) {
			err = fmt.Errorf("error reading exe: %w", err)
		}
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
		log.Debugf("no processes left for binID %v", bin.binID)
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
	uid := getUID(binPath)
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
					EBPFSection:  uprobes.returnInfo.ebpfSection,
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
				EBPFSection:  uprobes.functionInfo.ebpfSection,
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
		EBPFSection:  i.ebpfSection,
		EBPFFuncName: i.ebpfFunctionName,
	}
}
