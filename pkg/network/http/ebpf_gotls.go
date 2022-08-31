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
	"syscall"

	"github.com/cilium/ebpf"
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

type GoTLSProgram struct {
	manager *errtelemetry.Manager

	// Currently attached probes
	probeIDs []manager.ProbeIdentificationPair

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

	// Internal map holding the result of binary analysis. Used to determine
	// if analysis is needed when handling a new process' binary.
	inspected map[uint64]*bininspect.Result
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
		procRoot:  c.ProcRoot,
		inspected: make(map[uint64]*bininspect.Result),
	}

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
					p.handleNewBinary(ev.ProcessPid)
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

func (p *GoTLSProgram) handleNewBinary(pid uint32) {
	log.Debugf("New process with PID: %v", pid)

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

	result, ok := p.inspected[stat.Ino]
	if !ok {
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

		result, err = bininspect.InspectNewProcessBinary(elfFile, functionsConfig, structFieldsLookupFunctions)
		if err != nil {
			if !errors.Is(err, binversion.ErrNotGoExe) {
				log.Errorf("error reading exe: %s", err)
			}
			return
		}

		if err = p.addInspectionResultToMap(result, stat.Ino); err != nil {
			log.Error(err)
			return
		}

		p.inspected[stat.Ino] = result
	}

	if err := p.attachHooks(result, binPath); err != nil {
		log.Errorf("error while attaching hooks: %s", err)
		p.detachHooks()
	}
}

// addInspectionResultToMap runs a binary inspection and adds the result to the
// map that's being read by the probes, indexed by the binary's inode number `ino`.
func (p *GoTLSProgram) addInspectionResultToMap(result *bininspect.Result, ino uint64) error {
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

func (p *GoTLSProgram) attachHooks(result *bininspect.Result, binPath string) error {
	uid := getUID(binPath)

	for function, uprobes := range functionToProbes {
		if functionsConfig[function].IncludeReturnLocations && uprobes.returnInfo == nil {
			return fmt.Errorf("function %q configured to include return locations but no return uprobes found in config", function)
		}
		if functionsConfig[function].IncludeReturnLocations && uprobes.returnInfo != nil {
			for i, offset := range result.Functions[function].ReturnLocations {
				returnProbeID := manager.ProbeIdentificationPair{
					EBPFSection:  uprobes.returnInfo.ebpfSection,
					EBPFFuncName: uprobes.returnInfo.ebpfFunctionName,
					UID:          makeReturnUID(uid, i),
				}
				err := p.manager.AddHook("", &manager.Probe{
					ProbeIdentificationPair: returnProbeID,
					BinaryPath:              binPath,
					// Each return probe needs to have a unique uid value,
					// so add the index to the binary UID to make an overall UID.
					UprobeOffset: offset,
				})
				if err != nil {
					return fmt.Errorf("could not add return hook to function %q in offset %d due to: %w", function, offset, err)
				}
				p.probeIDs = append(p.probeIDs, returnProbeID)
			}
		}

		if uprobes.functionInfo != nil {
			probeID := manager.ProbeIdentificationPair{
				EBPFSection:  uprobes.functionInfo.ebpfSection,
				EBPFFuncName: uprobes.functionInfo.ebpfFunctionName,
				UID:          uid,
			}

			err := p.manager.AddHook("", &manager.Probe{
				BinaryPath:              binPath,
				UprobeOffset:            result.Functions[function].EntryLocation,
				ProbeIdentificationPair: probeID,
			})
			if err != nil {
				return fmt.Errorf("could not add hook for %q in offset %d due to: %w", uprobes.functionInfo.ebpfFunctionName, result.Functions[function].EntryLocation, err)
			}
			p.probeIDs = append(p.probeIDs, probeID)
		}
	}

	return nil
}

func (p *GoTLSProgram) detachHooks() {
	for _, probeID := range p.probeIDs {
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
}

func (i *uprobeInfo) getIdentificationPair() manager.ProbeIdentificationPair {
	return manager.ProbeIdentificationPair{
		EBPFSection:  i.ebpfSection,
		EBPFFuncName: i.ebpfFunctionName,
	}
}
