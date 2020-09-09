// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

// +build linux_bpf

package probe

import (
	"fmt"
	"strings"

	"github.com/DataDog/datadog-go/statsd"

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

var (
	allApproversFncs = make(map[eval.EventType]onApproversFnc)
	allDiscarderFncs = make(map[eval.EventType]onDiscarderFnc)
)

// Probe represents the runtime security eBPF probe in charge of
// setting up the required kProbes and decoding events sent from the kernel
type Probe struct {
	*ebpf.Probe
	config           *config.Config
	handler          EventHandler
	resolvers        *Resolvers
	onDiscardersFncs map[eval.EventType][]onDiscarderFnc
	tables           map[string]*ebpf.Table
	eventsStats      EventsStats
	syscallMonitor   *SyscallMonitor
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

		for _, eventType := range hookpoint.EventTypes {
			fnc, exists := allDiscarderFncs[eventType]
			if !exists {
				continue
			}

			fncs := p.onDiscardersFncs[eventType]
			fncs = append(fncs, fnc)
			p.onDiscardersFncs[eventType] = fncs
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
	case FileSetXAttrEventType:
		if _, err := event.SetXAttr.UnmarshalBinary(data[offset:]); err != nil {
			log.Errorf("failed to decode setxattr event: %s (offset %d, len %d)", err, offset, len(data))
			return
		}
	case FileRemoveXAttrEventType:
		if _, err := event.RemoveXAttr.UnmarshalBinary(data[offset:]); err != nil {
			log.Errorf("failed to decode removexattr event: %s (offset %d, len %d)", err, offset, len(data))
			return
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

// Init initialises the probe
func (p *Probe) Init() error {
	if !p.config.EnableKernelFilters {
		log.Warn("Forcing in-kernel filter policy to `pass`: filtering not enabled")
	}

	return nil
}

// ApplyFilterPolicy is called when a passing policy for an event type is applied
func (p *Probe) ApplyFilterPolicy(eventType eval.EventType, tableName string, mode PolicyMode, flags PolicyFlag) error {
	log.Infof("Setting in-kernel filter policy to `%s` for `%s`", mode, eventType)
	table := p.Table(tableName)
	if table == nil {
		return fmt.Errorf("unable to find policy table `%s`", tableName)
	}

	policy := &FilterPolicy{
		Mode:  mode,
		Flags: flags,
	}

	return table.Set(ebpf.ZeroUint32TableItem, policy)
}

// ApplyApprovers applies approvers
func (p *Probe) ApplyApprovers(eventType eval.EventType, approvers rules.Approvers) error {
	fnc, exists := allApproversFncs[eventType]
	if !exists {
		return nil
	}

	err := fnc(p, approvers)
	if err != nil {
		log.Errorf("Error while adding approvers fallback in-kernel policy to `%s` for `%s`: %s", PolicyModeAccept, eventType, err)
	}
	return err
}

// RegisterKProbe register the given kprobe
func (p *Probe) RegisterKProbe(kprobe *ebpf.KProbe) error {
	err := p.Module.RegisterKprobe(kprobe)
	if err == nil {
		log.Infof("kProbe `%s` registered", kprobe.Name)
	} else {
		log.Errorf("failed to register kProbe `%s`", kprobe.Name)
	}

	return err
}

// RegisterTracepoint registers the given tracepoint
func (p *Probe) RegisterTracepoint(tracepoint string) error {
	err := p.Module.RegisterTracepoint(tracepoint)
	if err == nil {
		log.Infof("tracepoint `%s` registered", tracepoint)
	} else {
		log.Errorf("failed to register tracepoint `%s`", tracepoint)
	}
	return err
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
		tables:           make(map[string]*ebpf.Table),
	}

	p.Probe = &ebpf.Probe{
		Tables:   p.getTableNames(),
		PerfMaps: p.getPerfMaps(),
	}

	resolvers, err := NewResolvers(p)
	if err != nil {
		return nil, err
	}

	p.resolvers = resolvers

	return p, nil
}

func init() {
	allApproversFncs["open"] = openOnNewApprovers

	allDiscarderFncs["open"] = openOnNewDiscarder
	allDiscarderFncs["unlink"] = unlinkOnNewDiscarder
}
