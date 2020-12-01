// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

// +build linux_bpf

package probe

import (
	"context"
	"fmt"
	"strings"
	"sync/atomic"
	"time"

	"github.com/DataDog/datadog-go/statsd"
	lib "github.com/DataDog/ebpf"
	"github.com/DataDog/ebpf/manager"
	"github.com/hashicorp/golang-lru/simplelru"
	"github.com/pkg/errors"

	"github.com/DataDog/datadog-agent/pkg/ebpf/bytecode"
	"github.com/DataDog/datadog-agent/pkg/security/config"
	"github.com/DataDog/datadog-agent/pkg/security/ebpf"
	"github.com/DataDog/datadog-agent/pkg/security/ebpf/probes"
	"github.com/DataDog/datadog-agent/pkg/security/rules"
	"github.com/DataDog/datadog-agent/pkg/security/secl/eval"
	"github.com/DataDog/datadog-agent/pkg/util/kernel"
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

// Discarder represents a discarder which is basically the field that we know for sure
// that the value will be always rejected by the rules
type Discarder struct {
	Field eval.Field
}

type onApproverHandler func(probe *Probe, approvers rules.Approvers) (activeApprovers, error)
type onDiscarderHandler func(rs *rules.RuleSet, event *Event, probe *Probe, discarder Discarder) error

var (
	allApproversHandlers = make(map[eval.EventType]onApproverHandler)
	allDiscarderHandlers = make(map[eval.EventType]onDiscarderHandler)
)

// Probe represents the runtime security eBPF probe in charge of
// setting up the required kProbes and decoding events sent from the kernel
type Probe struct {
	manager            *manager.Manager
	managerOptions     manager.Options
	config             *config.Config
	handler            EventHandler
	resolvers          *Resolvers
	discarderHandlers  map[eval.EventType][]onDiscarderHandler
	pidDiscarders      *lib.Map
	inodeDiscarders    *lib.Map
	invalidDiscarders  map[eval.Field]map[interface{}]bool
	regexCache         *simplelru.LRU
	flushingDiscarders int64
	approvers          map[eval.EventType]activeApprovers
	syscallMonitor     *SyscallMonitor
	loadController     *LoadController
	kernelVersion      kernel.Version
	_                  uint32 // padding for goarch=386
	eventsStats        EventsStats
	startTime          time.Time
	event              *Event
	mountEvent         *Event
}

// Map returns a map by its name
func (p *Probe) Map(name string) (*lib.Map, error) {
	if p.manager == nil {
		return nil, fmt.Errorf("failed to get map '%s', manager is null", name)
	}
	m, ok, err := p.manager.GetMap(name)
	if err != nil {
		return nil, err
	} else if !ok {
		return nil, fmt.Errorf("failed to get map '%s'", name)
	}
	return m, nil
}

func (p *Probe) detectKernelVersion() {
	if kernelVersion, err := kernel.HostVersion(); err != nil {
		log.Warn("unable to detect the kernel version")
	} else {
		p.kernelVersion = kernelVersion
	}
}

// Init initializes the probe
func (p *Probe) Init() error {
	p.startTime = time.Now()
	p.detectKernelVersion()

	asset := "runtime-security"
	openSyscall, err := manager.GetSyscallFnName("open")
	if err != nil {
		return err
	}
	if !strings.HasPrefix(openSyscall, "SyS_") && !strings.HasPrefix(openSyscall, "sys_") {
		asset += "-syscall-wrapper"
	}

	bytecodeReader, err := bytecode.GetReader(p.config.BPFDir, asset+".o")
	if err != nil {
		return err
	}

	p.manager = ebpf.NewRuntimeSecurityManager()

	// Set data and lost handlers
	for _, perfMap := range p.manager.PerfMaps {
		switch perfMap.Name {
		case "events":
			perfMap.PerfMapOptions = manager.PerfMapOptions{
				DataHandler: p.handleEvent,
				LostHandler: p.handleLostEvents,
			}
		case "mountpoints_events":
			perfMap.PerfMapOptions = manager.PerfMapOptions{
				DataHandler: p.handleMountEvent,
				LostHandler: p.handleLostEvents,
			}
		}
	}

	if selectors, exists := probes.SelectorsPerEventType["*"]; exists {
		p.managerOptions.ActivatedProbes = append(p.managerOptions.ActivatedProbes, selectors...)
	}

	if err := p.manager.InitWithOptions(bytecodeReader, p.managerOptions); err != nil {
		return errors.Wrap(err, "failed to init manager")
	}

	if p.pidDiscarders, err = p.Map("pid_discarders"); err != nil {
		return err
	}

	if p.inodeDiscarders, err = p.Map("inode_discarders"); err != nil {
		return err
	}

	if err := p.resolvers.Start(); err != nil {
		return err
	}

	if p.config.SyscallMonitor {
		p.syscallMonitor, err = NewSyscallMonitor(p.manager)
		if err != nil {
			return err
		}
	}

	return nil
}

// Start the runtime security probe
func (p *Probe) Start() error {
	if err := p.manager.Start(); err != nil {
		return err
	}
	go p.loadController.Start(context.Background())
	return nil
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

func (p *Probe) handleLostEvents(CPU int, count uint64, perfMap *manager.PerfMap, manager *manager.Manager) {
	log.Tracef("lost %d events\n", count)
	p.eventsStats.CountLost(int64(count))
}

var eventZero Event

func (p *Probe) zeroEvent() *Event {
	*p.event = eventZero
	return p.event
}

func (p *Probe) zeroMountEvent() *Event {
	*p.mountEvent = eventZero
	return p.mountEvent
}

func (p *Probe) unmarshalProcessContainer(data []byte, event *Event) (int, error) {
	read, err := unmarshalBinary(data, &event.Process, &event.Container)
	if err != nil {
		return 0, err
	}

	if entry := p.resolvers.ProcessResolver.Get(event.Process.Pid); entry != nil {
		event.Process.FileEvent = entry.FileEvent
		event.Container = entry.ContainerEvent
	}

	return read, nil
}

func (p *Probe) handleMountEvent(CPU int, data []byte, perfMap *manager.PerfMap, manager *manager.Manager) {
	offset := 0
	event := p.zeroMountEvent()

	read, err := event.UnmarshalBinary(data)
	if err != nil {
		log.Errorf("failed to decode event: %s", err)
		return
	}
	offset += read

	eventType := EventType(event.Type)

	log.Tracef("Decoding mount event %s(%d)", eventType, event.Type)

	read, err = p.unmarshalProcessContainer(data[offset:], event)
	if err != nil {
		log.Errorf("failed to decode event `%s`: %s", err, eventType)
		return
	}
	offset += read

	switch eventType {
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
		p.resolvers.MountResolver.Insert(event.Mount)
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
		log.Errorf("unsupported event type %d on perf map %s", eventType, perfMap.Name)
		return
	}

	p.eventsStats.CountEventType(eventType, 1)
	p.loadController.Count(eventType, event.Process.Pid)
	p.DispatchEvent(event)
}

func (p *Probe) handleEvent(CPU int, data []byte, perfMap *manager.PerfMap, manager *manager.Manager) {
	offset := 0
	event := p.zeroEvent()

	read, err := event.UnmarshalBinary(data)
	if err != nil {
		log.Errorf("failed to decode event: %s", err)
		return
	}
	offset += read

	eventType := EventType(event.Type)

	log.Tracef("Decoding event %s(%d)", eventType, event.Type)

	switch eventType {
	case ExecEventType:
		if _, err := event.Exec.UnmarshalBinary(data[offset:]); err != nil {
			log.Errorf("failed to decode exec event: %s (offset %d, len %d)", err, offset, len(data))
			return
		}

		p.resolvers.ProcessResolver.AddEntry(event.Exec.Pid, event.Exec.ProcessCacheEntry)

		return
	case ExitEventType:
		if _, err := event.Exit.UnmarshalBinary(data[offset:]); err != nil {
			log.Errorf("failed to decode exec event: %s (offset %d, len %d)", err, offset, len(data))
			return
		}

		// as far as we keep only one perf for all the event we can delete the entry right away, there won't be
		// any race
		p.resolvers.ProcessResolver.DelEntry(event.Exit.Pid)

		// no need to dispatch
		return
	case InvalidateDentryEventType:
		if _, err := event.InvalidateDentry.UnmarshalBinary(data[offset:]); err != nil {
			log.Errorf("failed to decode invalidate dentry event: %s (offset %d, len %d)", err, offset, len(data))
			return
		}

		log.Tracef("remove dentry cache entry for inode %d", event.InvalidateDentry.Inode)

		p.resolvers.DentryResolver.DelCacheEntry(event.InvalidateDentry.MountID, event.InvalidateDentry.Inode)

		// If a temporary file is created and deleted in a row a discarder can be added
		// after the in-kernel discarder cleanup and thus a discarder will be pushed for a deleted file.
		// If the inode is reused this can be a problem.
		// Call a user space remove function to ensure the discarder will be removed.
		p.removeDiscarderInode(event.InvalidateDentry.MountID, event.InvalidateDentry.Inode)

		// no need to dispatch
		return
	}

	read, err = p.unmarshalProcessContainer(data[offset:], event)
	if err != nil {
		log.Errorf("failed to decode event `%s`: %s", eventType, err)
		return
	}
	offset += read

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

		log.Tracef("remove dentry cache entry for inode %d", event.Rmdir.Inode)

		// defer it do ensure that it will be done after the dispatch that could re-add it
		defer p.resolvers.DentryResolver.DelCacheEntry(event.Rmdir.MountID, event.Rmdir.Inode)
	case FileUnlinkEventType:
		if _, err := event.Unlink.UnmarshalBinary(data[offset:]); err != nil {
			log.Errorf("failed to decode unlink event: %s (offset %d, len %d)", err, offset, len(data))
			return
		}

		log.Tracef("remove dentry cache entry for inode %d", event.Unlink.Inode)

		// defer it do ensure that it will be done after the dispatch that could re-add it
		defer p.resolvers.DentryResolver.DelCacheEntry(event.Unlink.MountID, event.Unlink.Inode)
	case FileRenameEventType:
		if _, err := event.Rename.UnmarshalBinary(data[offset:]); err != nil {
			log.Errorf("failed to decode rename event: %s (offset %d, len %d)", err, offset, len(data))
			return
		}

		log.Tracef("remove dentry cache entry for inode %d", event.Rename.New.Inode)

		// use the new.inode as the old one is a fake one generated from the probe. See RenameEvent.MarshalJSON
		// defer it do ensure that it will be done after the dispatch that could re-add it
		defer p.resolvers.DentryResolver.DelCacheEntry(event.Rename.New.MountID, event.Rename.New.Inode)
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
		log.Errorf("unsupported event type %d on perf map %s", eventType, perfMap.Name)
		return
	}

	log.Tracef("Dispatching event %+v\n", event)

	p.eventsStats.CountEventType(eventType, 1)
	p.loadController.Count(eventType, event.Process.Pid)
	p.DispatchEvent(event)
}

// OnNewDiscarder is called when a new discarder is found
func (p *Probe) OnNewDiscarder(rs *rules.RuleSet, event *Event, field eval.Field, eventType eval.EventType) error {
	// discarders disabled
	if !p.config.EnableDiscarders {
		return nil
	}

	if atomic.LoadInt64(&p.flushingDiscarders) == 1 {
		return nil
	}

	log.Tracef("New discarder of type %s for field %s", eventType, field)

	if handler, ok := allDiscarderHandlers[eventType]; ok {
		return handler(rs, event, p, Discarder{Field: field})
	}

	return nil
}

// ApplyFilterPolicy is called when a passing policy for an event type is applied
func (p *Probe) ApplyFilterPolicy(eventType eval.EventType, mode PolicyMode, flags PolicyFlag) error {
	log.Infof("Setting in-kernel filter policy to `%s` for `%s`", mode, eventType)
	table, err := p.Map("filter_policy")
	if err != nil {
		return errors.Wrap(err, "unable to find policy table")
	}

	et := parseEvalEventType(eventType)
	if et == UnknownEventType {
		return errors.New("unable to parse the eval event type")
	}

	policy := &FilterPolicy{
		Mode:  mode,
		Flags: flags,
	}

	return table.Put(ebpf.Uint32MapItem(et), policy)
}

// SetApprovers applies approvers and removes the unused ones
func (p *Probe) SetApprovers(eventType eval.EventType, approvers rules.Approvers) error {
	handler, exists := allApproversHandlers[eventType]
	if !exists {
		return nil
	}

	newApprovers, err := handler(p, approvers)
	if err != nil {
		log.Errorf("Error while adding approvers fallback in-kernel policy to `%s` for `%s`: %s", PolicyModeAccept, eventType, err)
	}

	for _, newApprover := range newApprovers {
		log.Tracef("Applying approver %+v", newApprover)
		if err := newApprover.Apply(p); err != nil {
			return err
		}
	}

	if previousApprovers, exist := p.approvers[eventType]; exist {
		previousApprovers.Sub(newApprovers)
		for _, previousApprover := range previousApprovers {
			log.Tracef("Removing previous approver %+v", previousApprover)
			if err := previousApprover.Remove(p); err != nil {
				return err
			}
		}
	}

	p.approvers[eventType] = newApprovers
	return nil
}

// SelectProbes applies the loaded set of rules and returns a report
// of the applied approvers for it.
func (p *Probe) SelectProbes(rs *rules.RuleSet) error {
	var activatedProbes []manager.ProbesSelector

	var selectedIDs []manager.ProbeIdentificationPair
	for eventType, selectors := range probes.SelectorsPerEventType {
		if eventType == "*" || rs.HasRulesForEventType(eventType) {
			activatedProbes = append(activatedProbes, selectors...)

			// Extract unique IDs for logging purposes only
			for _, selector := range selectors {
				for _, id := range selector.GetProbesIdentificationPairList() {
					var exists bool
					for _, selectedID := range selectedIDs {
						if selectedID.Matches(id) {
							exists = true
						}
					}
					if !exists {
						selectedIDs = append(selectedIDs, id)
					}
				}
			}
		}
	}

	// Print the list of unique probe identification IDs that are registered
	for _, id := range selectedIDs {
		log.Tracef("probe %s selected", id)
	}

	enabledEventsMap, err := p.Map("enabled_events")
	if err != nil {
		return err
	}

	enabledEvents := uint64(0)
	for _, eventName := range rs.GetEventTypes() {
		if eventName != "*" {
			eventType := parseEvalEventType(eventName)
			if eventType == UnknownEventType {
				return fmt.Errorf("unknown event type '%s'", eventName)
			}
			enabledEvents |= 1 << (eventType - 1)
		}
	}

	if err := enabledEventsMap.Put(ebpf.ZeroUint32MapItem, enabledEvents); err != nil {
		return errors.Wrap(err, "failed to set enabled events")
	}

	return p.manager.UpdateActivatedProbes(activatedProbes)
}

// FlushDiscarders removes all the discarders
func (p *Probe) FlushDiscarders() error {
	log.Debugf("Freezing discarders")

	flushingMap, err := p.Map("flushing_discarders")
	if err != nil {
		return err
	}

	if err := flushingMap.Put(ebpf.ZeroUint32MapItem, uint32(1)); err != nil {
		return errors.Wrap(err, "failed to set flush_discarders flag")
	}

	unfreezeDiscarders := func() {
		atomic.StoreInt64(&p.flushingDiscarders, 0)

		if err := flushingMap.Put(ebpf.ZeroUint32MapItem, uint32(0)); err != nil {
			log.Errorf("Failed to reset flush_discarders flag: %s", err)
		}

		log.Debugf("Unfreezing discarders")
	}
	defer unfreezeDiscarders()

	// Sleeping a bit to avoid races with executing kprobes and setting discarders
	if !atomic.CompareAndSwapInt64(&p.flushingDiscarders, 0, 1) {
		return errors.New("already flushing discarders")
	}
	time.Sleep(100 * time.Millisecond)

	var discardedInodes []inodeDiscarder
	var inodeParams inodeDiscarderParameters
	var inode inodeDiscarder
	for entries := p.inodeDiscarders.Iterate(); entries.Next(&inode, &inodeParams); {
		discardedInodes = append(discardedInodes, inode)
	}

	var discardedPids []uint32
	var pidParams pidDiscarderParameters
	for pid, entries := uint32(0), p.pidDiscarders.Iterate(); entries.Next(&pid, &pidParams); {
		discardedPids = append(discardedPids, pid)
	}

	discarderCount := len(discardedInodes) + len(discardedPids)
	if discarderCount == 0 {
		log.Debugf("No discarder found")
		return nil
	}

	flushWindow := time.Second * time.Duration(p.config.FlushDiscarderWindow)
	delay := flushWindow / time.Duration(discarderCount)

	flushDiscarders := func() {
		log.Debugf("Flushing discarders")

		for _, inode := range discardedInodes {
			if err := p.inodeDiscarders.Delete(&inode); err != nil {
				log.Tracef("Failed to flush discarder for inode %d: %s", inode, err)
			}

			discarderCount--
			if discarderCount > 0 {
				time.Sleep(delay)
			}
		}

		for _, pid := range discardedPids {
			if err := p.pidDiscarders.Delete(pid); err != nil {
				log.Tracef("Failed to flush discarder for pid %d: %s", pid, err)
			}

			discarderCount--
			if discarderCount > 0 {
				time.Sleep(delay)
			}
		}
	}

	if p.config.FlushDiscarderWindow != 0 {
		go flushDiscarders()
	} else {
		flushDiscarders()
	}

	return nil
}

// Snapshot runs the different snapshot functions of the resolvers that
// require to sync with the current state of the system
func (p *Probe) Snapshot() error {
	return p.resolvers.Snapshot()
}

// Close the probe
func (p *Probe) Close() error {
	return p.manager.Stop(manager.CleanAll)
}

// IsInvalidDiscarder returns whether the given value is a valid discarder for the given field
func (p *Probe) IsInvalidDiscarder(field eval.Field, value interface{}) bool {
	values, exists := p.invalidDiscarders[field]
	if !exists {
		return false
	}

	return values[value]
}

// rearrange invalid discarders for fast lookup
func getInvalidDiscarders() map[eval.Field]map[interface{}]bool {
	invalidDiscarders := make(map[eval.Field]map[interface{}]bool)

	if InvalidDiscarders != nil {
		for field, values := range InvalidDiscarders {
			ivalues := invalidDiscarders[field]
			if ivalues == nil {
				ivalues = make(map[interface{}]bool)
				invalidDiscarders[field] = ivalues
			}
			for _, value := range values {
				ivalues[value] = true
			}
		}
	}

	return invalidDiscarders
}

// NewProbe instantiates a new runtime security agent probe
func NewProbe(config *config.Config, client *statsd.Client) (*Probe, error) {
	regexCache, err := simplelru.NewLRU(64, nil)
	if err != nil {
		return nil, err
	}

	p := &Probe{
		config:            config,
		invalidDiscarders: getInvalidDiscarders(),
		approvers:         make(map[eval.EventType]activeApprovers),
		managerOptions:    ebpf.NewDefaultOptions(),
		regexCache:        regexCache,
	}

	if !p.config.EnableKernelFilters {
		log.Warn("Forcing in-kernel filter policy to `pass`: filtering not enabled")
	}

	if p.config.SyscallMonitor {
		// Add syscall monitor probes
		p.managerOptions.ActivatedProbes = append(p.managerOptions.ActivatedProbes, probes.SyscallMonitorSelectors...)
	}

	resolvers, err := NewResolvers(p)
	if err != nil {
		return nil, err
	}

	p.resolvers = resolvers
	p.event = NewEvent(p.resolvers)
	p.mountEvent = NewEvent(p.resolvers)
	p.loadController, err = NewLoadController(p, client)
	if err != nil {
		return nil, err
	}

	eventZero.resolvers = p.resolvers

	return p, nil
}

func processDiscarderWrapper(eventType EventType, fnc onDiscarderHandler) onDiscarderHandler {
	return func(rs *rules.RuleSet, event *Event, probe *Probe, discarder Discarder) error {
		if discarder.Field == "process.filename" {
			log.Tracef("Apply process.filename discarder for event `%s`, inode: %d", eventType, event.Process.Inode)

			// discard by PID for long running process
			if err := probe.discardPID(eventType, event.Process.Pid); err != nil {
				return err
			}

			return probe.discardInode(eventType, event.Process.MountID, event.Process.Inode)
		}

		if fnc != nil {
			return fnc(rs, event, probe, discarder)
		}

		return nil
	}
}

// function used to retrieve discarder information, *.filename, mountID, inode, file deleted
type inodeEventGetter = func(event *Event) (eval.Field, uint32, uint64, uint32, bool)

func filenameDiscarderWrapper(eventType EventType, handler onDiscarderHandler, getter inodeEventGetter) onDiscarderHandler {
	return func(rs *rules.RuleSet, event *Event, probe *Probe, discarder Discarder) error {
		field, mountID, inode, pathID, isDeleted := getter(event)

		if discarder.Field == field {
			value, err := event.GetFieldValue(field)
			if err != nil {
				return err
			}
			filename := value.(string)

			if filename == "" {
				return nil
			}

			if probe.IsInvalidDiscarder(field, filename) {
				return nil
			}

			isDiscarded, _, parentInode, err := probe.discardParentInode(rs, eventType, field, filename, mountID, inode, pathID)
			if !isDiscarded && !isDeleted {
				if _, ok := err.(*ErrInvalidKeyPath); !ok {
					log.Tracef("Apply `%s.filename` inode discarder for event `%s`, inode: %d", eventType, eventType, inode)

					// not able to discard the parent then only discard the filename
					err = probe.discardInode(eventType, mountID, inode)
				}
			} else {
				log.Tracef("Apply `%s.filename` parent inode discarder for event `%s` with value `%s`", eventType, eventType, filename)
			}

			if err != nil {
				err = errors.Wrapf(err, "unable to set inode discarders for `%s` for event `%s`, inode: %d", filename, eventType, parentInode)
			}

			return err
		}

		if handler != nil {
			return handler(rs, event, probe, discarder)
		}

		return nil
	}
}

func init() {
	// approvers
	allApproversHandlers["open"] = openOnNewApprovers

	// discarders
	SupportedDiscarders["process.filename"] = true

	allDiscarderHandlers["open"] = processDiscarderWrapper(FileOpenEventType,
		filenameDiscarderWrapper(FileOpenEventType, nil,
			func(event *Event) (eval.Field, uint32, uint64, uint32, bool) {
				return "open.filename", event.Open.MountID, event.Open.Inode, event.Open.PathID, false
			}))
	SupportedDiscarders["open.filename"] = true

	allDiscarderHandlers["mkdir"] = processDiscarderWrapper(FileMkdirEventType,
		filenameDiscarderWrapper(FileMkdirEventType, nil,
			func(event *Event) (eval.Field, uint32, uint64, uint32, bool) {
				return "mkdir.filename", event.Mkdir.MountID, event.Mkdir.Inode, event.Mkdir.PathID, false
			}))
	SupportedDiscarders["mkdir.filename"] = true

	allDiscarderHandlers["link"] = processDiscarderWrapper(FileLinkEventType, nil)

	allDiscarderHandlers["rename"] = processDiscarderWrapper(FileRenameEventType, nil)

	allDiscarderHandlers["unlink"] = processDiscarderWrapper(FileUnlinkEventType,
		filenameDiscarderWrapper(FileUnlinkEventType, nil,
			func(event *Event) (eval.Field, uint32, uint64, uint32, bool) {
				return "unlink.filename", event.Unlink.MountID, event.Unlink.Inode, event.Unlink.PathID, true
			}))
	SupportedDiscarders["unlink.filename"] = true

	allDiscarderHandlers["rmdir"] = processDiscarderWrapper(FileRmdirEventType,
		filenameDiscarderWrapper(FileRmdirEventType, nil,
			func(event *Event) (eval.Field, uint32, uint64, uint32, bool) {
				return "rmdir.filename", event.Rmdir.MountID, event.Rmdir.Inode, event.Rmdir.PathID, false
			}))
	SupportedDiscarders["rmdir.filename"] = true

	allDiscarderHandlers["chmod"] = processDiscarderWrapper(FileChmodEventType,
		filenameDiscarderWrapper(FileChmodEventType, nil,
			func(event *Event) (eval.Field, uint32, uint64, uint32, bool) {
				return "chmod.filename", event.Chmod.MountID, event.Chmod.Inode, event.Chmod.PathID, false
			}))
	SupportedDiscarders["chmod.filename"] = true

	allDiscarderHandlers["chown"] = processDiscarderWrapper(FileChownEventType,
		filenameDiscarderWrapper(FileChownEventType, nil,
			func(event *Event) (eval.Field, uint32, uint64, uint32, bool) {
				return "chown.filename", event.Chown.MountID, event.Chown.Inode, event.Chown.PathID, false
			}))
	SupportedDiscarders["chown.filename"] = true

	allDiscarderHandlers["utimes"] = processDiscarderWrapper(FileUtimeEventType,
		filenameDiscarderWrapper(FileUtimeEventType, nil,
			func(event *Event) (eval.Field, uint32, uint64, uint32, bool) {
				return "utimes.filename", event.Utimes.MountID, event.Utimes.Inode, event.Utimes.PathID, false
			}))
	SupportedDiscarders["utimes.filename"] = true

	allDiscarderHandlers["setxattr"] = processDiscarderWrapper(FileSetXAttrEventType,
		filenameDiscarderWrapper(FileSetXAttrEventType, nil,
			func(event *Event) (eval.Field, uint32, uint64, uint32, bool) {
				return "setxattr.filename", event.SetXAttr.MountID, event.SetXAttr.Inode, event.SetXAttr.PathID, false
			}))
	SupportedDiscarders["setxattr.filename"] = true

	allDiscarderHandlers["removexattr"] = processDiscarderWrapper(FileRemoveXAttrEventType,
		filenameDiscarderWrapper(FileRemoveXAttrEventType, nil,
			func(event *Event) (eval.Field, uint32, uint64, uint32, bool) {
				return "removexattr.filename", event.RemoveXAttr.MountID, event.RemoveXAttr.Inode, event.RemoveXAttr.PathID, false
			}))
	SupportedDiscarders["removexattr.filename"] = true
}
