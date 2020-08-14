// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

// +build linux_bpf

package probe

import (
	"fmt"
	"math"
	"strings"

	"github.com/DataDog/datadog-go/statsd"
	"github.com/iovisor/gobpf/elf"

	"github.com/DataDog/datadog-agent/pkg/ebpf/bytecode"
	"github.com/DataDog/datadog-agent/pkg/security/config"
	"github.com/DataDog/datadog-agent/pkg/security/ebpf"
	"github.com/DataDog/datadog-agent/pkg/security/rules"
	"github.com/DataDog/datadog-agent/pkg/security/secl/eval"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

const (
	// MetricPrefix is the prefix of the metrics sent by the runtime security agent
	MetricPrefix = "datadog.runtime_security"
)

// EventHandler represents an handler for the events sent by the probe
type EventHandler interface {
	HandleEvent(event *Event)
}

// Discarder represents a discarder whose a value for a field that
type Discarder struct {
	Field eval.Field
	Value interface{}
}

type onApproversFnc func(probe *Probe, approvers rules.Approvers) error
type onDiscarderFnc func(rs *rules.RuleSet, event *Event, probe *Probe, discarder Discarder) error

// Probe represents the runtime security eBPF probe in charge of
// setting up the required kProbes and decoding events sent from the kernel
type Probe struct {
	*ebpf.Probe
	config           *config.Config
	handler          EventHandler
	resolvers        *Resolvers
	onDiscardersFncs map[eval.EventType][]onDiscarderFnc
	enableFilters    bool
	tables           map[string]*ebpf.Table
	eventsStats      EventsStats
	syscallMonitor   *SyscallMonitor
}

// Capability represents the type of values we are able to filter kernel side
type Capability struct {
	PolicyFlags     PolicyFlag
	FieldValueTypes eval.FieldValueType
}

// Capabilities represents the filtering capabilities for a set of fields
type Capabilities map[eval.Field]Capability

// HookPoint represents
type HookPoint struct {
	Name            string
	KProbes         []*ebpf.KProbe
	Tracepoint      string
	Optional        bool
	EventTypes      map[eval.EventType]Capabilities
	OnNewApprovers  onApproversFnc
	OnNewDiscarders onDiscarderFnc
	PolicyTable     string
}

// cache of the syscall prefix depending on kernel version
var syscallPrefix string

func getSyscallFnName(name string) string {
	if syscallPrefix == "" {
		syscall, err := elf.GetSyscallFnName("open")
		if err != nil {
			panic(err)
		}
		syscallPrefix = strings.TrimSuffix(syscall, "open")
	}

	return syscallPrefix + name
}

func syscallKprobe(name string) []*ebpf.KProbe {
	return []*ebpf.KProbe{{
		EntryFunc: "kprobe/" + getSyscallFnName(name),
		ExitFunc:  "kretprobe/" + getSyscallFnName(name),
	}}
}

var allHookPoints = []*HookPoint{
	{
		Name: "security_inode_setattr",
		KProbes: []*ebpf.KProbe{{
			EntryFunc: "kprobe/security_inode_setattr",
		}},
		EventTypes: map[eval.EventType]Capabilities{
			"chmod":  {},
			"chown":  {},
			"utimes": {},
		},
	},
	{
		Name:    "sys_chmod",
		KProbes: syscallKprobe("chmod"),
		EventTypes: map[eval.EventType]Capabilities{
			"chmod": {},
		},
	},
	{
		Name:    "sys_fchmod",
		KProbes: syscallKprobe("fchmod"),
		EventTypes: map[eval.EventType]Capabilities{
			"chmod": {},
		},
	},
	{
		Name:    "sys_fchmodat",
		KProbes: syscallKprobe("fchmodat"),
		EventTypes: map[eval.EventType]Capabilities{
			"chmod": {},
		},
	},
	{
		Name:    "sys_chown",
		KProbes: syscallKprobe("chown"),
		EventTypes: map[eval.EventType]Capabilities{
			"chown": {},
		},
	},
	{
		Name:    "sys_fchown",
		KProbes: syscallKprobe("fchown"),
		EventTypes: map[eval.EventType]Capabilities{
			"chown": {},
		},
	},
	{
		Name:    "sys_fchownat",
		KProbes: syscallKprobe("fchownat"),
		EventTypes: map[eval.EventType]Capabilities{
			"chown": {},
		},
	},
	{
		Name:    "sys_lchown",
		KProbes: syscallKprobe("lchown"),
		EventTypes: map[eval.EventType]Capabilities{
			"chown": {},
		},
	},
	{
		Name: "mnt_want_write",
		KProbes: []*ebpf.KProbe{{
			EntryFunc: "kprobe/mnt_want_write",
		}},
		EventTypes: map[eval.EventType]Capabilities{
			"utimes": {},
			"chmod":  {},
			"chown":  {},
			"rmdir":  {},
			"unlink": {},
			"rename": {},
		},
	},
	{
		Name: "mnt_want_write_file",
		KProbes: []*ebpf.KProbe{{
			EntryFunc: "kprobe/mnt_want_write_file",
		}},
		EventTypes: map[eval.EventType]Capabilities{
			"chown": {},
		},
	},
	{
		Name:    "sys_utime",
		KProbes: syscallKprobe("utime"),
		EventTypes: map[eval.EventType]Capabilities{
			"utimes": {},
		},
	},
	{
		Name:    "sys_utimes",
		KProbes: syscallKprobe("utimes"),
		EventTypes: map[eval.EventType]Capabilities{
			"utimes": {},
		},
	},
	{
		Name:    "sys_utimensat",
		KProbes: syscallKprobe("utimensat"),
		EventTypes: map[eval.EventType]Capabilities{
			"utimes": {},
		},
	},
	{
		Name:    "sys_futimesat",
		KProbes: syscallKprobe("futimesat"),
		EventTypes: map[string]Capabilities{
			"utimes": {},
		},
	},
	{
		Name: "vfs_mkdir",
		KProbes: []*ebpf.KProbe{{
			EntryFunc: "kprobe/vfs_mkdir",
		}},
		EventTypes: map[eval.EventType]Capabilities{
			"mkdir": {},
		},
	},
	{
		Name: "filename_create",
		KProbes: []*ebpf.KProbe{{
			EntryFunc: "kprobe/filename_create",
		}},
		EventTypes: map[string]Capabilities{
			"mkdir": {},
			"link":  {},
		},
	},
	{
		Name:    "sys_mkdir",
		KProbes: syscallKprobe("mkdir"),
		EventTypes: map[eval.EventType]Capabilities{
			"mkdir": {},
		},
	},
	{
		Name:    "sys_mkdirat",
		KProbes: syscallKprobe("mkdirat"),
		EventTypes: map[eval.EventType]Capabilities{
			"mkdir": {},
		},
	},
	{
		Name: "vfs_rmdir",
		KProbes: []*ebpf.KProbe{{
			EntryFunc: "kprobe/vfs_rmdir",
		}},
		EventTypes: map[string]Capabilities{
			"rmdir":  {},
			"unlink": {},
		},
	},
	{
		Name:    "sys_rmdir",
		KProbes: syscallKprobe("rmdir"),
		EventTypes: map[eval.EventType]Capabilities{
			"rmdir": {},
		},
	},
	{
		Name: "vfs_rename",
		KProbes: []*ebpf.KProbe{{
			EntryFunc: "kprobe/vfs_rename",
		}},
		EventTypes: map[eval.EventType]Capabilities{
			"rename": {},
		},
	},
	{
		Name:    "sys_rename",
		KProbes: syscallKprobe("rename"),
		EventTypes: map[string]Capabilities{
			"rename": {},
		},
	},
	{
		Name:    "sys_renameat",
		KProbes: syscallKprobe("renameat"),
		EventTypes: map[eval.EventType]Capabilities{
			"rename": {},
		},
	},
	{
		Name:    "sys_renameat2",
		KProbes: syscallKprobe("renameat2"),
		EventTypes: map[eval.EventType]Capabilities{
			"rename": {},
		},
	},
	{
		Name: "vfs_link",
		KProbes: []*ebpf.KProbe{{
			EntryFunc: "kprobe/vfs_link",
		}},
		EventTypes: map[string]Capabilities{
			"link": {},
		},
	},
	{
		Name:    "sys_link",
		KProbes: syscallKprobe("link"),
		EventTypes: map[eval.EventType]Capabilities{
			"link": {},
		},
	},
	{
		Name:    "sys_linkat",
		KProbes: syscallKprobe("linkat"),
		EventTypes: map[eval.EventType]Capabilities{
			"link": {},
		},
	},
}

// GetFlags returns the policy flags for the set of capabilities
func (caps Capabilities) GetFlags() PolicyFlag {
	var flags PolicyFlag
	for _, cap := range caps {
		flags |= cap.PolicyFlags
	}
	return flags
}

// GetFields returns the fields associated with a set of capabilities
func (caps Capabilities) GetFields() []eval.Field {
	var fields []eval.Field

	for field := range caps {
		fields = append(fields, field)
	}

	return fields
}

// GetFieldCapabilities returns the field capabilities for a set of capabilities
func (caps Capabilities) GetFieldCapabilities() rules.FieldCapabilities {
	var fcs rules.FieldCapabilities

	for field, cap := range caps {
		fcs = append(fcs, rules.FieldCapability{
			Field: field,
			Types: cap.FieldValueTypes,
		})
	}

	return fcs
}

// NewRuleSet returns a new rule set
func (p *Probe) NewRuleSet(opts *rules.Opts) *rules.RuleSet {
	eventCtor := func() eval.Event {
		return NewEvent(p.resolvers)
	}

	return rules.NewRuleSet(&Model{}, eventCtor, opts)
}

func (p *Probe) getTableNames() []string {
	tables := []string{
		"pathnames",
		"noisy_processes_buffer",
		"noisy_processes_fb",
		"noisy_processes_bb",
	}

	tables = append(tables, openTables...)
	tables = append(tables, execTables...)
	tables = append(tables, unlinkTables...)

	return tables
}

// Table returns either an eprobe Table or a LRU based eprobe Table
func (p *Probe) Table(name string) *ebpf.Table {
	if table, exists := p.tables[name]; exists {
		return table
	}

	return p.Probe.Table(name)
}

func (p *Probe) getPerfMaps() []*ebpf.PerfMapDefinition {
	return []*ebpf.PerfMapDefinition{
		{
			Name:        "events",
			Handler:     p.handleEvent,
			LostHandler: p.handleLostEvents,
		},
		{
			Name:        "mountpoints_events",
			Handler:     p.handleEvent,
			LostHandler: p.handleLostEvents,
		},
	}
}

// Start the runtime security probe
func (p *Probe) Start() error {
	asset := "pkg/security/ebpf/c/runtime-security"
	openSyscall := getSyscallFnName("open")
	if !strings.HasPrefix(openSyscall, "SyS_") && !strings.HasPrefix(openSyscall, "sys_") {
		asset += "-syscall-wrapper"
	}

	bytecodeReader, err := bytecode.GetReader(p.config.BPFDir, asset+".o")
	if err != nil {
		return err
	}

	p.Module, err = ebpf.NewModuleFromReader(bytecodeReader)
	if err != nil {
		return err
	}

	if err = p.Load(); err != nil {
		return err
	}

	if err := p.resolvers.Start(); err != nil {
		return err
	}

	if p.config.SyscallMonitor {
		p.syscallMonitor, err = NewSyscallMonitor(
			p.Module,
			p.Table("noisy_processes_buffer"),
			p.Table("noisy_processes_fb"),
			p.Table("noisy_processes_bb"),
		)
		if err != nil {
			return err
		}
	}

	for _, hookpoint := range allHookPoints {
		if hookpoint.EventTypes == nil {
			continue
		}

		for eventType := range hookpoint.EventTypes {
			if hookpoint.OnNewDiscarders != nil {
				fncs := p.onDiscardersFncs[eventType]
				fncs = append(fncs, hookpoint.OnNewDiscarders)
				p.onDiscardersFncs[eventType] = fncs
			}
		}
	}

	return p.Probe.Start()
}

// SetEventHandler set the probe event handler
func (p *Probe) SetEventHandler(handler EventHandler) {
	p.handler = handler
}

// DispatchEvent sends an event to probe event handler
func (p *Probe) DispatchEvent(event *Event) {
	if p.handler != nil {
		p.handler.HandleEvent(event)
	}
}

// SendStats sends statistics about the probe to Datadog
func (p *Probe) SendStats(statsdClient *statsd.Client) error {
	if p.syscallMonitor != nil {
		if err := p.syscallMonitor.SendStats(statsdClient); err != nil {
			return err
		}
	}

	if err := statsdClient.Count(MetricPrefix+".events.lost", p.eventsStats.GetAndResetLost(), nil, 1.0); err != nil {
		return err
	}

	receivedEvents := MetricPrefix + ".events.received"
	for i := range p.eventsStats.PerEventType {
		if i == 0 {
			continue
		}

		eventType := EventType(i)
		tags := []string{fmt.Sprintf("event_type:%s", eventType.String())}
		if value := p.eventsStats.GetAndResetEventCount(eventType); value > 0 {
			if err := statsdClient.Count(receivedEvents, value, tags, 1.0); err != nil {
				return err
			}
		}
	}

	return nil
}

// GetStats returns Stats according to the system-probe module format
func (p *Probe) GetStats() (map[string]interface{}, error) {
	stats := make(map[string]interface{})

	var syscalls *SyscallStats
	var err error

	if p.syscallMonitor != nil {
		syscalls, err = p.syscallMonitor.GetStats()
	}

	stats["events"] = map[string]interface{}{
		"lost":     p.eventsStats.GetLost(),
		"syscalls": syscalls,
	}

	perEventType := make(map[string]int64)
	stats["per_event_type"] = perEventType
	for i := range p.eventsStats.PerEventType {
		if i == 0 {
			continue
		}

		eventType := EventType(i)
		perEventType[eventType.String()] = p.eventsStats.GetEventCount(eventType)
	}

	return stats, err
}

// GetEventsStats returns statistics about the events received by the probe
func (p *Probe) GetEventsStats() EventsStats {
	return p.eventsStats
}

func (p *Probe) handleLostEvents(count uint64) {
	log.Warnf("lost %d events\n", count)
	p.eventsStats.CountLost(int64(count))
}

func (p *Probe) handleEvent(data []byte) {
	offset := 0
	event := NewEvent(p.resolvers)

	read, err := event.UnmarshalBinary(data)
	if err != nil {
		log.Errorf("failed to decode event: %s", err)
		return
	}
	offset += read

	eventType := EventType(event.Type)
	log.Tracef("Decoding event %s", eventType.String())

	switch eventType {
	case FileOpenEventType:
		if _, err := event.Open.UnmarshalBinary(data[offset:]); err != nil {
			log.Errorf("failed to decode open event: %s (offset %d, len %d)", err, offset, len(data))
			return
		}
		event.Open.ResolveInode(p.resolvers)
	case FileMkdirEventType:
		if _, err := event.Mkdir.UnmarshalBinary(data[offset:]); err != nil {
			log.Errorf("failed to decode mkdir event: %s (offset %d, len %d)", err, offset, len(data))
			return
		}
	case FileRmdirEventType:
		if _, err := event.Rmdir.UnmarshalBinary(data[offset:]); err != nil {
			log.Errorf("failed to decode rmdir event: %s (offset %d, len %d)", err, offset, len(data))
			return
		}
	case FileUnlinkEventType:
		if _, err := event.Unlink.UnmarshalBinary(data[offset:]); err != nil {
			log.Errorf("failed to decode unlink event: %s (offset %d, len %d)", err, offset, len(data))
			return
		}
	case FileRenameEventType:
		if _, err := event.Rename.UnmarshalBinary(data[offset:]); err != nil {
			log.Errorf("failed to decode rename event: %s (offset %d, len %d)", err, offset, len(data))
			return
		}
	case FileChmodEventType:
		if _, err := event.Chmod.UnmarshalBinary(data[offset:]); err != nil {
			log.Errorf("failed to decode chmod event: %s (offset %d, len %d)", err, offset, len(data))
			return
		}
	case FileChownEventType:
		if _, err := event.Chown.UnmarshalBinary(data[offset:]); err != nil {
			log.Errorf("failed to decode chown event: %s (offset %d, len %d)", err, offset, len(data))
			return
		}
	case FileUtimeEventType:
		if _, err := event.Utimes.UnmarshalBinary(data[offset:]); err != nil {
			log.Errorf("failed to decode utime event: %s (offset %d, len %d)", err, offset, len(data))
			return
		}
	case FileLinkEventType:
		if _, err := event.Link.UnmarshalBinary(data[offset:]); err != nil {
			log.Errorf("failed to decode link event: %s (offset %d, len %d)", err, offset, len(data))
			return
		}
	case FileMountEventType:
		if _, err := event.Mount.UnmarshalBinary(data[offset:]); err != nil {
			log.Errorf("failed to decode mount event: %s (offset %d, len %d)", err, offset, len(data))
			return
		}
		// Resolve mount point
		event.Mount.ResolveMountPoint(p.resolvers)
		// Resolve root
		event.Mount.ResolveRoot(p.resolvers)
		// Insert new mount point in cache
		p.resolvers.MountResolver.Insert(&event.Mount)
	case FileUmountEventType:
		if _, err := event.Umount.UnmarshalBinary(data[offset:]); err != nil {
			log.Errorf("failed to decode umount event: %s (offset %d, len %d)", err, offset, len(data))
			return
		}
		// Delete new mount point from cache
		if err := p.resolvers.MountResolver.Delete(event.Umount.MountID); err != nil {
			log.Errorf("failed to delete mount point %d from cache: %s", event.Umount.MountID, err)
		}
	default:
		log.Errorf("unsupported event type %d", eventType)
		return
	}

	p.eventsStats.CountEventType(eventType, 1)

	log.Tracef("Dispatching event %+v\n", event)
	p.DispatchEvent(event)
}

// OnNewDiscarder is called when a new discarder is found
func (p *Probe) OnNewDiscarder(rs *rules.RuleSet, event *Event, field eval.Field) error {
	// discarders disabled
	if !p.config.EnableDiscarders {
		return nil
	}

	log.Tracef("New discarder event %+v for field %s\n", event, field)

	eventType, err := event.GetFieldEventType(field)
	if err != nil {
		return err
	}

	for _, fnc := range p.onDiscardersFncs[eventType] {
		value, err := event.GetFieldValue(field)
		if err != nil {
			return err
		}

		discarder := Discarder{
			Field: field,
			Value: value,
		}

		if err = fnc(rs, event, p, discarder); err != nil {
			return err
		}
	}

	return nil
}

// Applier describes the set of methods required to apply kernel event passing policies
type Applier interface {
	ApplyFilterPolicy(eventType eval.EventType, tableName string, mode PolicyMode, flags PolicyFlag) error
	ApplyApprovers(eventType eval.EventType, hook *HookPoint, approvers rules.Approvers) error
	GetReport() *Report
}

func (p *Probe) setKProbePolicy(hookPoint *HookPoint, rs *rules.RuleSet, eventType eval.EventType, capabilities Capabilities, applier Applier) error {
	if !p.enableFilters {
		if err := applier.ApplyFilterPolicy(eventType, hookPoint.PolicyTable, PolicyModeNoFilter, math.MaxUint8); err != nil {
			return err
		}
		return nil
	}

	// if approver disabled
	if !p.config.EnableApprovers {
		if err := applier.ApplyFilterPolicy(eventType, hookPoint.PolicyTable, PolicyModeAccept, math.MaxUint8); err != nil {
			return err
		}
		return nil
	}

	approvers, err := rs.GetApprovers(eventType, capabilities.GetFieldCapabilities())
	if err != nil {
		if err := applier.ApplyFilterPolicy(eventType, hookPoint.PolicyTable, PolicyModeAccept, math.MaxUint8); err != nil {
			return err
		}
		return nil
	}

	if err := applier.ApplyApprovers(eventType, hookPoint, approvers); err != nil {
		log.Errorf("Error while adding approvers fallback in-kernel policy to `%s` for `%s`: %s", PolicyModeAccept, eventType, err)
		if err := applier.ApplyFilterPolicy(eventType, hookPoint.PolicyTable, PolicyModeAccept, math.MaxUint8); err != nil {
			return err
		}
		return nil
	}

	if err := applier.ApplyFilterPolicy(eventType, hookPoint.PolicyTable, PolicyModeDeny, capabilities.GetFlags()); err != nil {
		return err
	}

	return nil
}

// ApplyRuleSet applies the loaded set of rules and returns a report
// of the applied approvers for it. If dryRun is set to true,
// the rules won't be applied but the report will still be returned.
func (p *Probe) ApplyRuleSet(rs *rules.RuleSet, dryRun bool) (*Report, error) {
	var applier Applier = NewReporter()
	if !dryRun {
		applier = &KFilterApplier{probe: p, reporter: applier}
	}

	already := make(map[*HookPoint]bool)

	if !p.enableFilters {
		log.Warn("Forcing in-kernel filter policy to `pass`: filtering not enabled")
	}

	for _, hookPoint := range allHookPoints {
		if hookPoint.EventTypes == nil {
			continue
		}

		// first set policies
		for eventType, capabilities := range hookPoint.EventTypes {
			if rs.HasRulesForEventType(eventType) {
				if hookPoint.PolicyTable == "" {
					continue
				}

				if err := p.setKProbePolicy(hookPoint, rs, eventType, capabilities, applier); err != nil {
					return nil, err
				}
			}
		}

		if dryRun {
			continue
		}

		// then register kprobes
		for eventType := range hookPoint.EventTypes {
			if eventType == "*" || rs.HasRulesForEventType(eventType) {
				if _, ok := already[hookPoint]; ok {
					continue
				}

				var active int
				var err error

				log.Infof("Registering Hook Point `%s`", hookPoint.Name)
				for _, kprobe := range hookPoint.KProbes {
					// use hook point name if kprobe name not provided
					if len(kprobe.Name) == 0 {
						kprobe.Name = hookPoint.Name
					}

					if err = p.Module.RegisterKprobe(kprobe); err == nil {
						log.Infof("kProbe `%s` registered", kprobe.Name)
						active++
					} else {
						log.Debugf("failed to register kProbe `%s`", kprobe.Name)
					}
				}
				if len(hookPoint.Tracepoint) > 0 && err == nil {
					if err = p.Module.RegisterTracepoint(hookPoint.Tracepoint); err == nil {
						log.Infof("tracepoint `%s` registered", hookPoint.Tracepoint)
						active++
					} else {
						log.Debugf("failed to register tracepoint `%s`", hookPoint.Tracepoint)
					}
				}

				if err != nil {
					if !hookPoint.Optional {
						return nil, err
					}
				}

				if active > 0 {
					log.Infof("Hook Point `%s` registered with %d active kProbes", hookPoint.Name, active)
				}
				already[hookPoint] = true
			}
		}
	}

	return applier.GetReport(), nil
}

// Snapshot runs the different snapshot functions of the resolvers that
// require to sync with the current state of the system
func (p *Probe) Snapshot() error {
	return p.resolvers.Snapshot(5)
}

// NewProbe instantiates a new runtime security agent probe
func NewProbe(config *config.Config) (*Probe, error) {
	p := &Probe{
		config:           config,
		onDiscardersFncs: make(map[eval.EventType][]onDiscarderFnc),
		enableFilters:    config.EnableKernelFilters,
		tables:           make(map[string]*ebpf.Table),
	}

	p.Probe = &ebpf.Probe{
		Tables:   p.getTableNames(),
		PerfMaps: p.getPerfMaps(),
	}

	resolvers, err := NewResolvers(p.Probe)
	if err != nil {
		return nil, err
	}

	p.resolvers = resolvers

	return p, nil
}

func init() {
	allHookPoints = append(allHookPoints, openHookPoints...)
	allHookPoints = append(allHookPoints, mountHookPoints...)
	allHookPoints = append(allHookPoints, execHookPoints...)
	allHookPoints = append(allHookPoints, UnlinkHookPoints...)
}
