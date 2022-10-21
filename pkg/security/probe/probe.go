// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux
// +build linux

package probe

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"
	"unsafe"

	"github.com/DataDog/datadog-go/v5/statsd"
	manager "github.com/DataDog/ebpf-manager"
	lib "github.com/cilium/ebpf"
	"github.com/hashicorp/go-multierror"
	"github.com/moby/sys/mountinfo"
	"github.com/vishvananda/netlink"
	"go.uber.org/atomic"
	"golang.org/x/exp/slices"
	"golang.org/x/sys/unix"
	"golang.org/x/time/rate"

	aconfig "github.com/DataDog/datadog-agent/pkg/config"
	pconfig "github.com/DataDog/datadog-agent/pkg/process/config"
	"github.com/DataDog/datadog-agent/pkg/process/util"
	"github.com/DataDog/datadog-agent/pkg/security/api"
	"github.com/DataDog/datadog-agent/pkg/security/config"
	"github.com/DataDog/datadog-agent/pkg/security/ebpf"
	kernel "github.com/DataDog/datadog-agent/pkg/security/ebpf/kernel"
	"github.com/DataDog/datadog-agent/pkg/security/ebpf/probes"
	"github.com/DataDog/datadog-agent/pkg/security/metrics"
	"github.com/DataDog/datadog-agent/pkg/security/probe/constantfetch"
	"github.com/DataDog/datadog-agent/pkg/security/secl/compiler/eval"
	"github.com/DataDog/datadog-agent/pkg/security/secl/model"
	"github.com/DataDog/datadog-agent/pkg/security/secl/rules"
	"github.com/DataDog/datadog-agent/pkg/security/seclog"
	"github.com/DataDog/datadog-agent/pkg/security/utils"
	utilkernel "github.com/DataDog/datadog-agent/pkg/util/kernel"
)

// ActivityDumpHandler represents an handler for the activity dumps sent by the probe
type ActivityDumpHandler interface {
	HandleActivityDump(dump *api.ActivityDumpStreamMessage)
}

// EventHandler represents an handler for the events sent by the probe
type EventHandler interface {
	HandleEvent(event *Event)
	HandleCustomEvent(rule *rules.Rule, event *CustomEvent)
}

// EventStream describes the interface implemented by reordered perf maps or ring buffers
type EventStream interface {
	Init(*manager.Manager, *config.Config) error
	SetMonitor(*PerfBufferMonitor)
	Start(*sync.WaitGroup) error
	Pause() error
	Resume() error
}

// NotifyDiscarderPushedCallback describe the callback used to retrieve pushed discarders information
type NotifyDiscarderPushedCallback func(eventType string, event *Event, field string)

// Probe represents the runtime security eBPF probe in charge of
// setting up the required kProbes and decoding events sent from the kernel
type Probe struct {
	// Constants and configuration
	manager        *manager.Manager
	managerOptions manager.Options
	config         *config.Config
	statsdClient   statsd.ClientInterface
	startTime      time.Time
	kernelVersion  *kernel.Version
	_              uint32 // padding for goarch=386
	ctx            context.Context
	cancelFnc      context.CancelFunc
	wg             sync.WaitGroup
	// Events section
	handlers    [model.MaxAllEventType][]EventHandler
	monitor     *Monitor
	resolvers   *Resolvers
	event       *Event
	eventStream EventStream
	scrubber    *pconfig.DataScrubber

	// ActivityDumps section
	activityDumpHandler ActivityDumpHandler

	// Approvers / discarders section
	erpc                               *ERPC
	erpcRequest                        *ERPCRequest
	pidDiscarders                      *pidDiscarders
	inodeDiscarders                    *inodeDiscarders
	flushingDiscarders                 *atomic.Bool
	approvers                          map[eval.EventType]activeApprovers
	discarderRateLimiter               *rate.Limiter
	notifyDiscarderPushedCallbacks     []NotifyDiscarderPushedCallback
	notifyDiscarderPushedCallbacksLock sync.Mutex

	constantOffsets map[string]uint64
	runtimeCompiled bool

	// network section
	tcProgramsLock sync.RWMutex
	tcPrograms     map[NetDeviceKey]*manager.Probe

	isRuntimeDiscarded bool
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
		return fmt.Errorf("unable to detect the kernel version: %w", err)
	}
	p.kernelVersion = kernelVersion
	return nil
}

// GetKernelVersion computes and returns the running kernel version
func (p *Probe) GetKernelVersion() (*kernel.Version, error) {
	if err := p.detectKernelVersion(); err != nil {
		return nil, err
	}
	return p.kernelVersion, nil
}

// UseRingBuffers returns true if eBPF ring buffers are supported and used
func (p *Probe) UseRingBuffers() bool {
	return p.kernelVersion.HaveRingBuffers() && p.config.EventStreamUseRingBuffer
}

func (p *Probe) sanityChecks() error {
	// make sure debugfs is mounted
	if mounted, err := utilkernel.IsDebugFSMounted(); !mounted {
		return err
	}

	if utilkernel.GetLockdownMode() == utilkernel.Confidentiality {
		return errors.New("eBPF not supported in lockdown `confidentiality` mode")
	}

	if p.config.NetworkEnabled && p.kernelVersion.IsRH7Kernel() {
		seclog.Warnf("The network feature of CWS isn't supported on Centos7, setting runtime_security_config.network.enabled to false")
		p.config.NetworkEnabled = false
	}

	return nil
}

// VerifyOSVersion returns an error if the current kernel version is not supported
func (p *Probe) VerifyOSVersion() error {
	if !p.kernelVersion.IsRH7Kernel() && !p.kernelVersion.IsRH8Kernel() && p.kernelVersion.Code < kernel.Kernel4_15 {
		return fmt.Errorf("the following kernel is not supported: %s", p.kernelVersion)
	}
	return nil
}

// VerifyEnvironment returns an error if the current environment seems to be misconfigured
func (p *Probe) VerifyEnvironment() *multierror.Error {
	var err *multierror.Error
	if aconfig.IsContainerized() {
		if mounted, _ := mountinfo.Mounted("/etc/passwd"); !mounted {
			err = multierror.Append(err, errors.New("/etc/passwd doesn't seem to be a mountpoint"))
		}

		if mounted, _ := mountinfo.Mounted("/etc/group"); !mounted {
			err = multierror.Append(err, errors.New("/etc/group doesn't seem to be a mountpoint"))
		}

		if mounted, _ := mountinfo.Mounted(util.HostProc()); !mounted {
			err = multierror.Append(err, errors.New("/etc/group doesn't seem to be a mountpoint"))
		}

		if mounted, _ := mountinfo.Mounted(p.kernelVersion.OsReleasePath); !mounted {
			err = multierror.Append(err, fmt.Errorf("%s doesn't seem to be a mountpoint", p.kernelVersion.OsReleasePath))
		}

		securityFSPath := filepath.Join(util.GetSysRoot(), "kernel/security")
		if mounted, _ := mountinfo.Mounted(securityFSPath); !mounted {
			err = multierror.Append(err, fmt.Errorf("%s doesn't seem to be a mountpoint", securityFSPath))
		}

		capsEffective, _, capErr := utils.CapEffCapEprm(utils.Getpid())
		if capErr != nil {
			err = multierror.Append(capErr, errors.New("failed to get process capabilities"))
		} else {
			requiredCaps := []string{
				"CAP_SYS_ADMIN",
				"CAP_SYS_RESOURCE",
				"CAP_SYS_PTRACE",
				"CAP_NET_ADMIN",
				"CAP_NET_BROADCAST",
				"CAP_NET_RAW",
				"CAP_IPC_LOCK",
				"CAP_CHOWN",
			}

			for _, requiredCap := range requiredCaps {
				capConst := model.KernelCapabilityConstants[requiredCap]
				if capsEffective&capConst == 0 {
					err = multierror.Append(err, fmt.Errorf("%s capability is missing", requiredCap))
				}
			}
		}
	}

	return err
}

func isSyscallWrapperRequired() (bool, error) {
	openSyscall, err := manager.GetSyscallFnName("open")
	if err != nil {
		return false, err
	}

	return !strings.HasPrefix(openSyscall, "SyS_") && !strings.HasPrefix(openSyscall, "sys_"), nil
}

