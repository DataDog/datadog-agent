// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// +build linux

package probe

import (
	"context"
	"fmt"
	"math"
	"math/rand"
	"os"
	"runtime"
	"strings"
	"sync/atomic"
	"time"

	"github.com/cihub/seelog"

	"github.com/DataDog/datadog-go/statsd"
	lib "github.com/DataDog/ebpf"
	"github.com/DataDog/ebpf/manager"
	"github.com/hashicorp/golang-lru/simplelru"
	"github.com/pkg/errors"

	"github.com/DataDog/datadog-agent/pkg/ebpf/bytecode"
	"github.com/DataDog/datadog-agent/pkg/security/config"
	"github.com/DataDog/datadog-agent/pkg/security/ebpf"
	"github.com/DataDog/datadog-agent/pkg/security/ebpf/probes"
	"github.com/DataDog/datadog-agent/pkg/security/model"
	"github.com/DataDog/datadog-agent/pkg/security/rules"
	"github.com/DataDog/datadog-agent/pkg/security/secl/eval"
	"github.com/DataDog/datadog-agent/pkg/util/kernel"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

const (
	// discarderRevisionSize array size used to store discarder revisions
	discarderRevisionSize = 4096
)

// EventHandler represents an handler for the events sent by the probe
type EventHandler interface {
	HandleEvent(event *Event)
	HandleCustomEvent(rule *rules.Rule, event *CustomEvent)
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
	// Constants and configuration
	manager        *manager.Manager
	managerOptions manager.Options
	config         *config.Config
	statsdClient   *statsd.Client
	startTime      time.Time
	kernelVersion  kernel.Version
	_              uint32 // padding for goarch=386
	ctx            context.Context
	cancelFnc      context.CancelFunc

	// Events section
	handler   EventHandler
	monitor   *Monitor
	resolvers *Resolvers
	event     *Event
	perfMap   *manager.PerfMap
	reOrderer *ReOrderer

	// Approvers / discarders section
	discarderRevisions *lib.Map
	inodeDiscarders    *lib.Map
	pidDiscarders      *lib.Map
	revisionCache      [discarderRevisionSize]uint32
	invalidDiscarders  map[eval.Field]map[interface{}]bool
	regexCache         *simplelru.LRU
	flushingDiscarders int64
	approvers          map[eval.EventType]activeApprovers
}

// GetResolvers returns the resolvers of Probe
func (p *Probe) GetResolvers() *Resolvers {
	return p.resolvers
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
func (p *Probe) Init(client *statsd.Client) error {
	p.startTime = time.Now()

	var err error
	var bytecodeReader bytecode.AssetReader

	useSyscallWrapper := false
	openSyscall, err := manager.GetSyscallFnName("open")
	if err != nil {
		return err
	}
	if !strings.HasPrefix(openSyscall, "SyS_") && !strings.HasPrefix(openSyscall, "sys_") {
		useSyscallWrapper = true
	}

	if p.config.EnableRuntimeCompiler {
		bytecodeReader, err = getRuntimeCompiledProbe(p.config, useSyscallWrapper)
		if err != nil {
			log.Warnf("error compiling runtime-security probe, falling back to pre-compiled: %s", err)
		} else {
			defer bytecodeReader.Close()
		}
	}

	// fallback to pre-compiled version
	if bytecodeReader == nil {
		asset := "runtime-security"
		if useSyscallWrapper {
			asset += "-syscall-wrapper"
		}

		bytecodeReader, err = bytecode.GetReader(p.config.BPFDir, asset+".o")
		if err != nil {
			return err
		}
		defer bytecodeReader.Close()
	}

	p.manager = ebpf.NewRuntimeSecurityManager()

	var ok bool
	if p.perfMap, ok = p.manager.GetPerfMap("events"); !ok {
		return errors.New("couldn't find events perf map")
	}

	// Set data and lost handlers
	p.perfMap.PerfMapOptions = manager.PerfMapOptions{
		DataHandler: p.reOrderer.HandleEvent,
		LostHandler: p.handleLostEvents,
	}

	if os.Getenv("RUNTIME_SECURITY_TESTSUITE") != "true" {
		p.managerOptions.ConstantEditors = append(p.managerOptions.ConstantEditors, manager.ConstantEditor{
			Name:  "system_probe_pid",
			Value: uint64(os.Getpid()),
		})
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

	if p.discarderRevisions, err = p.Map("discarder_revisions"); err != nil {
		return err
	}

	if err := p.resolvers.Start(p.ctx); err != nil {
		return err
	}

	p.monitor, err = NewMonitor(p, client)
	if err != nil {
		return err
	}

	return nil
}

// Start the runtime security probe
func (p *Probe) Start() error {
	go p.reOrderer.Start(p.ctx)

	if err := p.manager.Start(); err != nil {
		return err
	}

	if err := p.monitor.Start(p.ctx); err != nil {
		return err
	}
	return nil
}

// SetEventHandler set the probe event handler
func (p *Probe) SetEventHandler(handler EventHandler) {
	p.handler = handler
}

// DispatchEvent sends an event to the probe event handler
func (p *Probe) DispatchEvent(event *Event, size uint64, CPU int, perfMap *manager.PerfMap) {
	if logLevel, err := log.GetLogLevel(); err != nil || logLevel == seelog.TraceLvl {
		prettyEvent := event.String()
		log.Tracef("Dispatching event %s\n", prettyEvent)
	}

	if p.handler != nil {
		p.handler.HandleEvent(event)
	}

	// Process after evaluation because some monitors need the DentryResolver to have been called first.
	p.monitor.ProcessEvent(event, size, CPU, perfMap)
}

// DispatchCustomEvent sends a custom event to the probe event handler
func (p *Probe) DispatchCustomEvent(rule *rules.Rule, event *CustomEvent) {
	if logLevel, err := log.GetLogLevel(); err != nil || logLevel == seelog.TraceLvl {
		prettyEvent := event.String()
		log.Tracef("Dispatching custom event %s\n", prettyEvent)
	}

	if p.handler != nil && p.config.AgentMonitoringEvents {
		p.handler.HandleCustomEvent(rule, event)
	}
}

// SendStats sends statistics about the probe to Datadog
func (p *Probe) SendStats() error {
	return p.monitor.SendStats()
}

// GetMonitor returns the monitor of the probe
func (p *Probe) GetMonitor() *Monitor {
	return p.monitor
}

func (p *Probe) getDiscarderRevision(mountID uint32) uint32 {
	key := mountID % discarderRevisionSize
	return p.revisionCache[key]
}

func (p *Probe) setDiscarderRevision(mountID uint32, revision uint32) {
	key := mountID % discarderRevisionSize
	p.revisionCache[key] = revision
}

func (p *Probe) initDiscarderRevision(mountEvent *model.MountEvent) {
	var revision uint32

	if mountEvent.IsOverlayFS() {
		revision = uint32(rand.Intn(math.MaxUint16) + 1)
	}

	key := mountEvent.MountID % discarderRevisionSize
	p.revisionCache[key] = revision

	if err := p.discarderRevisions.Put(ebpf.Uint32MapItem(key), ebpf.Uint32MapItem(revision)); err != nil {
		log.Errorf("unable to initialize discarder revisions: %s", err)
	}
}

func (p *Probe) handleLostEvents(CPU int, count uint64, perfMap *manager.PerfMap, manager *manager.Manager) {
	log.Tracef("lost %d events", count)
	p.monitor.perfBufferMonitor.CountLostEvent(count, perfMap, CPU)
}

var eventZero Event

func (p *Probe) zeroEvent() *Event {
	*p.event = eventZero
	return p.event
}

func (p *Probe) unmarshalProcessContainer(data []byte, event *Event) (int, error) {
	read, err := model.UnmarshalBinary(data, &event.Process, &event.Container)
	if err != nil {
		return 0, err
	}

	return read, nil
}

func (p *Probe) invalidateDentry(mountID uint32, inode uint64, revision uint32) {
	if p.resolvers.MountResolver.IsOverlayFS(mountID) {
		log.Tracef("remove all dentry entries for mount id %d", mountID)
		p.resolvers.DentryResolver.DelCacheEntries(mountID)

		p.setDiscarderRevision(mountID, revision)
	} else {
		log.Tracef("remove dentry cache entry for inode %d", inode)
		p.resolvers.DentryResolver.DelCacheEntry(mountID, inode)

		// If a temporary file is created and deleted in a row a discarder can be added
		// after the in-kernel discarder cleanup and thus a discarder will be pushed for a deleted file.
		// If the inode is reused this can be a problem.
		// Call a user space remove function to ensure the discarder will be removed.
		p.removeDiscarderInode(mountID, inode)
	}
}

func (p *Probe) handleEvent(CPU uint64, data []byte) {
	offset := 0
	event := p.zeroEvent()
	dataLen := uint64(len(data))

	read, err := event.UnmarshalBinary(data)
	if err != nil {
		log.Errorf("failed to decode event: %s", err)
		return
	}
	offset += read

	eventType := event.GetEventType()
	p.monitor.perfBufferMonitor.CountEvent(eventType, event.TimestampRaw, 1, dataLen, p.perfMap, int(CPU))

	log.Debugf("Decoding event %s(%d)", eventType, event.Type)

	if eventType == model.InvalidateDentryEventType {
		if _, err := event.InvalidateDentry.UnmarshalBinary(data[offset:]); err != nil {
			log.Errorf("failed to decode invalidate dentry event: %s (offset %d, len %d)", err, offset, dataLen)
			return
		}

		p.invalidateDentry(event.InvalidateDentry.MountID, event.InvalidateDentry.Inode, event.InvalidateDentry.DiscarderRevision)

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
	case model.FileMountEventType:
		if _, err := event.Mount.UnmarshalBinary(data[offset:]); err != nil {
			log.Errorf("failed to decode mount event: %s (offset %d, len %d)", err, offset, dataLen)
			return
		}

		// Resolve mount point
		event.ResolveMountPoint(&event.Mount)
		// Resolve root
		event.ResolveMountRoot(&event.Mount)
		// Insert new mount point in cache
		p.resolvers.MountResolver.Insert(event.Mount)
	case model.FileUmountEventType:
		if _, err := event.Umount.UnmarshalBinary(data[offset:]); err != nil {
			log.Errorf("failed to decode umount event: %s (offset %d, len %d)", err, offset, dataLen)
			return
		}
		// Remove all dentry entries belonging to the mountID
		p.resolvers.DentryResolver.DelCacheEntries(event.Umount.MountID)

		if p.resolvers.MountResolver.IsOverlayFS(event.Umount.MountID) {
			p.setDiscarderRevision(event.Umount.MountID, event.Umount.DiscarderRevision)
		}

		// Delete new mount point from cache
		if err := p.resolvers.MountResolver.Delete(event.Umount.MountID); err != nil {
			log.Errorf("failed to delete mount point %d from cache: %s", event.Umount.MountID, err)
		}
	case model.FileOpenEventType:
		if _, err := event.Open.UnmarshalBinary(data[offset:]); err != nil {
			log.Errorf("failed to decode open event: %s (offset %d, len %d)", err, offset, dataLen)
			return
		}
	case model.FileMkdirEventType:
		if _, err := event.Mkdir.UnmarshalBinary(data[offset:]); err != nil {
			log.Errorf("failed to decode mkdir event: %s (offset %d, len %d)", err, offset, dataLen)
			return
		}
	case model.FileRmdirEventType:
		if _, err := event.Rmdir.UnmarshalBinary(data[offset:]); err != nil {
			log.Errorf("failed to decode rmdir event: %s (offset %d, len %d)", err, offset, dataLen)
			return
		}

		// defer it do ensure that it will be done after the dispatch that could re-add it
		defer p.invalidateDentry(event.Rmdir.MountID, event.Rmdir.Inode, event.Rmdir.DiscarderRevision)
	case model.FileUnlinkEventType:
		if _, err := event.Unlink.UnmarshalBinary(data[offset:]); err != nil {
			log.Errorf("failed to decode unlink event: %s (offset %d, len %d)", err, offset, dataLen)
			return
		}

		// defer it do ensure that it will be done after the dispatch that could re-add it
		defer p.invalidateDentry(event.Unlink.MountID, event.Unlink.Inode, event.Unlink.DiscarderRevision)
	case model.FileRenameEventType:
		if _, err := event.Rename.UnmarshalBinary(data[offset:]); err != nil {
			log.Errorf("failed to decode rename event: %s (offset %d, len %d)", err, offset, dataLen)
			return
		}

		defer p.invalidateDentry(event.Rename.New.MountID, event.Rename.New.Inode, event.Rename.DiscarderRevision)
	case model.FileChmodEventType:
		if _, err := event.Chmod.UnmarshalBinary(data[offset:]); err != nil {
			log.Errorf("failed to decode chmod event: %s (offset %d, len %d)", err, offset, dataLen)
			return
		}
	case model.FileChownEventType:
		if _, err := event.Chown.UnmarshalBinary(data[offset:]); err != nil {
			log.Errorf("failed to decode chown event: %s (offset %d, len %d)", err, offset, dataLen)
			return
		}
	case model.FileUtimeEventType:
		if _, err := event.Utimes.UnmarshalBinary(data[offset:]); err != nil {
			log.Errorf("failed to decode utime event: %s (offset %d, len %d)", err, offset, dataLen)
			return
		}
	case model.FileLinkEventType:
		if _, err := event.Link.UnmarshalBinary(data[offset:]); err != nil {
			log.Errorf("failed to decode link event: %s (offset %d, len %d)", err, offset, dataLen)
			return
		}
	case model.FileSetXAttrEventType:
		if _, err := event.SetXAttr.UnmarshalBinary(data[offset:]); err != nil {
			log.Errorf("failed to decode setxattr event: %s (offset %d, len %d)", err, offset, dataLen)
			return
		}
	case model.FileRemoveXAttrEventType:
		if _, err := event.RemoveXAttr.UnmarshalBinary(data[offset:]); err != nil {
			log.Errorf("failed to decode removexattr event: %s (offset %d, len %d)", err, offset, dataLen)
			return
		}
	case model.ForkEventType:
		if _, err := event.UnmarshalExecEvent(data[offset:]); err != nil {
			log.Errorf("failed to decode fork event: %s (offset %d, len %d)", err, offset, dataLen)
			return
		}
		event.updateProcessCachePointer(p.resolvers.ProcessResolver.AddForkEntry(event.Process.Pid, event.processCacheEntry))
	case model.ExecEventType:
		if _, err := event.UnmarshalExecEvent(data[offset:]); err != nil {
			log.Errorf("failed to decode exec event: %s (offset %d, len %d)", err, offset, len(data))
			return
		}
		event.updateProcessCachePointer(p.resolvers.ProcessResolver.AddExecEntry(event.Process.Pid, event.processCacheEntry))
	case model.ExitEventType:
		defer p.resolvers.ProcessResolver.DeleteEntry(event.Process.Pid, event.ResolveEventTimestamp())
	default:
		log.Errorf("unsupported event type %d", eventType)
		return
	}

	// resolve event context
	if eventType != model.ExitEventType {
		event.ResolveProcessCacheEntry()
	}

	p.DispatchEvent(event, dataLen, int(CPU), p.perfMap)
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

	et := model.ParseEvalEventType(eventType)
	if et == model.UnknownEventType {
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

	for eventType, selectors := range probes.SelectorsPerEventType {
		if eventType == "*" || rs.HasRulesForEventType(eventType) {
			activatedProbes = append(activatedProbes, selectors...)
		}
	}

	// Add syscall monitor probes
	if p.config.SyscallMonitor {
		activatedProbes = append(activatedProbes, probes.SyscallMonitorSelectors...)
	}

	// Print the list of unique probe identification IDs that are registered
	var selectedIDs []manager.ProbeIdentificationPair
	for _, selector := range activatedProbes {
		for _, id := range selector.GetProbesIdentificationPairList() {
			var exists bool
			for _, selectedID := range selectedIDs {
				if selectedID.Matches(id) {
					exists = true
				}
			}
			if !exists {
				selectedIDs = append(selectedIDs, id)
				log.Tracef("probe %s selected", id)
			}
		}
	}

	enabledEventsMap, err := p.Map("enabled_events")
	if err != nil {
		return err
	}

	enabledEvents := uint64(0)
	for _, eventName := range rs.GetEventTypes() {
		if eventName != "*" {
			eventType := model.ParseEvalEventType(eventName)
			if eventType == model.UnknownEventType {
				return fmt.Errorf("unknown event type '%s'", eventName)
			}
			enabledEvents |= 1 << (eventType - 1)
		}
	}

	// We might end up missing events during the snapshot. Ultimately we might want to stop the rules evaluation but
	// not the perf map entirely. For now this will do though :)
	if err := p.perfMap.Pause(); err != nil {
		return err
	}
	defer func() {
		if err := p.perfMap.Resume(); err != nil {
			log.Errorf("failed to resume perf map: %s", err)
		}
	}()

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
	p.cancelFnc()

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

// GetDebugStats returns the debug stats
func (p *Probe) GetDebugStats() map[string]interface{} {
	debug := map[string]interface{}{
		"start_time": p.startTime.String(),
	}
	// TODO(Will): add manager state
	return debug
}

// NewProbe instantiates a new runtime security agent probe
func NewProbe(config *config.Config, client *statsd.Client) (*Probe, error) {
	regexCache, err := simplelru.NewLRU(64, nil)
	if err != nil {
		return nil, err
	}

	ctx, cancel := context.WithCancel(context.Background())

	p := &Probe{
		config:            config,
		invalidDiscarders: getInvalidDiscarders(),
		approvers:         make(map[eval.EventType]activeApprovers),
		managerOptions:    ebpf.NewDefaultOptions(),
		regexCache:        regexCache,
		ctx:               ctx,
		cancelFnc:         cancel,
		statsdClient:      client,
	}
	p.detectKernelVersion()

	if !p.config.EnableKernelFilters {
		log.Warn("Forcing in-kernel filter policy to `pass`: filtering not enabled")
	}

	if p.config.SyscallMonitor {
		// Add syscall monitor probes
		p.managerOptions.ActivatedProbes = append(p.managerOptions.ActivatedProbes, probes.SyscallMonitorSelectors...)
	}

	// Add global constant editors
	p.managerOptions.ConstantEditors = append(p.managerOptions.ConstantEditors,
		manager.ConstantEditor{
			Name:  "do_fork_input",
			Value: getDoForkInput(p),
		},
		manager.ConstantEditor{
			Name:  "mount_id_offset",
			Value: getMountIDOffset(p),
		},
		manager.ConstantEditor{
			Name:  "sizeof_inode",
			Value: getSizeOfStructInode(p),
		},
		manager.ConstantEditor{
			Name:  "sb_magic_offset",
			Value: getSuperBlockMagicOffset(p),
		},
	)

	resolvers, err := NewResolvers(p, client)
	if err != nil {
		return nil, err
	}

	p.resolvers = resolvers
	p.event = NewEvent(p.resolvers)

	windowSize := uint64(15 * runtime.NumCPU())
	if windowSize < 60 {
		windowSize = 60
	}

	p.reOrderer = NewReOrderer(p.handleEvent,
		ExtractEventInfo,
		ReOrdererOpts{
			QueueSize:  100000,
			WindowSize: windowSize,
			Delay:      100 * time.Millisecond,
			Rate:       100 * time.Millisecond,
		})

	eventZero.resolvers = p.resolvers
	return p, nil
}

func processDiscarderWrapper(eventType model.EventType, fnc onDiscarderHandler) onDiscarderHandler {
	return func(rs *rules.RuleSet, event *Event, probe *Probe, discarder Discarder) error {
		if discarder.Field == "process.filename" {
			log.Tracef("Apply process.filename discarder for event `%s`, inode: %d", eventType, event.Process.Inode)

			// discard by PID for long running process
			if err := probe.discardPID(eventType, event.Process.Pid); err != nil {
				return err
			}

			return probe.discardInode(eventType, event.Process.MountID, event.Process.Inode, true)
		}

		if fnc != nil {
			return fnc(rs, event, probe, discarder)
		}

		return nil
	}
}

// function used to retrieve discarder information, *.filename, mountID, inode, file deleted
type inodeEventGetter = func(event *Event) (eval.Field, uint32, uint64, uint32, bool)

func filenameDiscarderWrapper(eventType model.EventType, handler onDiscarderHandler, getter inodeEventGetter) onDiscarderHandler {
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
					err = probe.discardInode(eventType, mountID, inode, true)
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

	allDiscarderHandlers["open"] = processDiscarderWrapper(model.FileOpenEventType,
		filenameDiscarderWrapper(model.FileOpenEventType, nil,
			func(event *Event) (eval.Field, uint32, uint64, uint32, bool) {
				return "open.filename", event.Open.MountID, event.Open.Inode, event.Open.PathID, false
			}))
	SupportedDiscarders["open.filename"] = true

	allDiscarderHandlers["mkdir"] = processDiscarderWrapper(model.FileMkdirEventType,
		filenameDiscarderWrapper(model.FileMkdirEventType, nil,
			func(event *Event) (eval.Field, uint32, uint64, uint32, bool) {
				return "mkdir.filename", event.Mkdir.MountID, event.Mkdir.Inode, event.Mkdir.PathID, false
			}))
	SupportedDiscarders["mkdir.filename"] = true

	allDiscarderHandlers["link"] = processDiscarderWrapper(model.FileLinkEventType, nil)

	allDiscarderHandlers["rename"] = processDiscarderWrapper(model.FileRenameEventType, nil)

	allDiscarderHandlers["unlink"] = processDiscarderWrapper(model.FileUnlinkEventType,
		filenameDiscarderWrapper(model.FileUnlinkEventType, nil,
			func(event *Event) (eval.Field, uint32, uint64, uint32, bool) {
				return "unlink.filename", event.Unlink.MountID, event.Unlink.Inode, event.Unlink.PathID, true
			}))
	SupportedDiscarders["unlink.filename"] = true

	allDiscarderHandlers["rmdir"] = processDiscarderWrapper(model.FileRmdirEventType,
		filenameDiscarderWrapper(model.FileRmdirEventType, nil,
			func(event *Event) (eval.Field, uint32, uint64, uint32, bool) {
				return "rmdir.filename", event.Rmdir.MountID, event.Rmdir.Inode, event.Rmdir.PathID, false
			}))
	SupportedDiscarders["rmdir.filename"] = true

	allDiscarderHandlers["chmod"] = processDiscarderWrapper(model.FileChmodEventType,
		filenameDiscarderWrapper(model.FileChmodEventType, nil,
			func(event *Event) (eval.Field, uint32, uint64, uint32, bool) {
				return "chmod.filename", event.Chmod.MountID, event.Chmod.Inode, event.Chmod.PathID, false
			}))
	SupportedDiscarders["chmod.filename"] = true

	allDiscarderHandlers["chown"] = processDiscarderWrapper(model.FileChownEventType,
		filenameDiscarderWrapper(model.FileChownEventType, nil,
			func(event *Event) (eval.Field, uint32, uint64, uint32, bool) {
				return "chown.filename", event.Chown.MountID, event.Chown.Inode, event.Chown.PathID, false
			}))
	SupportedDiscarders["chown.filename"] = true

	allDiscarderHandlers["utimes"] = processDiscarderWrapper(model.FileUtimeEventType,
		filenameDiscarderWrapper(model.FileUtimeEventType, nil,
			func(event *Event) (eval.Field, uint32, uint64, uint32, bool) {
				return "utimes.filename", event.Utimes.MountID, event.Utimes.Inode, event.Utimes.PathID, false
			}))
	SupportedDiscarders["utimes.filename"] = true

	allDiscarderHandlers["setxattr"] = processDiscarderWrapper(model.FileSetXAttrEventType,
		filenameDiscarderWrapper(model.FileSetXAttrEventType, nil,
			func(event *Event) (eval.Field, uint32, uint64, uint32, bool) {
				return "setxattr.filename", event.SetXAttr.MountID, event.SetXAttr.Inode, event.SetXAttr.PathID, false
			}))
	SupportedDiscarders["setxattr.filename"] = true

	allDiscarderHandlers["removexattr"] = processDiscarderWrapper(model.FileRemoveXAttrEventType,
		filenameDiscarderWrapper(model.FileRemoveXAttrEventType, nil,
			func(event *Event) (eval.Field, uint32, uint64, uint32, bool) {
				return "removexattr.filename", event.RemoveXAttr.MountID, event.RemoveXAttr.Inode, event.RemoveXAttr.PathID, false
			}))
	SupportedDiscarders["removexattr.filename"] = true
}
