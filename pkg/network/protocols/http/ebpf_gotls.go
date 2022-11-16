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

	"github.com/DataDog/datadog-agent/pkg/network/config"
	"github.com/DataDog/datadog-agent/pkg/network/go/bininspect"
	"github.com/DataDog/datadog-agent/pkg/network/go/binversion"
	"github.com/DataDog/datadog-agent/pkg/network/protocols/http/gotls/lookup"
	errtelemetry "github.com/DataDog/datadog-agent/pkg/network/telemetry"
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
		returnInfo: nil,
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
type inodeNumber = uint64
type runningProcessesSet map[pid]struct{}
type hookedBinariesMap map[inodeNumber]*hookedBinary

// hookedBinary represents a binary currently being hooked
type hookedBinary struct {
	// IDs of the probes currently attached on the binary
	probeIDs []manager.ProbeIdentificationPair
	// Set containing the PIDs of running processes spawned from this binary
	runningProcesses runningProcessesSet

	// Modification time of the hooked binary, at the time of hooking
	mTime syscall.Timespec
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

	// hookedBinaries keeps track of the currently hooked binary.
	hookedBinaries hookedBinariesMap

	// pidToIno keeps track of the inode numbers of the hooked binaries
	// associated with running processes.
	pidToIno map[pid]inodeNumber

	// binAnalysisMetric handles telemetry on the time spent doing binary
	// analysis
	binAnalysisMetric *errtelemetry.Metric
}

// Static evaluation to make sure we are not breaking the interface.
var _ subprogram = &GoTLSProgram{}

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
		procRoot:       c.ProcRoot,
		hookedBinaries: make(hookedBinariesMap),
		pidToIno:       make(map[pid]inodeNumber),
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

	go func() {
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
					go p.handleProcessStart(ev.ProcessPid)
				case *netlink.ExitProcEvent:
					go p.handleProcessStop(ev.ProcessPid)

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
}

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

func (p *GoTLSProgram) Stop() {
	if p == nil {
		return
	}

	close(p.procMonitor.done)
}

func supportedArch(arch string) bool {
	return arch == string(bininspect.GoArchX86_64)
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

	hookedBin := p.getHookedBinary(stat.Ino)
	if hookedBin == nil {
		hookedBin, err = p.hookNewBinary(binPath, stat.Ino)
		if err != nil {
			log.Debugf("could not hook new binary (%d): %s", pid, err)
			return
		}
		hookedBin.mTime = stat.Mtim
		p.setHookedBinary(stat.Ino, hookedBin)
	} else if stat.Mtim != hookedBin.mTime {
		log.Warnf("binary %q has been modified since it has been hooked, skipping process registration.", binPath)
		return
	}

	p.lock.Lock()
	defer p.lock.Unlock()
	hookedBin.runningProcesses[pid] = struct{}{}
	p.pidToIno[pid] = stat.Ino
}

func (p *GoTLSProgram) handleProcessStop(pid pid) {
	p.lock.Lock()
	defer p.lock.Unlock()

	ino, ok := p.pidToIno[pid]
	if !ok {
		return
	}
	delete(p.pidToIno, pid)

	hookedBin, present := p.hookedBinaries[ino]
	if !present {
		log.Error("could not retrieve hooked binary")
		return
	}

	delete(hookedBin.runningProcesses, pid)
	if len(hookedBin.runningProcesses) == 0 {
		log.Debugf("no processes left for ino %d", ino)
		p.unhookBinary(ino)
	}
}

func (p *GoTLSProgram) hookNewBinary(binPath string, ino inodeNumber) (*hookedBinary, error) {
	start := time.Now()

	f, err := os.Open(binPath)
	if err != nil {
		return nil, fmt.Errorf("could not open file %s, %w", binPath, err)
	}
	defer f.Close()

	elfFile, err := elf.NewFile(f)
	if err != nil {
		return nil, fmt.Errorf("file %s could not be parsed as an ELF file: %w", binPath, err)
	}

	inspectionResult, err := bininspect.InspectNewProcessBinary(elfFile, functionsConfig, structFieldsLookupFunctions)
	if err != nil {
		if !errors.Is(err, binversion.ErrNotGoExe) {
			err = fmt.Errorf("error reading exe: %w", err)
		}
		return nil, err
	}

	p.lock.Lock()
	defer p.lock.Unlock()
	if err = p.addInspectionResultToMap(inspectionResult, ino); err != nil {
		return nil, err
	}

	probeIDs, err := p.attachHooks(inspectionResult, binPath)
	if err != nil {
		p.removeInspectionResultFromMap(ino)
		return nil, fmt.Errorf("error while attaching hooks: %w", err)
	}

	elapsed := time.Since(start)

	p.binAnalysisMetric.Set(elapsed.Milliseconds())
	log.Debugf("attached hooks on %s (%d) in %s", binPath, ino, elapsed)

	return &hookedBinary{
		probeIDs:         probeIDs,
		runningProcesses: make(runningProcessesSet),
	}, nil
}

// addInspectionResultToMap runs a binary inspection and adds the result to the
// map that's being read by the probes, indexed by the binary's inode number `ino`.
func (p *GoTLSProgram) addInspectionResultToMap(result *bininspect.Result, ino inodeNumber) error {
	offsetsData, err := inspectionResultToProbeData(result)
	if err != nil {
		return fmt.Errorf("error while parsing inspection result: %w", err)
	}

	err = p.offsetsDataMap.Put(ino, offsetsData)
	if err != nil {
		return fmt.Errorf("could not write binary inspection result to map for ino %d: %w", ino, err)
	}

	return nil
}

func (p *GoTLSProgram) removeInspectionResultFromMap(ino inodeNumber) {
	err := p.offsetsDataMap.Delete(ino)
	if err != nil {
		log.Errorf("could not remove inspection result from map for ino %d: %s", ino, err)
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
func (p *GoTLSProgram) unhookBinary(ino inodeNumber) {
	bin := p.hookedBinaries[ino]

	p.detachHooks(bin.probeIDs)
	p.removeInspectionResultFromMap(ino)

	log.Debugf("detached hooks on ino %d", ino)
	delete(p.hookedBinaries, ino)
}

func (p *GoTLSProgram) detachHooks(probeIDs []manager.ProbeIdentificationPair) {
	for _, probeID := range probeIDs {
		err := p.manager.DetachHook(probeID)
		if err != nil {
			log.Errorf("failed detaching hook %s: %s", probeID.UID, err)
		}
	}
}

func (p *GoTLSProgram) getHookedBinary(ino inodeNumber) *hookedBinary {
	p.lock.RLock()
	defer p.lock.RUnlock()

	return p.hookedBinaries[ino]
}

func (p *GoTLSProgram) setHookedBinary(ino inodeNumber, bin *hookedBinary) {
	p.lock.Lock()
	defer p.lock.Unlock()

	p.hookedBinaries[ino] = bin
}

func (i *uprobeInfo) getIdentificationPair() manager.ProbeIdentificationPair {
	return manager.ProbeIdentificationPair{
		EBPFSection:  i.ebpfSection,
		EBPFFuncName: i.ebpfFunctionName,
	}
}
