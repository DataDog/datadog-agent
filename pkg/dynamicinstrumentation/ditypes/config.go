// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf

// Package ditypes contains various datatypes and otherwise shared components
// used by all the packages in dynamic instrumentation
package ditypes

import (
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
	StringMaxSize           = 60    // StringMaxSize is the length limit
	MaxReferenceDepth uint8 = 4     // MaxReferenceDepth is the default depth that DI will traverse datatypes for capturing values
	MaxFieldCount           = 20    // MaxFieldCount is the default limit for how many fields DI will capture in a single data type
	SliceMaxLength          = 5     // SliceMaxLength is the default limit in number of elements of a slice
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

// ProbesByID maps probe IDs with probes
type ProbesByID struct {
	probes map[ProbeID]*Probe
}

// NewProbesByID creates a new ProbesByID map
func NewProbesByID() *ProbesByID {
	return &ProbesByID{
		probes: make(map[ProbeID]*Probe),
	}
}

// Get returns a probe by ID
func (p *ProbesByID) Get(id ProbeID) *Probe {
	if p == nil || p.probes == nil {
		return nil
	}
	return p.probes[id]
}

// Set stores a probe by ID
func (p *ProbesByID) Set(id ProbeID, probe *Probe) {
	if p == nil {
		return
	}
	if p.probes == nil {
		p.probes = make(map[ProbeID]*Probe)
	}
	p.probes[id] = probe
}

// Delete removes a probe by ID
func (p *ProbesByID) Delete(id ProbeID) {
	if p == nil || p.probes == nil {
		return
	}
	delete(p.probes, id)
}

// Range calls f sequentially for each key and value present in the map
func (p *ProbesByID) Range(f func(id ProbeID, probe *Probe) bool) {
	if p == nil || p.probes == nil {
		return
	}
	for id, probe := range p.probes {
		if !f(id, probe) {
			break
		}
	}
}

// Len returns the number of items in the map
func (p *ProbesByID) Len() int {
	if p == nil || p.probes == nil {
		return 0
	}
	return len(p.probes)
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

	if procInfo.ProbesByID == nil {
		procInfo.ProbesByID = NewProbesByID()
	}
	procInfo.ProbesByID.Set(probeID.String(), probe)
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
		log.Errorf("could not close uprobe: %s", err)
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

// InstrumentationUprobesMap is a map for storing uprobe links
type InstrumentationUprobesMap struct {
	uprobes map[ProbeID]*link.Link
}

// NewInstrumentationUprobesMap creates a new InstrumentationUprobesMap
func NewInstrumentationUprobesMap() *InstrumentationUprobesMap {
	return &InstrumentationUprobesMap{
		uprobes: make(map[ProbeID]*link.Link),
	}
}

// Get returns an uprobe link by ID
func (p *InstrumentationUprobesMap) Get(id ProbeID) *link.Link {
	if p == nil || p.uprobes == nil {
		return nil
	}
	return p.uprobes[id]
}

// Set stores an uprobe link by ID
func (p *InstrumentationUprobesMap) Set(id ProbeID, l *link.Link) {
	if p == nil {
		return
	}
	if p.uprobes == nil {
		p.uprobes = make(map[ProbeID]*link.Link)
	}
	p.uprobes[id] = l
}

// Delete removes an uprobe link by ID
func (p *InstrumentationUprobesMap) Delete(id ProbeID) {
	if p == nil || p.uprobes == nil {
		return
	}
	delete(p.uprobes, id)
}

// Range calls f sequentially for each key and value present in the map
func (p *InstrumentationUprobesMap) Range(f func(id ProbeID, l *link.Link) bool) {
	if p == nil || p.uprobes == nil {
		return
	}
	for id, l := range p.uprobes {
		if !f(id, l) {
			break
		}
	}
}

// InstrumentationObjectsMap is a map for storing eBPF collections
type InstrumentationObjectsMap struct {
	objects map[ProbeID]*ebpf.Collection
}

// NewInstrumentationObjectsMap creates a new InstrumentationObjectsMap
func NewInstrumentationObjectsMap() *InstrumentationObjectsMap {
	return &InstrumentationObjectsMap{
		objects: make(map[ProbeID]*ebpf.Collection),
	}
}

// Get returns an eBPF collection by ID
func (p *InstrumentationObjectsMap) Get(id ProbeID) *ebpf.Collection {
	if p == nil || p.objects == nil {
		return nil
	}
	return p.objects[id]
}

// Set stores an eBPF collection by ID
func (p *InstrumentationObjectsMap) Set(id ProbeID, c *ebpf.Collection) {
	if p == nil {
		return
	}
	if p.objects == nil {
		p.objects = make(map[ProbeID]*ebpf.Collection)
	}
	p.objects[id] = c
}

// Delete removes an eBPF collection by ID
func (p *InstrumentationObjectsMap) Delete(id ProbeID) {
	if p == nil || p.objects == nil {
		return
	}
	delete(p.objects, id)
}

// Range calls f sequentially for each key and value present in the map
func (p *InstrumentationObjectsMap) Range(f func(id ProbeID, c *ebpf.Collection) bool) {
	if p == nil || p.objects == nil {
		return
	}
	for id, c := range p.objects {
		if !f(id, c) {
			break
		}
	}
}

// ProcessInfo represents a process, it contains the information relevant to
// dynamic instrumentation for this specific process
type ProcessInfo struct {
	PID         uint32
	ServiceName string
	RuntimeID   string
	BinaryPath  string

	TypeMap *TypeMap

	ConfigurationUprobe    *link.Link
	ProbesByID             *ProbesByID
	InstrumentationUprobes *InstrumentationUprobesMap
	InstrumentationObjects *InstrumentationObjectsMap
}

// SetupConfigUprobe sets the configuration probe for the process
func (pi *ProcessInfo) SetupConfigUprobe() (*ebpf.Map, error) {
	if pi == nil {
		return nil, fmt.Errorf("process info is nil")
	}
	if pi.ProbesByID == nil {
		return nil, fmt.Errorf("probes map not initialized for process %s", pi.ServiceName)
	}
	configProbe := pi.ProbesByID.Get(ConfigBPFProbeID)
	if configProbe == nil {
		return nil, fmt.Errorf("config probe was not set for process %s", pi.ServiceName)
	}

	if pi.InstrumentationUprobes == nil {
		return nil, fmt.Errorf("uprobes map not initialized for process %s", pi.ServiceName)
	}
	configLink := pi.InstrumentationUprobes.Get(ConfigBPFProbeID)
	if configLink == nil {
		return nil, fmt.Errorf("config uprobe was not set for process %s", pi.ServiceName)
	}
	pi.ConfigurationUprobe = configLink
	pi.InstrumentationUprobes.Delete(ConfigBPFProbeID)

	if pi.InstrumentationObjects == nil {
		return nil, fmt.Errorf("objects map not initialized for process %s", pi.ServiceName)
	}
	obj := pi.InstrumentationObjects.Get(configProbe.ID)
	if obj == nil {
		return nil, fmt.Errorf("config object was not set for process %s", pi.ServiceName)
	}
	m, ok := obj.Maps["events"]
	if !ok {
		return nil, fmt.Errorf("config ringbuffer was not set for process %s", pi.ServiceName)
	}
	return m, nil
}

// CloseConfigUprobe closes the uprobe connection for the configuration probe
func (pi *ProcessInfo) CloseConfigUprobe() error {
	if pi == nil || pi.ConfigurationUprobe == nil {
		return nil
	}
	return (*pi.ConfigurationUprobe).Close()
}

// SetUprobeLink associates the uprobe link with the specified probe
// in the tracked process
func (pi *ProcessInfo) SetUprobeLink(probeID ProbeID, l *link.Link) {
	if pi == nil {
		return
	}
	if pi.InstrumentationUprobes == nil {
		pi.InstrumentationUprobes = NewInstrumentationUprobesMap()
	}
	pi.InstrumentationUprobes.Set(probeID, l)
}

// CloseUprobeLink closes the probe and deletes the link for the probe
// in the tracked process
func (pi *ProcessInfo) CloseUprobeLink(probeID ProbeID) error {
	if pi == nil || pi.InstrumentationUprobes == nil {
		return nil
	}
	if l := pi.InstrumentationUprobes.Get(probeID); l != nil {
		err := (*l).Close()
		pi.InstrumentationUprobes.Delete(probeID)
		return err
	}
	return nil
}

// CloseAllUprobeLinks closes all probes and deletes their links for all probes
// in the tracked process
func (pi *ProcessInfo) CloseAllUprobeLinks() {
	if pi == nil || pi.InstrumentationUprobes == nil {
		return
	}
	pi.InstrumentationUprobes.Range(func(id ProbeID, _ *link.Link) bool {
		if err := pi.CloseUprobeLink(id); err != nil {
			log.Info("Failed to close uprobe link for probe", pi.BinaryPath, pi.PID, id, err)
		}
		return true
	})
	err := pi.CloseConfigUprobe()
	if err != nil {
		log.Info("Failed to close config uprobe for process", pi.BinaryPath, pi.PID, err)
	}
}

// GetProbes returns references to each probe in the associated process
func (pi *ProcessInfo) GetProbes() []*Probe {
	if pi == nil || pi.ProbesByID == nil {
		return nil
	}
	probes := make([]*Probe, 0, pi.ProbesByID.Len())
	pi.ProbesByID.Range(func(_ ProbeID, probe *Probe) bool {
		probes = append(probes, probe)
		return true
	})
	return probes
}

// GetProbe returns a reference to the specified probe in the associated process
func (pi *ProcessInfo) GetProbe(probeID ProbeID) *Probe {
	if pi == nil || pi.ProbesByID == nil {
		return nil
	}
	return pi.ProbesByID.Get(probeID)
}

// DeleteProbe closes the uprobe link and disassociates the probe in the associated process
func (pi *ProcessInfo) DeleteProbe(probeID ProbeID) {
	if pi == nil {
		return
	}
	err := pi.CloseUprobeLink(probeID)
	if err != nil {
		log.Errorf("could not close uprobe link: %s", err)
	}
	if pi.ProbesByID != nil {
		pi.ProbesByID.Delete(probeID)
	}
}

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
