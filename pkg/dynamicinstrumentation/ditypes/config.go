// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf

// Package ditypes contains various datatypes and otherwise shared components
// used by all the packages in dynamic instrumentation
package ditypes

import (
	"debug/dwarf"
	"fmt"
	"io"
	"strconv"
	"strings"

	"github.com/DataDog/datadog-agent/pkg/util/log"

	"github.com/DataDog/datadog-agent/pkg/dynamicinstrumentation/ratelimiter"
	"github.com/cilium/ebpf"
	"github.com/cilium/ebpf/link"
	"github.com/google/uuid"
)

const ConfigBPFProbeID = "config" // ConfigBPFProbeID is the ID used for the config bpf program

var (
	CaptureParameters       = true  // CaptureParameters is the default value for if probes should capture parameter values
	ArgumentsMaxSize        = 10000 // ArgumentsMaxSize is the default size in bytes of the output buffer used for param values
	StringMaxSize           = 512   // StringMaxSize is the default size in bytes of a single string
	MaxReferenceDepth uint8 = 4     // MaxReferenceDepth is the default depth that DI will traverse datatypes for capturing values
	MaxFieldCount           = 20    // MaxFieldCount is the default limit for how many fields DI will capture in a single data type
	SliceMaxSize            = 1800  // SliceMaxSize is the default limit in bytes of a slice
	SliceMaxLength          = 100   // SliceMaxLength is the default limit in number of elements of a slice
)

// ProbeID is the unique identifier for probes
type ProbeID = string

// ServiceName is the unique identifier for a service
type ServiceName = string

// PID stands for process ID
type PID = uint32

// DIProcs is the map that dynamic instrumentation uses for tracking processes and their relevant instrumentation info
type DIProcs map[PID]*ProcessInfo

// NewDIProcs creates a new DIProcs map
func NewDIProcs() DIProcs {
	return DIProcs{}
}

// GetProbes returns the relevant probes information for a specific process
func (procs DIProcs) GetProbes(pid PID) []*Probe {
	procInfo, ok := procs[pid]
	if !ok {
		return nil
	}
	return procInfo.GetProbes()
}

// GetProbe returns the relevant probe information for a specific probe being instrumented
// in a specific process
func (procs DIProcs) GetProbe(pid PID, probeID ProbeID) *Probe {
	procInfo, ok := procs[pid]
	if !ok {
		return nil
	}
	return procInfo.GetProbe(probeID)
}

// SetProbe associates instrumentation information with a probe for a specific process
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

// DeleteProbe removes instrumentation for the specified probe
// in the specified process
func (procs DIProcs) DeleteProbe(pid PID, probeID ProbeID) {
	procInfo, ok := procs[pid]
	if !ok {
		return
	}
	procInfo.DeleteProbe(probeID)
}

// CloseUprobe closes the uprobe link for the specific probe (by ID) of
// a the specified process (by PID)
func (procs DIProcs) CloseUprobe(pid PID, probeID ProbeID) {
	probe := procs.GetProbe(pid, probeID)
	if probe == nil {
		return
	}
	proc, ok := procs[pid]
	if !ok || proc == nil {
		log.Info("could not close uprobe, pid not found")
	}
	err := proc.CloseUprobeLink(probeID)
	if err != nil {
		log.Infof("could not close uprobe: %s", err)
	}
}

// SetRuntimeID sets the runtime ID for the specified process
func (procs DIProcs) SetRuntimeID(pid PID, runtimeID string) {
	proc, ok := procs[pid]
	if !ok || proc == nil {
		log.Info("could not set runtime ID, pid not found")
	}
	proc.RuntimeID = runtimeID
}

// ProcessInfo represents a process, it contains the information relevant to
// dynamic instrumentation for this specific process
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

// SetupConfigUprobe sets the configuration probe for the process
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

// CloseConfigUprobe closes the uprobe connection for the configuration probe
func (pi *ProcessInfo) CloseConfigUprobe() error {
	if pi.ConfigurationUprobe != nil {
		return (*pi.ConfigurationUprobe).Close()
	}
	return nil
}

// SetUprobeLink associates the uprobe link with the specified probe
// in the tracked process
func (pi *ProcessInfo) SetUprobeLink(probeID ProbeID, l *link.Link) {
	pi.InstrumentationUprobes[probeID] = l
}

// CloseUprobeLink closes the probe and deletes the link for the probe
// in the tracked process
func (pi *ProcessInfo) CloseUprobeLink(probeID ProbeID) error {
	if l, ok := pi.InstrumentationUprobes[probeID]; ok {
		err := (*l).Close()
		delete(pi.InstrumentationUprobes, probeID)
		return err
	}
	return nil
}

// CloseAllUprobeLinks closes all probes and deletes their links for all probes
// in the tracked process
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

// GetProbes returns references to each probe in the associated process
func (pi *ProcessInfo) GetProbes() []*Probe {
	probes := make([]*Probe, 0, len(pi.ProbesByID))
	for _, probe := range pi.ProbesByID {
		probes = append(probes, probe)
	}
	return probes
}

// GetProbe returns a reference to the specified probe in the associated process
func (pi *ProcessInfo) GetProbe(probeID ProbeID) *Probe {
	return pi.ProbesByID[probeID]
}

// DeleteProbe closes the uprobe link and disassociates the probe in the associated process
func (pi *ProcessInfo) DeleteProbe(probeID ProbeID) {
	err := pi.CloseUprobeLink(probeID)
	if err != nil {
		log.Infof("could not close uprobe link: %s", err)
	}
	delete(pi.ProbesByID, probeID)
}

// ProbesByID maps probe IDs with probes
type ProbesByID = map[ProbeID]*Probe

// FieldIdentifier is a tuple of struct names and field names
type FieldIdentifier struct {
	StructName, FieldName string
}

// InstrumentationInfo contains information used while setting up probes
type InstrumentationInfo struct {
	InstrumentationOptions *InstrumentationOptions

	// BPFParametersSourceCode is the source code needed for capturing parameters via this probe
	BPFParametersSourceCode string

	// BPFSourceCode is the source code of the BPF program attached via this probe
	BPFSourceCode string

	// BPFObjectFileReader is the compiled BPF program attached via this probe
	BPFObjectFileReader io.ReaderAt

	ConfigurationHash string

	// Toggle for whether or not the BPF object was rebuilt after changing parameters
	AttemptedRebuild bool
}

// InstrumentationOptions is a set of options for how data should be captured by probes
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

// GetBPFFuncName cleans the function name to be allowed by the bpf compiler
func (p *Probe) GetBPFFuncName() string {
	// can't have '.', '-' or '/' in bpf program name
	replacer := strings.NewReplacer(".", "_", "/", "_", "-", "_", "[", "_", "]", "_", "*", "ptr_", "(", "", ")", "")
	return replacer.Replace(p.FuncName)
}

// ConfigPath is a remote-config specific representation which is used for retrieving probe definitions
type ConfigPath struct {
	OrgID     int64
	Product   string
	ProbeType string
	ProbeUUID uuid.UUID
	Hash      string
}

// ParseConfigPath takes the remote-config specific string and parses a ConfigPath object out of it
// the string is expected to be datadog/<org_id>/<product>/<probe_type>_<probe_uuid>/<hash>
func ParseConfigPath(str string) (*ConfigPath, error) {
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
