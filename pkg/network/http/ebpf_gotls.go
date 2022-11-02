// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf
// +build linux_bpf

package http

import (
	"context"
	"debug/elf"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"syscall"
	"time"

	"github.com/cilium/ebpf"
	"github.com/jellydator/ttlcache/v3"
	"github.com/vishvananda/netlink"

	"github.com/DataDog/datadog-agent/pkg/network/config"
	"github.com/DataDog/datadog-agent/pkg/network/go/bininspect"
	"github.com/DataDog/datadog-agent/pkg/network/go/binversion"
	"github.com/DataDog/datadog-agent/pkg/network/http/gotls/lookup"
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

type hookedBinary struct {
	probeIDs          []manager.ProbeIdentificationPair
	running_processes runningProcessesSet
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

	// eBPF map holding the result of binary analysis, indexed by binaries'
	// inodes.
	offsetsDataMap *ebpf.Map

	// hookedBinaries keeps track of the currently hooked binary.
	hookedBinaries ttlcache.Cache[inodeNumber, *hookedBinary]

	// pidToIno keeps track of the inode numbers of the hooked binaries
	// associated with running processes.
	pidToIno map[pid]inodeNumber
}

// Static evaluation to make sure we are not breaking the interface.
var _ subprogram = &GoTLSProgram{}

func newGoTLSProgram(c *config.Config) *GoTLSProgram {
	if !c.EnableHTTPSMonitoring {
		return nil
	}
	if !supportedArch(runtime.GOARCH) {
		log.Errorf("System arch %q is not supported for goTLS", runtime.GOARCH)
		return nil
	}
	p := &GoTLSProgram{
		procRoot: c.ProcRoot,
		hookedBinaries: *ttlcache.New(
			ttlcache.WithTTL[inodeNumber, *hookedBinary](2 * time.Minute),
		),
		pidToIno: make(map[pid]inodeNumber),
	}

	// Detach the hooks and cleanup old eBPF map entries on eviction
	p.hookedBinaries.OnEviction(p.cacheEntryCleanup)

	go p.hookedBinaries.Start()

	p.procMonitor.done = make(chan struct{})
	p.procMonitor.events = make(chan netlink.ProcEvent, 10)
	p.procMonitor.errors = make(chan error, 1)

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
		log.Errorf("could not stat binary path %s: %s", binPath, err)
		return
	}

	hookedBin := p.hookedBinaries.Get(stat.Ino)
	if hookedBin == nil {
		f, err := os.Open(binPath)
		if err != nil {
			log.Errorf("could not open file %s, %s", binPath, err)
			return
		}
		defer f.Close()

		elfFile, err := elf.NewFile(f)
		if err != nil {
			log.Errorf("file %s could not be parsed as an ELF file: %s", binPath, err)
			return
		}

		inspectionResult, err := bininspect.InspectNewProcessBinary(elfFile, functionsConfig, structFieldsLookupFunctions)
		if err != nil {
			if !errors.Is(err, binversion.ErrNotGoExe) {
				log.Errorf("error reading exe: %s", err)
			}
			return
		}

		if err = p.addInspectionResultToMap(inspectionResult, stat.Ino); err != nil {
			log.Error(err)
			return
		}

		probeIDs, err := p.attachHooks(inspectionResult, binPath)
		if err != nil {
			log.Errorf("error while attaching hooks: %s", err)
			p.removeInspectionResultFromMap(stat.Ino)
			return
		}
		log.Debugf("attached hooks on %s (%d)", binPath, stat.Ino)
		hookedBin = p.hookedBinaries.Set(stat.Ino, &hookedBinary{probeIDs, make(runningProcessesSet)}, ttlcache.NoTTL)
	} else {
		log.Debugf("resetting TTL on %s (%d)", binPath, stat.Ino)
		hookedBin = p.hookedBinaries.Set(stat.Ino, hookedBin.Value(), ttlcache.NoTTL)
	}

	hookedBin.Value().running_processes[pid] = struct{}{}
	p.pidToIno[pid] = stat.Ino
}

func (p *GoTLSProgram) handleProcessStop(pid pid) {
	ino, ok := p.pidToIno[pid]
	if !ok {
		return
	}
	delete(p.pidToIno, pid)

	hookedBin := p.hookedBinaries.Get(ino)
	if hookedBin == nil {
		log.Error("could not retrieve binary entry from cache")
		return
	}

	delete(hookedBin.Value().running_processes, pid)
	if len(hookedBin.Value().running_processes) == 0 {
		log.Debugf("no processes left for ino %d", ino)
		p.hookedBinaries.Set(ino, hookedBin.Value(), ttlcache.DefaultTTL)
	}

	return
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

func (p *GoTLSProgram) detachHooks(probeIDs []manager.ProbeIdentificationPair) {
	for _, probeID := range probeIDs {
		err := p.manager.DetachHook(probeID)
		if err != nil {
			log.Errorf("failed detaching hook %s: %s", probeID.UID, err)
		}
	}
}

func (p *GoTLSProgram) Stop() {
	if p == nil {
		return
	}

	close(p.procMonitor.done)
	p.hookedBinaries.Stop()
}

func (i *uprobeInfo) getIdentificationPair() manager.ProbeIdentificationPair {
	return manager.ProbeIdentificationPair{
		EBPFSection:  i.ebpfSection,
		EBPFFuncName: i.ebpfFunctionName,
	}
}

func (p *GoTLSProgram) cacheEntryCleanup(ctx context.Context, reason ttlcache.EvictionReason, i *ttlcache.Item[inodeNumber, *hookedBinary]) {
	if reason != ttlcache.EvictionReasonExpired {
		log.Warnf("unhandled eviction reason: %d", reason)
		return
	}

	p.detachHooks(i.Value().probeIDs)
	p.removeInspectionResultFromMap(i.Key())

	log.Debugf("detached hooks on ino %d", i.Key())
}
