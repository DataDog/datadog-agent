// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// +build linux

package probe

import (
	"context"
	"fmt"
	"os"
	"strings"
	"sync/atomic"
	"time"
	"unsafe"

	"github.com/DataDog/datadog-go/statsd"
	lib "github.com/DataDog/ebpf"
	"github.com/DataDog/ebpf/manager"
	"github.com/cihub/seelog"
	"github.com/pkg/errors"

	"github.com/DataDog/datadog-agent/pkg/ebpf/bytecode"
	pconfig "github.com/DataDog/datadog-agent/pkg/process/config"
	"github.com/DataDog/datadog-agent/pkg/security/config"
	"github.com/DataDog/datadog-agent/pkg/security/ebpf"
	"github.com/DataDog/datadog-agent/pkg/security/ebpf/kernel"
	"github.com/DataDog/datadog-agent/pkg/security/ebpf/probes"
	seclog "github.com/DataDog/datadog-agent/pkg/security/log"
	"github.com/DataDog/datadog-agent/pkg/security/model"
	"github.com/DataDog/datadog-agent/pkg/security/rules"
	"github.com/DataDog/datadog-agent/pkg/security/secl/eval"
	"github.com/DataDog/datadog-agent/pkg/security/utils"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// EventHandler represents an handler for the events sent by the probe
type EventHandler interface {
	HandleEvent(event *Event)
	HandleCustomEvent(rule *rules.Rule, event *CustomEvent)
}

// Probe represents the runtime security eBPF probe in charge of
// setting up the required kProbes and decoding events sent from the kernel
type Probe struct {
	// Constants and configuration
	manager        *manager.Manager
	managerOptions manager.Options
	config         *config.Config
	statsdClient   *statsd.Client
	startTime      time.Time
	kernelVersion  *kernel.Version
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
	scrubber  *pconfig.DataScrubber

	// Approvers / discarders section
	erpc               *ERPC
	pidDiscarders      *pidDiscarders
	inodeDiscarders    *inodeDiscarders
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

func (p *Probe) detectKernelVersion() error {
	kernelVersion, err := kernel.NewKernelVersion()
	if err != nil {
		return errors.Wrap(err, "unable to detect the kernel version")
	}
	p.kernelVersion = kernelVersion
	return nil
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

	var ok bool
	if p.perfMap, ok = p.manager.GetPerfMap("events"); !ok {
		return errors.New("couldn't find events perf map")
	}

	p.perfMap.PerfMapOptions = manager.PerfMapOptions{
		DataHandler: p.reOrderer.HandleEvent,
		LostHandler: p.handleLostEvents,
	}

	if os.Getenv("RUNTIME_SECURITY_TESTSUITE") != "true" {
		p.managerOptions.ConstantEditors = append(p.managerOptions.ConstantEditors, manager.ConstantEditor{
			Name:  "system_probe_pid",
			Value: uint64(utils.Getpid()),
		})
	}

	if selectors, exists := probes.SelectorsPerEventType["*"]; exists {
		p.managerOptions.ActivatedProbes = append(p.managerOptions.ActivatedProbes, selectors...)
	}

	if err := p.manager.InitWithOptions(bytecodeReader, p.managerOptions); err != nil {
		return errors.Wrap(err, "failed to init manager")
	}

	pidDiscardersMap, err := p.Map("pid_discarders")
	if err != nil {
		return err
	}
	p.pidDiscarders = newPidDiscarders(pidDiscardersMap, p.erpc)

	inodeDiscardersMap, err := p.Map("inode_discarders")
	if err != nil {
		return err
	}

	discarderRevisionsMap, err := p.Map("discarder_revisions")
	if err != nil {
		return err
	}

	if p.inodeDiscarders, err = newInodeDiscarders(inodeDiscardersMap, discarderRevisionsMap, p.erpc, p.resolvers.DentryResolver); err != nil {
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

	return p.monitor.Start(p.ctx)
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

func (p *Probe) handleLostEvents(CPU int, count uint64, perfMap *manager.PerfMap, manager *manager.Manager) {
	log.Tracef("lost %d events", count)
	p.monitor.perfBufferMonitor.CountLostEvent(count, perfMap, CPU)
}

func (p *Probe) zeroEvent() *Event {
	*p.event = eventZero
	return p.event
}

func (p *Probe) unmarshalProcessContainer(data []byte, event *Event) (int, error) {
	read, err := model.UnmarshalBinary(data, &event.ProcessContext, &event.ContainerContext)
	if err != nil {
		return 0, err
	}

	return read, nil
}

func (p *Probe) invalidateDentry(mountID uint32, inode uint64, revision uint32) {
	// sanity check
	if mountID == 0 || inode == 0 {
		log.Errorf("invalid mount_id/inode tuple %d:%d", mountID, inode)
	}

	if p.resolvers.MountResolver.IsOverlayFS(mountID) {
		log.Tracef("remove all dentry entries for mount id %d", mountID)
		p.resolvers.DentryResolver.DelCacheEntries(mountID)

		p.inodeDiscarders.setRevision(mountID, revision)
	} else {
		log.Tracef("remove dentry cache entry for inode %d", inode)
		p.resolvers.DentryResolver.DelCacheEntry(mountID, inode)
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

	// no need to dispatch events
	switch eventType {
	case model.MountReleasedEventType:
		if _, err := event.MountReleased.UnmarshalBinary(data[offset:]); err != nil {
			log.Errorf("failed to decode mount released event: %s (offset %d, len %d)", err, offset, dataLen)
			return
		}

		// Remove all dentry entries belonging to the mountID
		p.resolvers.DentryResolver.DelCacheEntries(event.MountReleased.MountID)

		if p.resolvers.MountResolver.IsOverlayFS(event.MountReleased.MountID) {
			p.inodeDiscarders.setRevision(event.MountReleased.MountID, event.MountReleased.DiscarderRevision)
		}

		// Delete new mount point from cache
		if err := p.resolvers.MountResolver.Delete(event.MountReleased.MountID); err != nil {
			log.Warnf("failed to delete mount point %d from cache: %s", event.MountReleased.MountID, err)
		}
		return
	case model.InvalidateDentryEventType:
		if _, err := event.InvalidateDentry.UnmarshalBinary(data[offset:]); err != nil {
			log.Errorf("failed to decode invalidate dentry event: %s (offset %d, len %d)", err, offset, dataLen)
			return
		}

		p.invalidateDentry(event.InvalidateDentry.MountID, event.InvalidateDentry.Inode, event.InvalidateDentry.DiscarderRevision)

		return
	case model.ArgsEnvsEventType:
		if _, err := event.ArgsEnvs.UnmarshalBinary(data[offset:]); err != nil {
			log.Errorf("failed to decode args envs event: %s (offset %d, len %d)", err, offset, dataLen)
			return
		}

		p.resolvers.ProcessResolver.UpdateArgsEnvs(&event.ArgsEnvs)

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
		event.SetMountPoint(&event.Mount)
		// Resolve root
		event.SetMountRoot(&event.Mount)
		// Insert new mount point in cache
		p.resolvers.MountResolver.Insert(event.Mount)

		// There could be entries of a previous mount_id in the cache for instance,
		// runc does the following : it bind mounts itself (using /proc/exe/self),
		// opens a file descriptor on the new file with O_CLOEXEC then umount the bind mount using
		// MNT_DETACH. It then does an exec syscall, that will cause the fd to be closed.
		// Our dentry resolution of the exec event causes the inode/mount_id to be put in cache,
		// so we remove all dentry entries belonging to the mountID.
		p.resolvers.DentryResolver.DelCacheEntries(event.Mount.MountID)
	case model.FileUmountEventType:
		if _, err := event.Umount.UnmarshalBinary(data[offset:]); err != nil {
			log.Errorf("failed to decode umount event: %s (offset %d, len %d)", err, offset, dataLen)
			return
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
		defer p.invalidateDentry(event.Rmdir.File.MountID, event.Rmdir.File.Inode, event.Rmdir.DiscarderRevision)
	case model.FileUnlinkEventType:
		if _, err := event.Unlink.UnmarshalBinary(data[offset:]); err != nil {
			log.Errorf("failed to decode unlink event: %s (offset %d, len %d)", err, offset, dataLen)
			return
		}

		// defer it do ensure that it will be done after the dispatch that could re-add it
		defer p.invalidateDentry(event.Unlink.File.MountID, event.Unlink.File.Inode, event.Unlink.DiscarderRevision)
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
		if _, err := event.UnmarshalProcess(data[offset:]); err != nil {
			log.Errorf("failed to decode fork event: %s (offset %d, len %d)", err, offset, dataLen)
			return
		}

		p.resolvers.ProcessResolver.ApplyBootTime(event.processCacheEntry)

		p.resolvers.ProcessResolver.AddForkEntry(event.ProcessContext.Pid, event.processCacheEntry)
	case model.ExecEventType:
		// unmarshal and fill event.processCacheEntry
		if _, err := event.UnmarshalProcess(data[offset:]); err != nil {
			log.Errorf("failed to decode exec event: %s (offset %d, len %d)", err, offset, len(data))
			return
		}
		p.resolvers.ProcessResolver.SetProcessArgs(event.processCacheEntry)
		p.resolvers.ProcessResolver.SetProcessEnvs(event.processCacheEntry)

		if _, err := p.resolvers.ProcessResolver.SetProcessPath(event.processCacheEntry); err != nil {
			log.Debugf("failed to resolve exec path: %s", err)
		}
		p.resolvers.ProcessResolver.SetProcessFilesystem(event.processCacheEntry)
		p.resolvers.ProcessResolver.SetProcessContainerPath(event.processCacheEntry)

		p.resolvers.ProcessResolver.SetProcessTTY(event.processCacheEntry)

		p.resolvers.ProcessResolver.SetProcessUsersGroups(event.processCacheEntry)

		p.resolvers.ProcessResolver.ApplyBootTime(event.processCacheEntry)

		p.resolvers.ProcessResolver.AddExecEntry(event.ProcessContext.Pid, event.processCacheEntry)

		// copy some of the field from the entry
		event.Exec.Process = event.processCacheEntry.Process
		event.Exec.FileFields = event.processCacheEntry.Process.FileFields
	case model.ExitEventType:
		defer p.resolvers.ProcessResolver.DeleteEntry(event.ProcessContext.Pid, event.ResolveEventTimestamp())
	case model.SetuidEventType:
		if _, err := event.SetUID.UnmarshalBinary(data[offset:]); err != nil {
			log.Errorf("failed to decode setuid event: %s (offset %d, len %d)", err, offset, len(data))
			return
		}
		defer p.resolvers.ProcessResolver.UpdateUID(event.ProcessContext.Pid, event)
	case model.SetgidEventType:
		if _, err := event.SetGID.UnmarshalBinary(data[offset:]); err != nil {
			log.Errorf("failed to decode setgid event: %s (offset %d, len %d)", err, offset, len(data))
			return
		}
		defer p.resolvers.ProcessResolver.UpdateGID(event.ProcessContext.Pid, event)
	case model.CapsetEventType:
		if _, err := event.Capset.UnmarshalBinary(data[offset:]); err != nil {
			log.Errorf("failed to decode capset event: %s (offset %d, len %d)", err, offset, len(data))
			return
		}
		defer p.resolvers.ProcessResolver.UpdateCapset(event.ProcessContext.Pid, event)
	default:
		log.Errorf("unsupported event type %d", eventType)
		return
	}

	// resolve event context
	if eventType != model.ExitEventType {
		event.ResolveProcessCacheEntry()

		// in case of exec event we take the parent a process context as this
		// the parent which generated the exec
		if eventType == model.ExecEventType {
			if ancestor := event.processCacheEntry.ProcessContext.Ancestor; ancestor != nil {
				event.ProcessContext = ancestor.ProcessContext
			}
		} else {
			event.ProcessContext = event.processCacheEntry.ProcessContext
		}
	}

	p.DispatchEvent(event, dataLen, int(CPU), p.perfMap)

	// flush exited process
	p.resolvers.ProcessResolver.DequeueExited()
}

// OnRuleMatch is called when a rule matches just before sending
func (p *Probe) OnRuleMatch(rule *rules.Rule, event *Event) {
	// ensure that all the fields are resolved before sending
	event.ResolveContainerID(&event.ContainerContext)
	event.ResolveContainerTags(&event.ContainerContext)
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
	log.Debug("Freezing discarders")

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
	var mapValue [256]byte

	var inode inodeDiscarder
	for entries := p.inodeDiscarders.Iterate(); entries.Next(&inode, unsafe.Pointer(&mapValue[0])); {
		discardedInodes = append(discardedInodes, inode)
	}

	var discardedPids []uint32
	for pid, entries := uint32(0), p.pidDiscarders.Iterate(); entries.Next(&pid, unsafe.Pointer(&mapValue[0])); {
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
			if err := p.inodeDiscarders.Delete(unsafe.Pointer(&inode)); err != nil {
				log.Tracef("Failed to flush discarder for inode %d: %s", inode, err)
			}

			discarderCount--
			if discarderCount > 0 {
				time.Sleep(delay)
			}
		}

		for _, pid := range discardedPids {
			if err := p.pidDiscarders.Delete(unsafe.Pointer(&pid)); err != nil {
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
	// We need to stop the manager first, otherwise there can be a dead lock between the eBPF perf map reader and the
	// reorderer (at the channel level)
	if err := p.manager.Stop(manager.CleanAll); err != nil {
		return err
	}
	p.cancelFnc()
	return p.resolvers.Close()
}

// GetDebugStats returns the debug stats
func (p *Probe) GetDebugStats() map[string]interface{} {
	debug := map[string]interface{}{
		"start_time": p.startTime.String(),
	}
	// TODO(Will): add manager state
	return debug
}

// NewRuleSet returns a new rule set
func (p *Probe) NewRuleSet(opts *rules.Opts) *rules.RuleSet {
	eventCtor := func() eval.Event {
		return NewEvent(p.resolvers, p.scrubber)
	}
	opts.Logger = seclog.DatadogAgentLogger{}

	return rules.NewRuleSet(&Model{}, eventCtor, opts)
}

// NewProbe instantiates a new runtime security agent probe
func NewProbe(config *config.Config, client *statsd.Client) (*Probe, error) {
	erpc, err := NewERPC()
	if err != nil {
		return nil, err
	}

	ctx, cancel := context.WithCancel(context.Background())

	p := &Probe{
		config:         config,
		approvers:      make(map[eval.EventType]activeApprovers),
		manager:        ebpf.NewRuntimeSecurityManager(),
		managerOptions: ebpf.NewDefaultOptions(),
		ctx:            ctx,
		cancelFnc:      cancel,
		statsdClient:   client,
		erpc:           erpc,
	}

	if err = p.detectKernelVersion(); err != nil {
		return nil, err
	}

	numCPU, err := utils.NumCPU()
	if err != nil {
		return nil, errors.Wrap(err, "failed to parse CPU count")
	}
	p.managerOptions.MapSpecEditors = probes.AllMapSpecEditors(numCPU)

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
			Name:  "runtime_pid",
			Value: uint64(utils.Getpid()),
		},
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
		manager.ConstantEditor{
			Name:  "getattr2",
			Value: getAttr2(p),
		},
	)
	p.managerOptions.ConstantEditors = append(p.managerOptions.ConstantEditors, TTYConstants(p)...)
	p.managerOptions.ConstantEditors = append(p.managerOptions.ConstantEditors, erpc.GetConstants()...)
	p.managerOptions.ConstantEditors = append(p.managerOptions.ConstantEditors, DiscarderConstants...)
	p.managerOptions.ConstantEditors = append(p.managerOptions.ConstantEditors, getCGroupWriteConstants())

	// constants syscall monitor
	if p.config.SyscallMonitor {
		p.managerOptions.ConstantEditors = append(p.managerOptions.ConstantEditors, manager.ConstantEditor{
			Name:  "syscall_monitor",
			Value: uint64(1),
		})
	}

	// tail calls
	p.managerOptions.TailCallRouter = probes.AllTailRoutes()

	resolvers, err := NewResolvers(config, p)
	if err != nil {
		return nil, err
	}
	p.resolvers = resolvers

	p.reOrderer = NewReOrderer(p.handleEvent,
		ExtractEventInfo,
		ReOrdererOpts{
			QueueSize:  10000,
			Rate:       50 * time.Millisecond,
			Retention:  5,
			MetricRate: 5 * time.Second,
		})

	p.scrubber = pconfig.NewDefaultDataScrubber()
	p.scrubber.AddCustomSensitiveWords(config.CustomSensitiveWords)

	p.event = NewEvent(p.resolvers, p.scrubber)

	eventZero.resolvers = p.resolvers
	eventZero.scrubber = p.scrubber

	return p, nil
}