// Init initializes the probe
func (p *Probe) Init() error {
	p.startTime = time.Now()

	useSyscallWrapper, err := isSyscallWrapperRequired()
	if err != nil {
		return err
	}

	loader := ebpf.NewProbeLoader(p.config, useSyscallWrapper, p.statsdClient)
	defer loader.Close()

	bytecodeReader, runtimeCompiled, err := loader.Load()
	if err != nil {
		return err
	}
	defer bytecodeReader.Close()

	p.runtimeCompiled = runtimeCompiled

	if err := p.eventStream.Init(p.manager, p.config); err != nil {
		return err
	}

	if p.isRuntimeDiscarded {
		p.managerOptions.ConstantEditors = append(p.managerOptions.ConstantEditors, manager.ConstantEditor{
			Name:  "runtime_discarded",
			Value: uint64(1),
		})
	}

	p.managerOptions.ActivatedProbes = append(p.managerOptions.ActivatedProbes, probes.SnapshotSelectors...)

	if err := p.manager.InitWithOptions(bytecodeReader, p.managerOptions); err != nil {
		return fmt.Errorf("failed to init manager: %w", err)
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

	p.inodeDiscarders = newInodeDiscarders(inodeDiscardersMap, p.erpc, p.resolvers.DentryResolver)

	if err := p.resolvers.Start(p.ctx); err != nil {
		return err
	}

	p.monitor, err = NewMonitor(p)
	if err != nil {
		return err
	}

	p.eventStream.SetMonitor(p.monitor.perfBufferMonitor)

	return nil
}

// IsRuntimeCompiled returns true if the eBPF programs where successfully runtime compiled
func (p *Probe) IsRuntimeCompiled() bool {
	return p.runtimeCompiled
}

// Setup the runtime security probe
func (p *Probe) Setup() error {
	if err := p.manager.Start(); err != nil {
		return err
	}

	return p.monitor.Start(p.ctx, &p.wg)
}

// Start processing events
func (p *Probe) Start() error {
	return p.eventStream.Start(&p.wg)
}

// AddActivityDumpHandler set the probe activity dump handler
func (p *Probe) AddActivityDumpHandler(handler ActivityDumpHandler) {
	p.activityDumpHandler = handler
}

// AddEventHandler set the probe event handler
func (p *Probe) AddEventHandler(eventType model.EventType, handler EventHandler) {
	p.handlers[eventType] = append(p.handlers[eventType], handler)
}

// DispatchEvent sends an event to the probe event handler
func (p *Probe) DispatchEvent(event *Event) {
	seclog.TraceTagf(event.GetEventType(), "Dispatching event %s", event)

	// send wildcard first
	for _, handler := range p.handlers[model.UnknownEventType] {
		handler.HandleEvent(event)
	}

	// send specific event
	for _, handler := range p.handlers[event.GetEventType()] {
		handler.HandleEvent(event)
	}

	// Process after evaluation because some monitors need the DentryResolver to have been called first.
	p.monitor.ProcessEvent(event)
}

// DispatchActivityDump sends an activity dump to the probe activity dump handler
func (p *Probe) DispatchActivityDump(dump *api.ActivityDumpStreamMessage) {
	if handler := p.activityDumpHandler; handler != nil {
		handler.HandleActivityDump(dump)
	}
}

// DispatchCustomEvent sends a custom event to the probe event handler
func (p *Probe) DispatchCustomEvent(rule *rules.Rule, event *CustomEvent) {
	seclog.TraceTagf(event.GetEventType(), "Dispatching custom event %s", event)

	// send specific event
	if p.config.AgentMonitoringEvents {
		// send wildcard first
		for _, handler := range p.handlers[model.UnknownEventType] {
			handler.HandleCustomEvent(rule, event)
		}

		// send specific event
		for _, handler := range p.handlers[event.GetEventType()] {
			handler.HandleCustomEvent(rule, event)
		}
	}
}

func (p *Probe) sendTCProgramsStats() {
	p.tcProgramsLock.RLock()
	defer p.tcProgramsLock.RUnlock()

	if val := float64(len(p.tcPrograms)); val > 0 {
		_ = p.statsdClient.Gauge(metrics.MetricTCProgram, val, []string{}, 1.0)
	}
}

// SendStats sends statistics about the probe to Datadog
func (p *Probe) SendStats() error {
	p.sendTCProgramsStats()

	return p.monitor.SendStats()
}

// GetMonitor returns the monitor of the probe
func (p *Probe) GetMonitor() *Monitor {
	return p.monitor
}

func (p *Probe) zeroEvent() *Event {
	*p.event = eventZero
	return p.event
}

func (p *Probe) unmarshalContexts(data []byte, event *Event) (int, error) {
	read, err := model.UnmarshalBinary(data, &event.PIDContext, &event.SpanContext, &event.ContainerContext)
	if err != nil {
		return 0, err
	}

	return read, nil
}

func (p *Probe) invalidateDentry(mountID uint32, inode uint64) {
	// sanity check
	if mountID == 0 || inode == 0 {
		seclog.Tracef("invalid mount_id/inode tuple %d:%d", mountID, inode)
		return
	}

	seclog.Tracef("remove dentry cache entry for inode %d", inode)
	p.resolvers.DentryResolver.DelCacheEntry(mountID, inode)
}

func (p *Probe) handleEvent(CPU int, data []byte) {
	offset := 0
	event := p.zeroEvent()

	dataLen := uint64(len(data))

	read, err := event.UnmarshalBinary(data)
	if err != nil {
		seclog.Errorf("failed to decode event: %s", err)
		return
	}
	offset += read

	eventType := event.GetEventType()
	p.monitor.perfBufferMonitor.CountEvent(eventType, event.TimestampRaw, 1, dataLen, eventStreamMap, CPU)

	// no need to dispatch events
	switch eventType {
	case model.MountReleasedEventType:
		if _, err = event.MountReleased.UnmarshalBinary(data[offset:]); err != nil {
			seclog.Errorf("failed to decode mount released event: %s (offset %d, len %d)", err, offset, dataLen)
			return
		}

		// Remove all dentry entries belonging to the mountID
		p.resolvers.DentryResolver.DelCacheEntries(event.MountReleased.MountID)

		// Delete new mount point from cache
		if err = p.resolvers.MountResolver.Delete(event.MountReleased.MountID); err != nil {
			seclog.Debugf("failed to delete mount point %d from cache: %s", event.MountReleased.MountID, err)
		}
		return
	case model.InvalidateDentryEventType:
		if _, err = event.InvalidateDentry.UnmarshalBinary(data[offset:]); err != nil {
			seclog.Errorf("failed to decode invalidate dentry event: %s (offset %d, len %d)", err, offset, dataLen)
			return
		}

		p.invalidateDentry(event.InvalidateDentry.MountID, event.InvalidateDentry.Inode)

		return
	case model.ArgsEnvsEventType:
		if _, err = event.ArgsEnvs.UnmarshalBinary(data[offset:]); err != nil {
			seclog.Errorf("failed to decode args envs event: %s (offset %d, len %d)", err, offset, dataLen)
			return
		}

		p.resolvers.ProcessResolver.UpdateArgsEnvs(&event.ArgsEnvs)

		return
	case model.CgroupTracingEventType:
		if !p.config.ActivityDumpEnabled {
			seclog.Errorf("shouldn't receive Cgroup event if activity dumps are disabled")
			return
		}

		if _, err = event.CgroupTracing.UnmarshalBinary(data[offset:]); err != nil {
			seclog.Errorf("failed to decode cgroup tracing event: %s (offset %d, len %d)", err, offset, dataLen)
			return
		}

		p.monitor.activityDumpManager.HandleCgroupTracingEvent(&event.CgroupTracing)
		return
	}

	read, err = p.unmarshalContexts(data[offset:], event)
	if err != nil {
		seclog.Errorf("failed to decode event `%s`: %s", eventType, err)
		return
	}
	offset += read

	// save netns handle if applicable
	nsPath := utils.NetNSPathFromPid(event.PIDContext.Pid)
	_, _ = p.resolvers.NamespaceResolver.SaveNetworkNamespaceHandle(event.PIDContext.NetNS, nsPath)

	if model.GetEventTypeCategory(eventType.String()) == model.NetworkCategory {
		if read, err = event.NetworkContext.UnmarshalBinary(data[offset:]); err != nil {
			seclog.Errorf("failed to decode Network Context")
		}
		offset += read
	}

	switch eventType {
	case model.FileMountEventType:
		if _, err = event.Mount.UnmarshalBinary(data[offset:]); err != nil {
			seclog.Errorf("failed to decode mount event: %s (offset %d, len %d)", err, offset, dataLen)
			return
		}

		// There could be entries of a previous mount_id in the cache for instance,
		// runc does the following : it bind mounts itself (using /proc/exe/self),
		// opens a file descriptor on the new file with O_CLOEXEC then umount the bind mount using
		// MNT_DETACH. It then does an exec syscall, that will cause the fd to be closed.
		// Our dentry resolution of the exec event causes the inode/mount_id to be put in cache,
		// so we remove all dentry entries belonging to the mountID.
		p.resolvers.DentryResolver.DelCacheEntries(event.Mount.MountID)

		// Resolve mount point
		event.SetMountPoint(&event.Mount)
		// Resolve root
		event.SetMountRoot(&event.Mount)
		// Insert new mount point in cache
		err = p.resolvers.MountResolver.Insert(event.Mount)
		if err != nil {
			seclog.Errorf("failed to insert mount event: %v", err)
		}

		if event.Mount.GetFSType() == "nsfs" {
			nsid := uint32(event.Mount.RootInode)
			_, mountPath, _, _ := p.resolvers.MountResolver.GetMountPath(event.Mount.MountID, event.PIDContext.Pid)
			mountNetNSPath := utils.NetNSPathFromPath(mountPath)
			_, _ = p.resolvers.NamespaceResolver.SaveNetworkNamespaceHandle(nsid, mountNetNSPath)
		}
	case model.FileUmountEventType:
		if _, err = event.Umount.UnmarshalBinary(data[offset:]); err != nil {
			seclog.Errorf("failed to decode umount event: %s (offset %d, len %d)", err, offset, dataLen)
			return
		}

		mount := p.resolvers.MountResolver.Get(event.Umount.MountID, event.PIDContext.Pid)
		if mount != nil && mount.GetFSType() == "nsfs" {
			nsid := uint32(mount.RootInode)
			if namespace := p.resolvers.NamespaceResolver.ResolveNetworkNamespace(nsid); namespace != nil {
				p.FlushNetworkNamespace(namespace)
			}
		}

	case model.FileOpenEventType:
		if _, err = event.Open.UnmarshalBinary(data[offset:]); err != nil {
			seclog.Errorf("failed to decode open event: %s (offset %d, len %d)", err, offset, dataLen)
			return
		}
	case model.FileMkdirEventType:
		if _, err = event.Mkdir.UnmarshalBinary(data[offset:]); err != nil {
			seclog.Errorf("failed to decode mkdir event: %s (offset %d, len %d)", err, offset, dataLen)
			return
		}
	case model.FileRmdirEventType:
		if _, err = event.Rmdir.UnmarshalBinary(data[offset:]); err != nil {
			seclog.Errorf("failed to decode rmdir event: %s (offset %d, len %d)", err, offset, dataLen)
			return
		}

		if event.Rmdir.Retval >= 0 {
			// defer it do ensure that it will be done after the dispatch that could re-add it
			defer p.invalidateDentry(event.Rmdir.File.MountID, event.Rmdir.File.Inode)
		}
	case model.FileUnlinkEventType:
		if _, err = event.Unlink.UnmarshalBinary(data[offset:]); err != nil {
			seclog.Errorf("failed to decode unlink event: %s (offset %d, len %d)", err, offset, dataLen)
			return
		}

		if event.Unlink.Retval >= 0 {
			// defer it do ensure that it will be done after the dispatch that could re-add it
			defer p.invalidateDentry(event.Unlink.File.MountID, event.Unlink.File.Inode)
		}
	case model.FileRenameEventType:
		if _, err = event.Rename.UnmarshalBinary(data[offset:]); err != nil {
			seclog.Errorf("failed to decode rename event: %s (offset %d, len %d)", err, offset, dataLen)
			return
		}

		if event.Rename.Retval >= 0 {
			// defer it do ensure that it will be done after the dispatch that could re-add it
			defer p.invalidateDentry(event.Rename.New.MountID, event.Rename.New.Inode)
		}
	case model.FileChmodEventType:
		if _, err = event.Chmod.UnmarshalBinary(data[offset:]); err != nil {
			seclog.Errorf("failed to decode chmod event: %s (offset %d, len %d)", err, offset, dataLen)
			return
		}
	case model.FileChownEventType:
		if _, err = event.Chown.UnmarshalBinary(data[offset:]); err != nil {
			seclog.Errorf("failed to decode chown event: %s (offset %d, len %d)", err, offset, dataLen)
			return
		}
	case model.FileUtimesEventType:
		if _, err = event.Utimes.UnmarshalBinary(data[offset:]); err != nil {
			seclog.Errorf("failed to decode utime event: %s (offset %d, len %d)", err, offset, dataLen)
			return
		}
	case model.FileLinkEventType:
		if _, err = event.Link.UnmarshalBinary(data[offset:]); err != nil {
			seclog.Errorf("failed to decode link event: %s (offset %d, len %d)", err, offset, dataLen)
			return
		}

		// need to invalidate as now nlink > 1
		if event.Link.Retval >= 0 {
			// defer it do ensure that it will be done after the dispatch that could re-add it
			defer p.invalidateDentry(event.Link.Source.MountID, event.Link.Source.Inode)
		}
	case model.FileSetXAttrEventType:
		if _, err = event.SetXAttr.UnmarshalBinary(data[offset:]); err != nil {
			seclog.Errorf("failed to decode setxattr event: %s (offset %d, len %d)", err, offset, dataLen)
			return
		}
	case model.FileRemoveXAttrEventType:
		if _, err = event.RemoveXAttr.UnmarshalBinary(data[offset:]); err != nil {
			seclog.Errorf("failed to decode removexattr event: %s (offset %d, len %d)", err, offset, dataLen)
			return
		}
	case model.ForkEventType:
		if _, err = event.UnmarshalProcessCacheEntry(data[offset:]); err != nil {
			seclog.Errorf("failed to decode fork event: %s (offset %d, len %d)", err, offset, dataLen)
			return
		}

		if IsKThread(event.ProcessCacheEntry.PPid, event.ProcessCacheEntry.Pid) {
			return
		}

		p.resolvers.ProcessResolver.ApplyBootTime(event.ProcessCacheEntry)
		event.ProcessCacheEntry.SetSpan(event.SpanContext.SpanID, event.SpanContext.TraceID)

		p.resolvers.ProcessResolver.AddForkEntry(event.ProcessCacheEntry)
	case model.ExecEventType:
		// unmarshal and fill event.processCacheEntry
		if _, err = event.UnmarshalProcessCacheEntry(data[offset:]); err != nil {
			seclog.Errorf("failed to decode exec event: %s (offset %d, len %d)", err, offset, len(data))
			return
		}

		if err = p.resolvers.ProcessResolver.ResolveNewProcessCacheEntry(event.ProcessCacheEntry); err != nil {
			seclog.Debugf("failed to resolve new process cache entry context: %s", err)
		}

		p.resolvers.ProcessResolver.AddExecEntry(event.ProcessCacheEntry)

		event.Exec.Process = &event.ProcessCacheEntry.Process
	case model.ExitEventType:
		if _, err = event.Exit.UnmarshalBinary(data[offset:]); err != nil {
			seclog.Errorf("failed to decode exit event: %s (offset %d, len %d)", err, offset, len(data))
			return
		}

		event.ProcessCacheEntry = event.ResolveProcessCacheEntry()
		// Use the event timestamp as exit time
		// The local process cache hasn't been updated yet with the exit time when the exit event is first seen
		// The pid_cache kernel map has the exit_time but it's only accessed if there's a local miss
		event.ProcessCacheEntry.Process.ExitTime = event.ResolveEventTimestamp()
		event.Exit.Process = &event.ProcessCacheEntry.Process
	case model.SetuidEventType:
		if _, err = event.SetUID.UnmarshalBinary(data[offset:]); err != nil {
			seclog.Errorf("failed to decode setuid event: %s (offset %d, len %d)", err, offset, len(data))
			return
		}
		defer p.resolvers.ProcessResolver.UpdateUID(event.PIDContext.Pid, event)
	case model.SetgidEventType:
		if _, err = event.SetGID.UnmarshalBinary(data[offset:]); err != nil {
			seclog.Errorf("failed to decode setgid event: %s (offset %d, len %d)", err, offset, len(data))
			return
		}
		defer p.resolvers.ProcessResolver.UpdateGID(event.PIDContext.Pid, event)
	case model.CapsetEventType:
		if _, err = event.Capset.UnmarshalBinary(data[offset:]); err != nil {
			seclog.Errorf("failed to decode capset event: %s (offset %d, len %d)", err, offset, len(data))
			return
		}
		defer p.resolvers.ProcessResolver.UpdateCapset(event.PIDContext.Pid, event)
	case model.SELinuxEventType:
		if _, err = event.SELinux.UnmarshalBinary(data[offset:]); err != nil {
			seclog.Errorf("failed to decode selinux event: %s (offset %d, len %d)", err, offset, len(data))
			return
		}
	case model.BPFEventType:
		if _, err = event.BPF.UnmarshalBinary(data[offset:]); err != nil {
			seclog.Errorf("failed to decode bpf event: %s (offset %d, len %d)", err, offset, len(data))
			return
		}
	case model.PTraceEventType:
		if _, err = event.PTrace.UnmarshalBinary(data[offset:]); err != nil {
			seclog.Errorf("failed to decode ptrace event: %s (offset %d, len %d)", err, offset, len(data))
			return
		}
		// resolve tracee process context
		cacheEntry := event.resolvers.ProcessResolver.Resolve(event.PTrace.PID, event.PTrace.PID)
		if cacheEntry != nil {
			event.PTrace.Tracee = &cacheEntry.ProcessContext
		}
	case model.MMapEventType:
		if _, err = event.MMap.UnmarshalBinary(data[offset:]); err != nil {
			seclog.Errorf("failed to decode mmap event: %s (offset %d, len %d)", err, offset, len(data))
			return
		}

		if event.MMap.Flags&unix.MAP_ANONYMOUS != 0 {
			// no need to trigger a dentry resolver, not backed by any file
			event.MMap.File.SetPathnameStr("")
			event.MMap.File.SetBasenameStr("")
		}
	case model.MProtectEventType:
		if _, err = event.MProtect.UnmarshalBinary(data[offset:]); err != nil {
			seclog.Errorf("failed to decode mprotect event: %s (offset %d, len %d)", err, offset, len(data))
			return
		}
	case model.LoadModuleEventType:
		if _, err = event.LoadModule.UnmarshalBinary(data[offset:]); err != nil {
			seclog.Errorf("failed to decode load_module event: %s (offset %d, len %d)", err, offset, len(data))
			return
		}

		if event.LoadModule.LoadedFromMemory {
			// no need to trigger a dentry resolver, not backed by any file
			event.LoadModule.File.SetPathnameStr("")
			event.LoadModule.File.SetBasenameStr("")
		}
	case model.UnloadModuleEventType:
		if _, err = event.UnloadModule.UnmarshalBinary(data[offset:]); err != nil {
			seclog.Errorf("failed to decode unload_module event: %s (offset %d, len %d)", err, offset, len(data))
		}
	case model.SignalEventType:
		if _, err = event.Signal.UnmarshalBinary(data[offset:]); err != nil {
			seclog.Errorf("failed to decode signal event: %s (offset %d, len %d)", err, offset, len(data))
			return
		}
		// resolve target process context
		cacheEntry := event.resolvers.ProcessResolver.Resolve(event.Signal.PID, event.Signal.PID)
		if cacheEntry != nil {
			event.Signal.Target = &cacheEntry.ProcessContext
		}
	case model.SpliceEventType:
		if _, err = event.Splice.UnmarshalBinary(data[offset:]); err != nil {
			seclog.Errorf("failed to decode splice event: %s (offset %d, len %d)", err, offset, len(data))
			return
		}
	case model.NetDeviceEventType:
		if _, err = event.NetDevice.UnmarshalBinary(data[offset:]); err != nil {
			seclog.Errorf("failed to decode net_device event: %s (offset %d, len %d)", err, offset, len(data))
			return
		}
		_ = p.setupNewTCClassifier(event.NetDevice.Device)
	case model.VethPairEventType:
		if _, err = event.VethPair.UnmarshalBinary(data[offset:]); err != nil {
			seclog.Errorf("failed to decode veth_pair event: %s (offset %d, len %d)", err, offset, len(data))
			return
		}
		_ = p.setupNewTCClassifier(event.VethPair.PeerDevice)
	case model.DNSEventType:
		if _, err = event.DNS.UnmarshalBinary(data[offset:]); err != nil {
			if errors.Is(err, model.ErrDNSNamePointerNotSupported) {
				seclog.Tracef("failed to decode DNS event: %s (offset %d, len %d)", err, offset, len(data))
			} else {
				seclog.Errorf("failed to decode DNS event: %s (offset %d, len %d)", err, offset, len(data))
			}

			return
		}
	case model.BindEventType:
		if _, err = event.Bind.UnmarshalBinary(data[offset:]); err != nil {
			seclog.Errorf("failed to decode bind event: %s (offset %d, len %d)", err, offset, len(data))
			return
		}
	case model.SyscallsEventType:
		if _, err = event.Syscalls.UnmarshalBinary(data[offset:]); err != nil {
			seclog.Errorf("failed to decode syscalls event: %s (offset %d, len %d)", err, offset, len(data))
			return
		}
	default:
		seclog.Errorf("unsupported event type %d", eventType)
		return
	}

	// resolve the process cache entry
	event.ProcessCacheEntry = event.ResolveProcessCacheEntry()

	// use ProcessCacheEntry process context as process context
	event.ProcessContext = &event.ProcessCacheEntry.ProcessContext

	if IsKThread(event.ProcessContext.PPid, event.ProcessContext.Pid) {
		return
	}

	if eventType == model.ExitEventType {
		defer p.resolvers.ProcessResolver.DeleteEntry(event.ProcessCacheEntry.Pid, event.ResolveEventTimestamp())
	}

	p.DispatchEvent(event)

	// flush exited process
	p.resolvers.ProcessResolver.DequeueExited()
}

// OnRuleMatch is called when a rule matches just before sending
func (p *Probe) OnRuleMatch(rule *rules.Rule, event *Event) {
	// ensure that all the fields are resolved before sending
	event.ResolveContainerID(&event.ContainerContext)
	event.ResolveContainerTags(&event.ContainerContext)
}

// AddNewNotifyDiscarderPushedCallback add a callback to the list of func that have to be called when a discarder is pushed to kernel
func (p *Probe) AddNewNotifyDiscarderPushedCallback(cb NotifyDiscarderPushedCallback) {
	p.notifyDiscarderPushedCallbacksLock.Lock()
	defer p.notifyDiscarderPushedCallbacksLock.Unlock()

	p.notifyDiscarderPushedCallbacks = append(p.notifyDiscarderPushedCallbacks, cb)
}

// OnNewDiscarder is called when a new discarder is found
func (p *Probe) OnNewDiscarder(rs *rules.RuleSet, event *Event, field eval.Field, eventType eval.EventType) error {
	// discarders disabled
	if !p.config.EnableDiscarders {
		return nil
	}

	if p.flushingDiscarders.Load() {
		return nil
	}

	if p.isRuntimeDiscarded {
		fakeTime := time.Unix(0, int64(event.TimestampRaw))
		if !p.discarderRateLimiter.AllowN(fakeTime, 1) {
			return nil
		}
	}

	seclog.Tracef("New discarder of type %s for field %s", eventType, field)

	if handlers, ok := allDiscarderHandlers[eventType]; ok {
		for _, handler := range handlers {
			discarderPushed, _ := handler(rs, event, p, Discarder{Field: field})

			if discarderPushed {
				p.notifyDiscarderPushedCallbacksLock.Lock()
				defer p.notifyDiscarderPushedCallbacksLock.Unlock()
				for _, cb := range p.notifyDiscarderPushedCallbacks {
					cb(eventType, event, field)
				}
			}
		}
	}

	return nil
}

// ApplyFilterPolicy is called when a passing policy for an event type is applied
func (p *Probe) ApplyFilterPolicy(eventType eval.EventType, mode PolicyMode, flags PolicyFlag) error {
	seclog.Infof("Setting in-kernel filter policy to `%s` for `%s`", mode, eventType)
	table, err := p.Map("filter_policy")
	if err != nil {
		return fmt.Errorf("unable to find policy table: %w", err)
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
		seclog.Errorf("Error while adding approvers fallback in-kernel policy to `%s` for `%s`: %s", PolicyModeAccept, eventType, err)
	}

	for _, newApprover := range newApprovers {
		seclog.Tracef("Applying approver %+v", newApprover)
		if err := newApprover.Apply(p); err != nil {
			return err
		}
	}

	if previousApprovers, exist := p.approvers[eventType]; exist {
		previousApprovers.Sub(newApprovers)
		for _, previousApprover := range previousApprovers {
			seclog.Tracef("Removing previous approver %+v", previousApprover)
			if err := previousApprover.Remove(p); err != nil {
				return err
			}
		}
	}

	p.approvers[eventType] = newApprovers
	return nil
}

func (p *Probe) selectTCProbes() manager.ProbesSelector {
	p.tcProgramsLock.RLock()
	defer p.tcProgramsLock.RUnlock()

	// Although unlikely, a race is still possible with the umount event of a network namespace:
	//   - a reload event is triggered
	//   - selectTCProbes is invoked and the list of currently running probes is generated
	//   - a container exits and the umount event of its network namespace is handled now (= its TC programs are stopped)
	//   - the manager executes UpdateActivatedProbes
	// In this setup, if we didn't use the best effort selector, the manager would try to init & attach a program that
	// was deleted when the container exited.
	var activatedProbes manager.BestEffort
	for _, tcProbe := range p.tcPrograms {
		if tcProbe.IsRunning() {
			activatedProbes.Selectors = append(activatedProbes.Selectors, &manager.ProbeSelector{
				ProbeIdentificationPair: tcProbe.ProbeIdentificationPair,
			})
		}
	}
	return &activatedProbes
}

func (p *Probe) isNeededForActivityDump(eventType eval.EventType) bool {
	if p.config.ActivityDumpEnabled {
		for _, e := range p.config.ActivityDumpTracedEventTypes {
			if e.String() == eventType {
				return true
			}
		}
	}
	return false
}

// SelectProbes applies the loaded set of rules and returns a report
// of the applied approvers for it.
func (p *Probe) SelectProbes(eventTypes []eval.EventType) error {
	var activatedProbes []manager.ProbesSelector

	for eventType, selectors := range probes.GetSelectorsPerEventType() {
		if eventType == "*" || slices.Contains(eventTypes, eventType) || p.isNeededForActivityDump(eventType) {
			activatedProbes = append(activatedProbes, selectors...)
		}
	}

	if p.config.NetworkEnabled {
		activatedProbes = append(activatedProbes, probes.NetworkSelectors...)

		// add probes depending on loaded modules
		loadedModules, err := utils.FetchLoadedModules()
		if err == nil {
			if _, ok := loadedModules["veth"]; ok {
				activatedProbes = append(activatedProbes, probes.NetworkVethSelectors...)
			}
			if _, ok := loadedModules["nf_nat"]; ok {
				activatedProbes = append(activatedProbes, probes.NetworkNFNatSelectors...)
			}
		}
	}

	activatedProbes = append(activatedProbes, p.selectTCProbes())

	// Add syscall monitor probes
	if p.config.ActivityDumpEnabled {
		for _, e := range p.config.ActivityDumpTracedEventTypes {
			if e == model.SyscallsEventType {
				activatedProbes = append(activatedProbes, probes.SyscallMonitorSelectors...)
				break
			}
		}
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
				seclog.Tracef("probe %s selected", id)
			}
		}
	}

	enabledEventsMap, err := p.Map("enabled_events")
	if err != nil {
		return err
	}

	enabledEvents := uint64(0)
	for _, eventName := range eventTypes {
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
	if err := p.eventStream.Pause(); err != nil {
		return err
	}
	defer func() {
		if err := p.eventStream.Resume(); err != nil {
			seclog.Errorf("failed to resume event stream: %s", err)
		}
	}()

	if err := enabledEventsMap.Put(ebpf.ZeroUint32MapItem, enabledEvents); err != nil {
		return fmt.Errorf("failed to set enabled events: %w", err)
	}

	return p.manager.UpdateActivatedProbes(activatedProbes)
}

