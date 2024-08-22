// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.
package ditypes

import (
	"debug/dwarf"
	"fmt"
	"strconv"
	"strings"

	"github.com/DataDog/datadog-agent/pkg/util/log"

	"github.com/DataDog/datadog-agent/pkg/di/ratelimiter"
	"github.com/cilium/ebpf"
	"github.com/cilium/ebpf/link"
	"github.com/google/uuid"
)

const ConfigBPFProbeID = "config"

var (
	CaptureParameters       = true
	ArgumentsMaxSize        = 10000
	StringMaxSize           = 512
	MaxReferenceDepth uint8 = 4
	MaxFieldCount     int   = 20
	SliceMaxSize            = 1800
	SliceMaxLength          = 100
)

type ProbeID = string
type ServiceName = string
type PID = uint32

type DIProcs map[PID]*ProcessInfo

func NewDIProcs() DIProcs {
	return DIProcs{}
}

func (procs DIProcs) GetProbes(pid PID) []*Probe {
	procInfo, ok := procs[pid]
	if !ok {
		return nil
	}
	return procInfo.GetProbes()
}

func (procs DIProcs) GetProbe(pid PID, probeID ProbeID) *Probe {
	procInfo, ok := procs[pid]
	if !ok {
		return nil
	}
	return procInfo.GetProbe(probeID)
}

func (procs DIProcs) SetProbe(pid PID, service, typeName, method string, probeID, runtimeID uuid.UUID, opts *InstrumentationOptions) {
	procInfo, ok := procs[pid]
	if !ok {
		return
	}
	probe := &Probe{
		ID:                  probeID.String(),
		ServiceName:         service,
		FuncName:            fmt.Sprintf("%s.%s", typeName, method),
		InstrumentationInfo: &InstrumentationInfo{InstrumentationOptions: opts},
	}

	procInfo.ProbesByID[probeID.String()] = probe
	// TODO: remove this from here
	procInfo.RuntimeID = runtimeID.String()
}

func (procs DIProcs) DeleteProbe(pid PID, probeID ProbeID) {
	procInfo, ok := procs[pid]
	if !ok {
		return
	}
	procInfo.DeleteProbe(probeID)
}

func (procs DIProcs) CloseUprobe(pid PID, probeID ProbeID) {
	probe := procs.GetProbe(pid, probeID)
	if probe == nil {
		return
	}
	procs[pid].CloseUprobeLink(probeID)
}

func (procs DIProcs) SetRuntimeID(pid PID, runtimeID string) {
	procs[pid].RuntimeID = runtimeID
}

type ProcessInfo struct {
	PID         uint32
	ServiceName string
	RuntimeID   string
	BinaryPath  string

	TypeMap   *TypeMap
	DwarfData *dwarf.Data

	ConfigurationUprobe    *link.Link
	ProbesByID             ProbesByID
	InstrumentationUprobes map[ProbeID]*link.Link
	InstrumentationObjects map[ProbeID]*ebpf.Collection
}

func (pi *ProcessInfo) SetupConfigUprobe() (*ebpf.Map, error) {
	configProbe, ok := pi.ProbesByID[ConfigBPFProbeID]
	if !ok {
		return nil, fmt.Errorf("config probe was not set for process %s", pi.ServiceName)
	}

	configLink, ok := pi.InstrumentationUprobes[ConfigBPFProbeID]
	if !ok {
		return nil, fmt.Errorf("config uprobe was not set for process %s", pi.ServiceName)
	}
	pi.ConfigurationUprobe = configLink
	delete(pi.InstrumentationUprobes, ConfigBPFProbeID)

	m, ok := pi.InstrumentationObjects[configProbe.ID].Maps["events"]
	if !ok {
		return nil, fmt.Errorf("config ringbuffer was not set for process %s", pi.ServiceName)
	}
	return m, nil
}

func (pi *ProcessInfo) CloseConfigUprobe() error {
	if pi.ConfigurationUprobe != nil {
		return (*pi.ConfigurationUprobe).Close()
	}
	return nil
}

func (pi *ProcessInfo) SetUprobeLink(probeID ProbeID, l *link.Link) {
	pi.InstrumentationUprobes[probeID] = l
}

func (pi *ProcessInfo) CloseUprobeLink(probeID ProbeID) error {
	if l, ok := pi.InstrumentationUprobes[probeID]; ok {
		err := (*l).Close()
		delete(pi.InstrumentationUprobes, probeID)
		return err
	}
	return nil
}

