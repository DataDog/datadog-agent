// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf
// +build linux_bpf

package http

import (
	"debug/elf"
	"fmt"
	"os"
	"runtime"
	"strconv"
	"strings"

	"github.com/DataDog/datadog-agent/pkg/network/config"
	"github.com/DataDog/datadog-agent/pkg/network/go/bininspect"
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
	manager  *errtelemetry.Manager
	probeIDs []manager.ProbeIdentificationPair
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
	return &GoTLSProgram{}
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
	// In the future Start() should just initiate the new processes listener
	// and this implementation should be done for each new process found.
	binPath := os.Getenv("GO_TLS_TEST")
	if binPath != "" {
		p.handleNewBinary(binPath)
	}
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

func (p *GoTLSProgram) handleNewBinary(binPath string) {
	f, err := os.Open(binPath)
	if err != nil {
		log.Errorf("Could not open file %q due to %s", binPath, err)
		return
	}
	defer f.Close()
	elfFile, err := elf.NewFile(f)
	if err != nil {
		log.Errorf("File %q could not be parsed as elf: %s", binPath, err)
		return
	}

	result, err := bininspect.InspectNewProcessBinary(elfFile, functionsConfig, structFieldsLookupFunctions)
	if err != nil {
		log.Errorf("Failed inspecting binary %q: %s", binPath, err)
		return
	}

	// result and bin path are being passed as parameters as a preparation for the future when we will have a process
	// watcher, so we will run on more than one binary in one goTLSProgram.
	if err := p.addInspectionResultToMap(result, binPath); err != nil {
		log.Errorf("error in adding inspection result to map: %s", err)
		return
	}

	if err := p.attachHooks(result, binPath); err != nil {
		log.Errorf("error while attaching hooks: %s", err)
		p.detachHooks()
	}
}

// addInspectionResultToMap runs a binary inspection and adds the result to the map that's being read by the probes.
// It assumed the given path is from /proc dir and gets the pid from the path. It will fail otherwise.
// This assumption is temporary and we'll be removed once this code works in a process watcher.
func (p *GoTLSProgram) addInspectionResultToMap(result *bininspect.Result, binPath string) error {
	probeData, err := inspectionResultToProbeData(result)
	if err != nil {
		return fmt.Errorf("error while parsing inspection result: %w", err)
	}

	dataMap, _, err := p.manager.GetMap(offsetsDataMap)
	if err != nil {
		return fmt.Errorf("%q map not found: %w", offsetsDataMap, err)
	}

	// Map key is the pid, so it will be identified in the probe as the relevant data
	splitPath := strings.Split(binPath, "/")
	if len(splitPath) != 4 {
		// parts should be "", "proc", "<pid>", "exe"
		return fmt.Errorf("got an unexpected path format: %q, expected /proc/<pid>/exe", binPath)
	}
	// This assumption is temporary, until we'll have a process watcher
	pidStr := splitPath[2]
	pid, err := strconv.ParseInt(pidStr, 10, 32)
	if err != nil {
		return fmt.Errorf("failed extracting pid number for binary %q: %w", binPath, err)
	}
	err = dataMap.Put(uint32(pid), probeData)
	if err != nil {
		return fmt.Errorf("failed writing binary inspection result to map for binary %q: %w", binPath, err)
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
	// In the future, this should stop the new process listener.
	p.detachHooks()

}

func (i *uprobeInfo) getIdentificationPair() manager.ProbeIdentificationPair {
	return manager.ProbeIdentificationPair{
		EBPFSection:  i.ebpfSection,
		EBPFFuncName: i.ebpfFunctionName,
	}
}