// DumpDiscarders removes all the discarders
func (p *Probe) DumpDiscarders() (string, error) {
	seclog.Debugf("Dumping discarders")

	dump, err := os.CreateTemp("/tmp", "discarder-dump-")
	if err != nil {
		return "", err
	}
	defer dump.Close()

	if err := os.Chmod(dump.Name(), 0400); err != nil {
		return "", err
	}

	fmt.Fprintf(dump, "Discarder Dump\n%s\n", time.Now().UTC().String())

	fmt.Fprintf(dump, `
Legend:
Discarder Count: Discardee Info
Discarder Count: Discardee Parameters
`)

	fmt.Fprintf(dump, "\nInode Discarders\n")

	discardedInodeCount := 0
	inodeDiscardersInfo, inodeDiscardersErr := p.inodeDiscarders.Info()
	if inodeDiscardersErr != nil {
		seclog.Errorf("could not get info about inode discarders: %s", inodeDiscardersErr)
	} else {
		var inode inodeDiscarderMapEntry
		var inodeParams inodeDiscarderParams
		maxInodeDiscarders := int(inodeDiscardersInfo.MaxEntries)

		for entries := p.inodeDiscarders.Iterate(); entries.Next(&inode, &inodeParams); {
			discardedInodeCount++
			fields := model.FileFields{MountID: inode.PathKey.MountID, Inode: inode.PathKey.Inode, PathID: inode.PathKey.PathID}
			path, err := p.resolvers.resolveFileFieldsPath(&fields, &model.PIDContext{Pid: 1, Tid: 1})
			if err != nil {
				path = err.Error()
			}
			printDiscardee(dump, fmt.Sprintf("%s %+v", path, inode), fmt.Sprintf("%+v", inodeParams), discardedInodeCount)
			if discardedInodeCount == maxInodeDiscarders {
				seclog.Infof("Discarded inode count has reached max discarder map size")
				break
			}
		}
	}

	fmt.Fprintf(dump, "\nProcess Discarders\n")

	discardedPIDCount := 0
	pidDiscardersInfo, pidDiscardersErr := p.pidDiscarders.Info()
	if pidDiscardersErr != nil {
		seclog.Errorf("could not get info about PID discarders: %s", pidDiscardersErr)
	} else {
		var pid uint32
		var pidParams pidDiscarderParams
		maxPIDDiscarders := int(pidDiscardersInfo.MaxEntries)

		for entries := p.pidDiscarders.Iterate(); entries.Next(&pid, &pidParams); {
			discardedPIDCount++
			printDiscardee(dump, fmt.Sprintf("%+v", pid), fmt.Sprintf("%+v", pidParams), discardedPIDCount)
			if discardedPIDCount == maxPIDDiscarders {
				seclog.Infof("Discarded PID count has reached max discarder map size")
				break
			}
		}
	}

	fmt.Fprintf(dump, "\nDiscarder Stats - Front Buffer\n")
	frontBufferPrintErr := p.printDiscarderStats(dump, frontBufferDiscarderStatsMapName)
	if frontBufferPrintErr != nil {
		seclog.Errorf("could not dump discarder stats map %s: %s", frontBufferDiscarderStatsMapName, frontBufferPrintErr)
	}

	fmt.Fprintf(dump, "\nDiscarder Stats - Back Buffer\n")
	backBufferPrintErr := p.printDiscarderStats(dump, backBufferDiscarderStatsMapName)
	if backBufferPrintErr != nil {
		seclog.Errorf("could not dump discarder stats map %s: %s", backBufferDiscarderStatsMapName, backBufferPrintErr)
	}

	fmt.Fprintf(dump, "\nEnd Discarder Dump\n")

	seclog.Infof("%d inode discarders found, %d pid discarders found", discardedInodeCount, discardedPIDCount)
	return dump.Name(), nil
}