func (pi *ProcessInfo) CloseAllUprobeLinks() {
	for probeID := range pi.InstrumentationUprobes {
		if err := pi.CloseUprobeLink(probeID); err != nil {
			log.Info("Failed to close uprobe link for probe", pi.BinaryPath, pi.PID, probeID, err)
		}
	}
	err := pi.CloseConfigUprobe()
	if err != nil {
		log.Info("Failed to close config uprobe for process", pi.BinaryPath, pi.PID, err)
	}
}

func (pi *ProcessInfo) GetProbes() []*Probe {
	probes := make([]*Probe, 0, len(pi.ProbesByID))
	for _, probe := range pi.ProbesByID {
		probes = append(probes, probe)
	}
	return probes
}

func (pi *ProcessInfo) GetProbe(probeID ProbeID) *Probe {
	return pi.ProbesByID[probeID]
}

func (pi *ProcessInfo) DeleteProbe(probeID ProbeID) {
	pi.CloseUprobeLink(probeID)
	delete(pi.ProbesByID, probeID)
}

type ProbesByID = map[ProbeID]*Probe

type FieldIdentifier struct {
	StructName, FieldName string
}

type InstrumentationInfo struct {
	InstrumentationOptions *InstrumentationOptions

	// BPFSourceCode is the source code of the BPF program attached via this probe
	BPFSourceCode string

	// BPFObjectFilePath is the path to the compiled BPF program attached via this probe
	BPFObjectFilePath string

	ConfigurationHash string

	// Toggle for whether or not the BPF object was rebuilt after changing parameters
	AttemptedRebuild bool
}

type InstrumentationOptions struct {
	CaptureParameters bool
	ArgumentsMaxSize  int
	StringMaxSize     int
	MaxReferenceDepth int
	MaxFieldCount     int
	SliceMaxSize      int
	SliceMaxLength    int
}

// Probe represents a location in a GoProgram that can be instrumented
// dynamically. It contains information about the service and the function
// associated with the probe.
type Probe struct {
	// ID is a unique identifier for the probe.
	ID string

	// ServiceName is the name of the service in which the probe should be placed.
	ServiceName string

	// FuncName is the name of the function that triggers the probe.
	FuncName string

	InstrumentationInfo *InstrumentationInfo

	RateLimiter *ratelimiter.SingleRateLimiter
}

func (p *Probe) GetBPFFuncName() string {
	// can't have '.', '-' or '/' in bpf program name
	replacer := strings.NewReplacer(".", "_", "/", "_", "-", "_")
	return replacer.Replace(p.FuncName)
}

type ConfigPath struct {
	OrgID     int64
	Product   string
	ProbeType string
	ProbeUUID uuid.UUID
	Hash      string
}

func ParseConfigPath(str string) (*ConfigPath, error) {
	// str is expected to be datadog/<org_id>/<product>/<probe_type>_<probe_uuid>/<hash>
	parts := strings.Split(str, "/")
	if len(parts) != 5 {
		return nil, fmt.Errorf("failed to parse config path %s", str)
	}
	orgIDStr, product, probeIDStr, hash := parts[1], parts[2], parts[3], parts[4]
	orgID, err := strconv.ParseInt(orgIDStr, 10, 64)
	if err != nil {
		return nil, fmt.Errorf("failed to parse orgID %s (from %s)", orgIDStr, str)
	}
	if product != "LIVE_DEBUGGING" {
		return nil, fmt.Errorf("product %s not supported (from %s)", product, str)
	}

	typeAndID := strings.Split(probeIDStr, "_")
	if len(typeAndID) != 2 {
		return nil, fmt.Errorf("failed to parse probe type and UUID %s (from %s)", probeIDStr, str)
	}
	probeType, probeUUIDStr := typeAndID[0], typeAndID[1]
	if probeType != "logProbe" {
		return nil, fmt.Errorf("probe type %s not supported (from %s)", probeType, str)
	}
	probeUUID, err := uuid.Parse(probeUUIDStr)
	if err != nil {
		return nil, fmt.Errorf("failed to parse probeUUID %s (from %s)", probeUUIDStr, str)
	}

	return &ConfigPath{
		OrgID:     orgID,
		Product:   product,
		ProbeType: probeType,
		ProbeUUID: probeUUID,
		Hash:      hash,
	}, nil
}