// FlushDiscarders removes all the discarders
func (p *Probe) FlushDiscarders() error {
	seclog.Debugf("Freezing discarders")

	flushingMap, err := p.Map("flushing_discarders")
	if err != nil {
		return err
	}

	if err := flushingMap.Put(ebpf.ZeroUint32MapItem, uint32(1)); err != nil {
		return fmt.Errorf("failed to set flush_discarders flag: %w", err)
	}

	unfreezeDiscarders := func() {
		p.flushingDiscarders.Store(false)

		if err := flushingMap.Put(ebpf.ZeroUint32MapItem, uint32(0)); err != nil {
			seclog.Errorf("Failed to reset flush_discarders flag: %s", err)
		}

		seclog.Debugf("Unfreezing discarders")
	}
	defer unfreezeDiscarders()

	if p.flushingDiscarders.Swap(true) {
		return errors.New("already flushing discarders")
	}
	// Sleeping a bit to avoid races with executing kprobes and setting discarders
	time.Sleep(time.Second)

	var discardedInodes []inodeDiscarderMapEntry
	var mapValue [256]byte

	var inode inodeDiscarderMapEntry
	for entries := p.inodeDiscarders.Iterate(); entries.Next(&inode, unsafe.Pointer(&mapValue[0])); {
		discardedInodes = append(discardedInodes, inode)
	}

	var discardedPids []uint32
	for pid, entries := uint32(0), p.pidDiscarders.Iterate(); entries.Next(&pid, unsafe.Pointer(&mapValue[0])); {
		discardedPids = append(discardedPids, pid)
	}

	discarderCount := len(discardedInodes) + len(discardedPids)
	if discarderCount == 0 {
		seclog.Debugf("No discarder found")
		return nil
	}

	flushWindow := time.Second * time.Duration(p.config.FlushDiscarderWindow)
	delay := flushWindow / time.Duration(discarderCount)

	flushDiscarders := func() {
		seclog.Debugf("Flushing discarders")

		var req ERPCRequest

		for _, inode := range discardedInodes {
			if err := p.inodeDiscarders.expireInodeDiscarder(&req, inode.PathKey.MountID, inode.PathKey.Inode); err != nil {
				seclog.Tracef("Failed to flush discarder for inode %d: %s", inode, err)
			}

			discarderCount--
			if discarderCount > 0 {
				time.Sleep(delay)
			}
		}

		for _, pid := range discardedPids {
			if err := p.pidDiscarders.expirePidDiscarder(&req, pid); err != nil {
				seclog.Tracef("Failed to flush discarder for inode %d: %s", inode, err)
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
	// Cancelling the context will stop the reorderer = we won't dequeue events anymore and new events from the
	// perf map reader are ignored
	p.cancelFnc()

	// we wait until both the reorderer and the monitor are stopped
	p.wg.Wait()

	// Stopping the manager will stop the perf map reader and unload eBPF programs
	if err := p.manager.Stop(manager.CleanAll); err != nil {
		return err
	}

	// when we reach this point, we do not generate nor consume events anymore, we can close the resolvers
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
func (p *Probe) NewRuleSet(opts *rules.Opts, evalOpts *eval.Opts, macroStore *eval.MacroStore) *rules.RuleSet {
	eventCtor := func() eval.Event {
		return NewEvent(p.resolvers, p.scrubber, p)
	}
	opts.WithLogger(seclog.DefaultLogger)

	return rules.NewRuleSet(&Model{probe: p}, eventCtor, opts, evalOpts, macroStore)
}

// QueuedNetworkDeviceError is used to indicate that the new network device was queued until its namespace handle is
// resolved.
type QueuedNetworkDeviceError struct {
	msg string
}

func (err QueuedNetworkDeviceError) Error() string {
	return err.msg
}

func (p *Probe) setupNewTCClassifier(device model.NetDevice) error {
	// select netns handle
	var handle *os.File
	var err error
	netns := p.resolvers.NamespaceResolver.ResolveNetworkNamespace(device.NetNS)
	if netns != nil {
		handle, err = netns.GetNamespaceHandleDup()
	}
	if netns == nil || err != nil || handle == nil {
		// queue network device so that a TC classifier can be added later
		p.resolvers.NamespaceResolver.QueueNetworkDevice(device)
		return QueuedNetworkDeviceError{msg: fmt.Sprintf("device %s is queued until %d is resolved", device.Name, device.NetNS)}
	}
	defer handle.Close()
	return p.setupNewTCClassifierWithNetNSHandle(device, handle)
}

// setupNewTCClassifierWithNetNSHandle creates and attaches TC probes on the provided device. WARNING: this function
// will not close the provided netns handle, so the caller of this function needs to take care of it.
func (p *Probe) setupNewTCClassifierWithNetNSHandle(device model.NetDevice, netnsHandle *os.File) error {
	p.tcProgramsLock.Lock()
	defer p.tcProgramsLock.Unlock()

	var combinedErr multierror.Error
	for _, tcProbe := range probes.GetTCProbes() {
		// make sure we're not overriding an existing network probe
		deviceKey := NetDeviceKey{IfIndex: device.IfIndex, NetNS: device.NetNS, NetworkDirection: tcProbe.NetworkDirection}
		_, ok := p.tcPrograms[deviceKey]
		if ok {
			continue
		}

		newProbe := tcProbe.Copy()
		newProbe.CopyProgram = true
		newProbe.UID = probes.SecurityAgentUID + device.GetKey()
		newProbe.IfIndex = int(device.IfIndex)
		newProbe.IfIndexNetns = uint64(netnsHandle.Fd())
		newProbe.IfIndexNetnsID = device.NetNS
		newProbe.KeepProgramSpec = false
		newProbe.TCFilterPrio = p.config.NetworkClassifierPriority
		newProbe.TCFilterHandle = netlink.MakeHandle(0, p.config.NetworkClassifierHandle)

		netnsEditor := []manager.ConstantEditor{
			{
				Name:  "netns",
				Value: uint64(device.NetNS),
			},
		}

		if err := p.manager.CloneProgram(probes.SecurityAgentUID, newProbe, netnsEditor, nil); err != nil {
			_ = multierror.Append(&combinedErr, fmt.Errorf("couldn't clone %s: %v", tcProbe.ProbeIdentificationPair, err))
		} else {
			p.tcPrograms[deviceKey] = newProbe
		}
	}
	return combinedErr.ErrorOrNil()
}

// flushNetworkNamespace thread unsafe version of FlushNetworkNamespace
func (p *Probe) flushNetworkNamespace(namespace *NetworkNamespace) {
	p.tcProgramsLock.Lock()
	defer p.tcProgramsLock.Unlock()
	for tcKey, tcProbe := range p.tcPrograms {
		if tcKey.NetNS == namespace.nsID {
			_ = p.manager.DetachHook(tcProbe.ProbeIdentificationPair)
			delete(p.tcPrograms, tcKey)
		}
	}
}

// FlushNetworkNamespace removes all references and stops all TC programs in the provided network namespace. This method
// flushes the network namespace in the network namespace resolver as well.
func (p *Probe) FlushNetworkNamespace(namespace *NetworkNamespace) {
	p.resolvers.NamespaceResolver.FlushNetworkNamespace(namespace)

	// cleanup internal structures
	p.flushNetworkNamespace(namespace)
}

// flushInactiveProbes detaches and deletes inactive probes. This function returns a map containing the count of interfaces
// per network namespace (ignoring the interfaces that are lazily deleted).
func (p *Probe) flushInactiveProbes() map[uint32]int {
	p.tcProgramsLock.Lock()
	defer p.tcProgramsLock.Unlock()

	probesCountNoLazyDeletion := make(map[uint32]int)

	var linkName string
	for tcKey, tcProbe := range p.tcPrograms {
		if !tcProbe.IsTCFilterActive() {
			_ = p.manager.DetachHook(tcProbe.ProbeIdentificationPair)
			delete(p.tcPrograms, tcKey)
		} else {
			link, err := tcProbe.ResolveLink()
			if err == nil {
				linkName = link.Attrs().Name
			} else {
				linkName = ""
			}
			// ignore interfaces that are lazily deleted
			if link.Attrs().HardwareAddr.String() != "" && !p.resolvers.NamespaceResolver.IsLazyDeletionInterface(linkName) {
				probesCountNoLazyDeletion[tcKey.NetNS]++
			}
		}
	}

	return probesCountNoLazyDeletion
}

// NewProbe instantiates a new runtime security agent probe
func NewProbe(config *config.Config, statsdClient statsd.ClientInterface) (*Probe, error) {
	erpc, err := NewERPC()
	if err != nil {
		return nil, err
	}

	ctx, cancel := context.WithCancel(context.Background())

	p := &Probe{
		config:               config,
		approvers:            make(map[eval.EventType]activeApprovers),
		managerOptions:       ebpf.NewDefaultOptions(),
		ctx:                  ctx,
		cancelFnc:            cancel,
		erpc:                 erpc,
		erpcRequest:          &ERPCRequest{},
		tcPrograms:           make(map[NetDeviceKey]*manager.Probe),
		statsdClient:         statsdClient,
		discarderRateLimiter: rate.NewLimiter(rate.Every(time.Second/5), 100),
		flushingDiscarders:   atomic.NewBool(false),
		isRuntimeDiscarded:   os.Getenv("RUNTIME_SECURITY_TESTSUITE") != "true",
	}

	if err := p.detectKernelVersion(); err != nil {
		// we need the kernel version to start, fail if we can't get it
		return nil, err
	}

	if err := p.sanityChecks(); err != nil {
		return nil, err
	}

	if err := p.VerifyOSVersion(); err != nil {
		seclog.Warnf("the current kernel isn't officially supported, some features might not work properly: %v", err)
	}

	if err := p.VerifyEnvironment(); err != nil {
		seclog.Warnf("the current environment may be misconfigured: %v", err)
	}

	useRingBuffers := p.UseRingBuffers()
	useMmapableMaps := p.kernelVersion.HaveMmapableMaps()

	p.manager = ebpf.NewRuntimeSecurityManager(useRingBuffers)

	p.ensureConfigDefaults()

	numCPU, err := utils.NumCPU()
	if err != nil {
		return nil, fmt.Errorf("failed to parse CPU count: %w", err)
	}
	p.managerOptions.MapSpecEditors = probes.AllMapSpecEditors(
		numCPU,
		p.config.ActivityDumpTracedCgroupsCount,
		useMmapableMaps,
		useRingBuffers,
		uint32(p.config.EventStreamBufferSize),
	)

	if !p.config.EnableKernelFilters {
		seclog.Warnf("Forcing in-kernel filter policy to `pass`: filtering not enabled")
	}

	if p.config.ActivityDumpEnabled {
		for _, e := range p.config.ActivityDumpTracedEventTypes {
			if e == model.SyscallsEventType {
				// Add syscall monitor probes
				p.managerOptions.ActivatedProbes = append(p.managerOptions.ActivatedProbes, probes.SyscallMonitorSelectors...)
				break
			}
		}
	}

	p.constantOffsets, err = p.GetOffsetConstants()
	if err != nil {
		seclog.Warnf("constant fetcher failed: %v", err)
		return nil, err
	}
	// the constant fetching mechanism can be quite memory intensive, between kernel header downloading,
	// runtime compilation, BTF parsing...
	// let's ensure the GC has run at this point before doing further memory intensive stuff
	runtime.GC()

	p.managerOptions.ConstantEditors = append(p.managerOptions.ConstantEditors, constantfetch.CreateConstantEditors(p.constantOffsets)...)

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
			Name:  "getattr2",
			Value: getAttr2(p),
		},
		manager.ConstantEditor{
			Name:  "vfs_unlink_dentry_position",
			Value: getVFSLinkDentryPosition(p),
		},
		manager.ConstantEditor{
			Name:  "vfs_mkdir_dentry_position",
			Value: getVFSMKDirDentryPosition(p),
		},
		manager.ConstantEditor{
			Name:  "vfs_link_target_dentry_position",
			Value: getVFSLinkTargetDentryPosition(p),
		},
		manager.ConstantEditor{
			Name:  "vfs_setxattr_dentry_position",
			Value: getVFSSetxattrDentryPosition(p),
		},
		manager.ConstantEditor{
			Name:  "vfs_removexattr_dentry_position",
			Value: getVFSRemovexattrDentryPosition(p),
		},
		manager.ConstantEditor{
			Name:  "vfs_rename_input_type",
			Value: getVFSRenameInputType(p),
		},
		manager.ConstantEditor{
			Name:  "check_helper_call_input",
			Value: getCheckHelperCallInputType(p),
		},
		manager.ConstantEditor{
			Name:  "cgroup_activity_dumps_enabled",
			Value: utils.BoolTouint64(config.ActivityDumpEnabled && areCGroupADsEnabled(config)),
		},
		manager.ConstantEditor{
			Name:  "net_struct_type",
			Value: getNetStructType(p.kernelVersion),
		},
		manager.ConstantEditor{
			Name:  "syscall_monitor_event_period",
			Value: uint64(p.config.ActivityDumpSyscallMonitorPeriod.Nanoseconds()),
		},
		manager.ConstantEditor{
			Name:  "setup_new_exec_is_last",
			Value: utils.BoolTouint64(!p.kernelVersion.IsRH7Kernel() && p.kernelVersion.Code >= kernel.Kernel5_5), // the setup_new_exec kprobe is after security_bprm_committed_creds in kernels that are not RH7, and additionally, have a kernel version of at least 5.5
		},
		manager.ConstantEditor{
			Name:  "tracepoint_module_load_sends_event",
			Value: utils.BoolTouint64(p.kernelVersion.IsRH7Kernel()), // the condition may need to be fine-tuned based on the kernel version
		},
	)

	p.managerOptions.ConstantEditors = append(p.managerOptions.ConstantEditors, DiscarderConstants...)
	p.managerOptions.ConstantEditors = append(p.managerOptions.ConstantEditors, getCGroupWriteConstants())

	// if we are using tracepoints to probe syscall exits, i.e. if we are using an old kernel version (< 4.12)
	// we need to use raw_syscall tracepoints for exits, as syscall are not trace when running an ia32 userspace
	// process
	if probes.ShouldUseSyscallExitTracepoints() {
		p.managerOptions.ConstantEditors = append(p.managerOptions.ConstantEditors,
			manager.ConstantEditor{
				Name:  "tracepoint_raw_syscall_fallback",
				Value: utils.BoolTouint64(true),
			},
		)
	}

	if useRingBuffers {
		p.managerOptions.ConstantEditors = append(p.managerOptions.ConstantEditors,
			manager.ConstantEditor{
				Name:  "use_ring_buffer",
				Value: utils.BoolTouint64(true),
			},
		)
	}

	// tail calls
	p.managerOptions.TailCallRouter = probes.AllTailRoutes(p.config.ERPCDentryResolutionEnabled, p.config.NetworkEnabled, useMmapableMaps)
	if !p.config.ERPCDentryResolutionEnabled || useMmapableMaps {
		// exclude the programs that use the bpf_probe_write_user helper
		p.managerOptions.ExcludedFunctions = probes.AllBPFProbeWriteUserProgramFunctions()
	}

	if !p.config.NetworkEnabled {
		// prevent all TC classifiers from loading
		p.managerOptions.ExcludedFunctions = append(p.managerOptions.ExcludedFunctions, probes.GetAllTCProgramFunctions()...)
	}

	resolvers, err := NewResolvers(config, p)
	if err != nil {
		return nil, err
	}
	p.resolvers = resolvers

	p.scrubber = pconfig.NewDefaultDataScrubber()
	p.scrubber.AddCustomSensitiveWords(config.CustomSensitiveWords)

	p.event = NewEvent(p.resolvers, p.scrubber, p)

	eventZero.resolvers = p.resolvers
	eventZero.scrubber = p.scrubber
	eventZero.probe = p

	if useRingBuffers {
		p.eventStream = NewRingBuffer(p.handleEvent)
	} else {
		p.eventStream, err = NewOrderedPerfMap(p.ctx, p.handleEvent, p.statsdClient)
		if err != nil {
			return nil, err
		}
	}

	return p, nil
}

func (p *Probe) ensureConfigDefaults() {
	// enable runtime compiled constants on COS by default
	if !p.config.RuntimeCompiledConstantsIsSet && p.kernelVersion.IsCOSKernel() {
		p.config.RuntimeCompiledConstantsEnabled = true
	}
}

// GetOffsetConstants returns the offsets and struct sizes constants
func (p *Probe) GetOffsetConstants() (map[string]uint64, error) {
	constantFetcher := constantfetch.ComposeConstantFetchers(constantfetch.GetAvailableConstantFetchers(p.config, p.kernelVersion, p.statsdClient))
	kv, err := p.GetKernelVersion()
	if err != nil {
		return nil, fmt.Errorf("failed to fetch probe kernel version: %w", err)
	}
	AppendProbeRequestsToFetcher(constantFetcher, kv)
	return constantFetcher.FinishAndGetResults()
}

// GetConstantFetcherStatus returns the status of the constant fetcher associated with this probe
func (p *Probe) GetConstantFetcherStatus() (*constantfetch.ConstantFetcherStatus, error) {
	constantFetcher := constantfetch.ComposeConstantFetchers(constantfetch.GetAvailableConstantFetchers(p.config, p.kernelVersion, p.statsdClient))
	kv, err := p.GetKernelVersion()
	if err != nil {
		return nil, fmt.Errorf("failed to fetch probe kernel version: %w", err)
	}
	AppendProbeRequestsToFetcher(constantFetcher, kv)
	return constantFetcher.FinishAndGetStatus()
}

// AppendProbeRequestsToFetcher returns the offsets and struct sizes constants, from a constant fetcher
func AppendProbeRequestsToFetcher(constantFetcher constantfetch.ConstantFetcher, kv *kernel.Version) {
	constantFetcher.AppendSizeofRequest(constantfetch.SizeOfInode, "struct inode", "linux/fs.h")
	constantFetcher.AppendOffsetofRequest(constantfetch.OffsetNameSuperBlockStructSMagic, "struct super_block", "s_magic", "linux/fs.h")
	constantFetcher.AppendOffsetofRequest(constantfetch.OffsetNameDentryStructDSB, "struct dentry", "d_sb", "linux/dcache.h")
	constantFetcher.AppendOffsetofRequest(constantfetch.OffsetNameSignalStructStructTTY, "struct signal_struct", "tty", "linux/sched/signal.h")
	constantFetcher.AppendOffsetofRequest(constantfetch.OffsetNameTTYStructStructName, "struct tty_struct", "name", "linux/tty.h")
	constantFetcher.AppendOffsetofRequest(constantfetch.OffsetNameCredStructUID, "struct cred", "uid", "linux/cred.h")

	// bpf offsets
	constantFetcher.AppendOffsetofRequest(constantfetch.OffsetNameBPFMapStructID, "struct bpf_map", "id", "linux/bpf.h")
	if kv.Code != 0 && (kv.Code >= kernel.Kernel4_15 || kv.IsRH7Kernel()) {
		constantFetcher.AppendOffsetofRequest(constantfetch.OffsetNameBPFMapStructName, "struct bpf_map", "name", "linux/bpf.h")
	}
	constantFetcher.AppendOffsetofRequest(constantfetch.OffsetNameBPFMapStructMapType, "struct bpf_map", "map_type", "linux/bpf.h")
	constantFetcher.AppendOffsetofRequest(constantfetch.OffsetNameBPFProgAuxStructID, "struct bpf_prog_aux", "id", "linux/bpf.h")
	if kv.Code != 0 && (kv.Code >= kernel.Kernel4_15 || kv.IsRH7Kernel()) {
		constantFetcher.AppendOffsetofRequest(constantfetch.OffsetNameBPFProgAuxStructName, "struct bpf_prog_aux", "name", "linux/bpf.h")
	}
	constantFetcher.AppendOffsetofRequest(constantfetch.OffsetNameBPFProgStructTag, "struct bpf_prog", "tag", "linux/filter.h")
	constantFetcher.AppendOffsetofRequest(constantfetch.OffsetNameBPFProgStructAux, "struct bpf_prog", "aux", "linux/filter.h")
	constantFetcher.AppendOffsetofRequest(constantfetch.OffsetNameBPFProgStructType, "struct bpf_prog", "type", "linux/filter.h")

	if kv.Code != 0 && (kv.Code > kernel.Kernel4_16 || kv.IsSuse12Kernel() || kv.IsSuse15Kernel()) {
		constantFetcher.AppendOffsetofRequest(constantfetch.OffsetNameBPFProgStructExpectedAttachType, "struct bpf_prog", "expected_attach_type", "linux/filter.h")
	}
	// namespace nr offsets
	constantFetcher.AppendOffsetofRequest(constantfetch.OffsetNamePIDStructLevel, "struct pid", "level", "linux/pid.h")
	constantFetcher.AppendOffsetofRequest(constantfetch.OffsetNamePIDStructNumbers, "struct pid", "numbers", "linux/pid.h")
	constantFetcher.AppendSizeofRequest(constantfetch.SizeOfUPID, "struct upid", "linux/pid.h")

	// splice event
	constantFetcher.AppendOffsetofRequest(constantfetch.OffsetNamePipeInodeInfoStructBufs, "struct pipe_inode_info", "bufs", "linux/pipe_fs_i.h")

	// network related constants
	constantFetcher.AppendOffsetofRequest(constantfetch.OffsetNameNetDeviceStructIfIndex, "struct net_device", "ifindex", "linux/netdevice.h")
	constantFetcher.AppendOffsetofRequest(constantfetch.OffsetNameSockCommonStructSKCNet, "struct sock_common", "skc_net", "net/sock.h")
	constantFetcher.AppendOffsetofRequest(constantfetch.OffsetNameSockCommonStructSKCFamily, "struct sock_common", "skc_family", "net/sock.h")
	constantFetcher.AppendOffsetofRequest(constantfetch.OffsetNameFlowI4StructSADDR, "struct flowi4", "saddr", "net/flow.h")
	constantFetcher.AppendOffsetofRequest(constantfetch.OffsetNameFlowI4StructULI, "struct flowi4", "uli", "net/flow.h")
	constantFetcher.AppendOffsetofRequest(constantfetch.OffsetNameFlowI6StructSADDR, "struct flowi6", "saddr", "net/flow.h")
	constantFetcher.AppendOffsetofRequest(constantfetch.OffsetNameFlowI6StructULI, "struct flowi6", "uli", "net/flow.h")
	constantFetcher.AppendOffsetofRequest(constantfetch.OffsetNameSocketStructSK, "struct socket", "sk", "linux/net.h")

	// Interpreter constants
	constantFetcher.AppendOffsetofRequest(constantfetch.OffsetNameLinuxBinprmStructFile, "struct linux_binprm", "file", "linux/binfmts.h")

	if !kv.IsRH7Kernel() {
		constantFetcher.AppendOffsetofRequest(constantfetch.OffsetNameNFConnStructCTNet, "struct nf_conn", "ct_net", "net/netfilter/nf_conntrack.h")
	}

	if getNetStructType(kv) == netStructHasProcINum {
		constantFetcher.AppendOffsetofRequest(constantfetch.OffsetNameNetStructProcInum, "struct net", "proc_inum", "net/net_namespace.h")
	} else {
		constantFetcher.AppendOffsetofRequest(constantfetch.OffsetNameNetStructNS, "struct net", "ns", "net/net_namespace.h")
	}

	// iouring
	if kv.Code != 0 && (kv.Code >= kernel.Kernel5_1) {
		constantFetcher.AppendOffsetofRequest(constantfetch.OffsetNameIoKiocbStructCtx, "struct io_kiocb", "ctx", "")
	}
}
