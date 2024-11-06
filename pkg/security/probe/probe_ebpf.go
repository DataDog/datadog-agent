// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

// Package probe holds probe related files
package probe

import (
	"context"
	"errors"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"sync"
	"time"

	lib "github.com/cilium/ebpf"
	"github.com/hashicorp/go-multierror"
	"github.com/moby/sys/mountinfo"
	"go.uber.org/atomic"
	"golang.org/x/sys/unix"
	"golang.org/x/time/rate"
	"gopkg.in/yaml.v3"

	"github.com/DataDog/datadog-go/v5/statsd"
	manager "github.com/DataDog/ebpf-manager"
	"github.com/DataDog/ebpf-manager/tracefs"

	"github.com/DataDog/datadog-agent/comp/core/telemetry"
	"github.com/DataDog/datadog-agent/pkg/config/env"
	ddebpf "github.com/DataDog/datadog-agent/pkg/ebpf"
	ebpftelemetry "github.com/DataDog/datadog-agent/pkg/ebpf/telemetry"
	"github.com/DataDog/datadog-agent/pkg/security/config"
	"github.com/DataDog/datadog-agent/pkg/security/ebpf"
	"github.com/DataDog/datadog-agent/pkg/security/ebpf/kernel"
	"github.com/DataDog/datadog-agent/pkg/security/ebpf/probes"
	"github.com/DataDog/datadog-agent/pkg/security/events"
	"github.com/DataDog/datadog-agent/pkg/security/metrics"
	pconfig "github.com/DataDog/datadog-agent/pkg/security/probe/config"
	"github.com/DataDog/datadog-agent/pkg/security/probe/constantfetch"
	"github.com/DataDog/datadog-agent/pkg/security/probe/erpc"
	"github.com/DataDog/datadog-agent/pkg/security/probe/eventstream"
	"github.com/DataDog/datadog-agent/pkg/security/probe/eventstream/reorderer"
	"github.com/DataDog/datadog-agent/pkg/security/probe/eventstream/ringbuffer"
	"github.com/DataDog/datadog-agent/pkg/security/probe/kfilters"
	"github.com/DataDog/datadog-agent/pkg/security/probe/managerhelper"
	"github.com/DataDog/datadog-agent/pkg/security/resolvers"
	"github.com/DataDog/datadog-agent/pkg/security/resolvers/mount"
	"github.com/DataDog/datadog-agent/pkg/security/resolvers/netns"
	"github.com/DataDog/datadog-agent/pkg/security/resolvers/path"
	"github.com/DataDog/datadog-agent/pkg/security/resolvers/process"
	"github.com/DataDog/datadog-agent/pkg/security/rules/bundled"
	"github.com/DataDog/datadog-agent/pkg/security/secl/compiler/eval"
	"github.com/DataDog/datadog-agent/pkg/security/secl/containerutils"
	"github.com/DataDog/datadog-agent/pkg/security/secl/model"
	"github.com/DataDog/datadog-agent/pkg/security/secl/rules"
	"github.com/DataDog/datadog-agent/pkg/security/seclog"
	"github.com/DataDog/datadog-agent/pkg/security/security_profile/dump"
	"github.com/DataDog/datadog-agent/pkg/security/serializers"
	"github.com/DataDog/datadog-agent/pkg/security/utils"
	utilkernel "github.com/DataDog/datadog-agent/pkg/util/kernel"
)

// EventStream describes the interface implemented by reordered perf maps or ring buffers
type EventStream interface {
	Init(*manager.Manager, *pconfig.Config) error
	SetMonitor(eventstream.LostEventCounter)
	Start(*sync.WaitGroup) error
	Pause() error
	Resume() error
}

const (
	// MaxOnDemandEventsPerSecond represents the maximum number of on demand events per second
	// allowed before we switch off the subsystem
	MaxOnDemandEventsPerSecond = 1_000
)

var (
	// defaultEventTypes event types used whatever the event handlers or the rules
	defaultEventTypes = []eval.EventType{
		model.ForkEventType.String(),
		model.ExecEventType.String(),
		model.ExitEventType.String(),
	}
)

var _ PlatformProbe = (*EBPFProbe)(nil)

// EBPFProbe defines a platform probe
type EBPFProbe struct {
	Resolvers *resolvers.EBPFResolvers

	// Constants and configuration
	opts         Opts
	config       *config.Config
	statsdClient statsd.ClientInterface

	probe          *Probe
	Manager        *manager.Manager
	managerOptions manager.Options
	kernelVersion  *kernel.Version

	// internals
	event           *model.Event
	monitors        *EBPFMonitors
	profileManagers *SecurityProfileManagers
	fieldHandlers   *EBPFFieldHandlers

	ctx       context.Context
	cancelFnc context.CancelFunc
	wg        sync.WaitGroup

	// TC Classifier
	newTCNetDevices chan model.NetDevice

	// Ring
	eventStream EventStream

	// ActivityDumps section
	activityDumpHandler dump.ActivityDumpHandler

	// Approvers / discarders section
	Erpc                     *erpc.ERPC
	erpcRequest              *erpc.Request
	inodeDiscarders          *inodeDiscarders
	discarderPushedCallbacks []DiscarderPushedCallback
	kfilters                 map[eval.EventType]kfilters.ActiveKFilters

	// Approvers / discarders section
	discarderPushedCallbacksLock sync.RWMutex
	discarderRateLimiter         *rate.Limiter

	// kill action
	killListMap           *lib.Map
	supportsBPFSendSignal bool
	processKiller         *ProcessKiller

	isRuntimeDiscarded bool
	constantOffsets    map[string]uint64
	runtimeCompiled    bool
	useSyscallWrapper  bool
	useFentry          bool

	// On demand
	onDemandManager     *OnDemandProbesManager
	onDemandRateLimiter *rate.Limiter

	// hash action
	fileHasher *FileHasher

	// snapshot
	ruleSetVersion    uint64
	playSnapShotState *atomic.Bool
}

// GetProfileManager returns the Profile Managers
func (p *EBPFProbe) GetProfileManager() interface{} {
	return p.profileManagers
}

func (p *EBPFProbe) detectKernelVersion() error {
	kernelVersion, err := kernel.NewKernelVersion()
	if err != nil {
		return fmt.Errorf("unable to detect the kernel version: %w", err)
	}
	p.kernelVersion = kernelVersion
	return nil
}

// GetKernelVersion computes and returns the running kernel version
func (p *EBPFProbe) GetKernelVersion() *kernel.Version {
	return p.kernelVersion
}

// UseRingBuffers returns true if eBPF ring buffers are supported and used
func (p *EBPFProbe) UseRingBuffers() bool {
	return p.config.Probe.EventStreamUseRingBuffer && p.kernelVersion.HaveRingBuffers()
}

func (p *EBPFProbe) selectFentryMode() {
	if !p.config.Probe.EventStreamUseFentry {
		p.useFentry = false
		return
	}

	supported := p.kernelVersion.HaveFentrySupport()
	if !supported {
		seclog.Errorf("fentry enabled but not supported, falling back to kprobe mode")
	}
	p.useFentry = supported
}

func (p *EBPFProbe) sanityChecks() error {
	// make sure debugfs is mounted
	if _, err := tracefs.Root(); err != nil {
		return err
	}

	if utilkernel.GetLockdownMode() == utilkernel.Confidentiality {
		return errors.New("eBPF not supported in lockdown `confidentiality` mode")
	}

	if p.config.Probe.NetworkEnabled && p.kernelVersion.IsRH7Kernel() {
		seclog.Warnf("The network feature of CWS isn't supported on Centos7, setting event_monitoring_config.network.enabled to false")
		p.config.Probe.NetworkEnabled = false
	}

	return nil
}

// NewModel returns a new Model
func (p *EBPFProbe) NewModel() *model.Model {
	return NewEBPFModel(p)
}

// VerifyOSVersion returns an error if the current kernel version is not supported
func (p *EBPFProbe) VerifyOSVersion() error {
	if !p.kernelVersion.IsRH7Kernel() && !p.kernelVersion.IsRH8Kernel() && p.kernelVersion.Code < kernel.Kernel4_15 {
		return fmt.Errorf("the following kernel is not supported: %s", p.kernelVersion)
	}
	return nil
}

// VerifyEnvironment returns an error if the current environment seems to be misconfigured
func (p *EBPFProbe) VerifyEnvironment() *multierror.Error {
	var err *multierror.Error
	if env.IsContainerized() {
		if mounted, _ := mountinfo.Mounted("/etc/passwd"); !mounted {
			err = multierror.Append(err, errors.New("/etc/passwd doesn't seem to be a mountpoint"))
		}

		if mounted, _ := mountinfo.Mounted("/etc/group"); !mounted {
			err = multierror.Append(err, errors.New("/etc/group doesn't seem to be a mountpoint"))
		}

		if mounted, _ := mountinfo.Mounted(utilkernel.ProcFSRoot()); !mounted {
			err = multierror.Append(err, errors.New("/etc/group doesn't seem to be a mountpoint"))
		}

		if mounted, _ := mountinfo.Mounted(p.kernelVersion.OsReleasePath); !mounted {
			err = multierror.Append(err, fmt.Errorf("%s doesn't seem to be a mountpoint", p.kernelVersion.OsReleasePath))
		}

		securityFSPath := filepath.Join(utilkernel.SysFSRoot(), "kernel/security")
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

// Init initializes the probe
func (p *EBPFProbe) Init() error {
	useSyscallWrapper, err := ebpf.IsSyscallWrapperRequired()
	if err != nil {
		return err
	}
	p.useSyscallWrapper = useSyscallWrapper

	loader := ebpf.NewProbeLoader(p.config.Probe, p.useSyscallWrapper, p.UseRingBuffers(), p.useFentry, p.statsdClient)
	defer loader.Close()

	bytecodeReader, runtimeCompiled, err := loader.Load()
	if err != nil {
		return err
	}
	defer bytecodeReader.Close()

	p.runtimeCompiled = runtimeCompiled

	if err := p.eventStream.Init(p.Manager, p.config.Probe); err != nil {
		return err
	}

	if p.isRuntimeDiscarded {
		p.managerOptions.ConstantEditors = append(p.managerOptions.ConstantEditors, manager.ConstantEditor{
			Name:  "runtime_discarded",
			Value: uint64(1),
		})
	}

	p.managerOptions.ActivatedProbes = append(p.managerOptions.ActivatedProbes, probes.SnapshotSelectors()...)

	if err := p.Manager.InitWithOptions(bytecodeReader, p.managerOptions); err != nil {
		return fmt.Errorf("failed to init manager: %w", err)
	}

	p.inodeDiscarders = newInodeDiscarders(p.Erpc, p.Resolvers.DentryResolver)

	if err := p.Resolvers.Start(p.ctx); err != nil {
		return err
	}

	err = p.monitors.Init()
	if err != nil {
		return err
	}

	p.profileManagers, err = NewSecurityProfileManagers(p)
	if err != nil {
		return err
	}
	p.profileManagers.AddActivityDumpHandler(p.activityDumpHandler)

	p.eventStream.SetMonitor(p.monitors.eventStreamMonitor)

	p.killListMap, err = managerhelper.Map(p.Manager, "kill_list")
	if err != nil {
		return err
	}

	p.processKiller.Start(p.ctx, &p.wg)

	return nil
}

// IsRuntimeCompiled returns true if the eBPF programs where successfully runtime compiled
func (p *EBPFProbe) IsRuntimeCompiled() bool {
	return p.runtimeCompiled
}

func (p *EBPFProbe) setupRawPacketProgs() error {
	packetsMap, _, err := p.Manager.GetMap("packets")
	if err != nil {
		return err
	}
	routerMap, _, err := p.Manager.GetMap("classifier_router")
	if err != nil {
		return err
	}

	progSpec, err := probes.GetRawPacketTCFilterProg(packetsMap.FD(), routerMap.FD())
	if err != nil {
		return err
	}

	colSpec := lib.CollectionSpec{
		Programs: map[string]*lib.ProgramSpec{
			progSpec.Name: progSpec,
		},
	}

	col, err := lib.NewCollection(&colSpec)
	if err != nil {
		return fmt.Errorf("failed to load program: %w", err)
	}

	return p.Manager.UpdateTailCallRoutes(
		manager.TailCallRoute{
			Program:       col.Programs[progSpec.Name],
			Key:           probes.TCRawPacketFilterKey,
			ProgArrayName: "classifier_router",
		},
	)
}

// Setup the probe
func (p *EBPFProbe) Setup() error {
	if err := p.Manager.Start(); err != nil {
		return err
	}
	ddebpf.AddNameMappings(p.Manager, "cws")

	p.applyDefaultFilterPolicies()

	needRawSyscalls := p.isNeededForActivityDump(model.SyscallsEventType.String())

	if err := p.updateProbes(defaultEventTypes, needRawSyscalls); err != nil {
		return err
	}

	p.profileManagers.Start(p.ctx, &p.wg)

	if p.probe.IsNetworkRawPacketEnabled() {
		if err := p.setupRawPacketProgs(); err != nil {
			return err
		}
	}

	return nil
}

// Start the probe
func (p *EBPFProbe) Start() error {
	// Apply rules to the snapshotted data before starting the event stream to avoid concurrency issues
	p.playSnapshot(true)

	// start new tc classifier loop
	go p.startSetupNewTCClassifierLoop()

	return p.eventStream.Start(&p.wg)
}

// PlaySnapshot plays a snapshot
func (p *EBPFProbe) playSnapshot(notifyConsumers bool) {
	seclog.Debugf("playing the snapshot")
	// Get the snapshotted data
	var events []*model.Event

	entryToEvent := func(entry *model.ProcessCacheEntry) {
		if entry.Source != model.ProcessCacheEntryFromSnapshot && !entry.IsExec {
			return
		}
		entry.Retain()
		event := NewEBPFEvent(p.fieldHandlers)
		event.Type = uint32(model.ExecEventType)
		event.TimestampRaw = uint64(time.Now().UnixNano())
		event.ProcessCacheEntry = entry
		event.ProcessContext = &entry.ProcessContext
		event.Exec.Process = &entry.Process
		event.ProcessContext.Process.ContainerID = entry.ContainerID
		event.ProcessContext.Process.CGroup = entry.CGroup

		if _, err := entry.HasValidLineage(); err != nil {
			event.Error = &model.ErrProcessBrokenLineage{Err: err}
		}

		events = append(events, event)
	}
	p.Resolvers.ProcessResolver.Walk(entryToEvent)
	for _, event := range events {
		p.DispatchEvent(event, notifyConsumers)
		event.ProcessCacheEntry.Release()
	}
}

func (p *EBPFProbe) sendAnomalyDetection(event *model.Event) {
	tags := p.probe.GetEventTags(string(event.ContainerContext.ContainerID))
	if service := p.probe.GetService(event); service != "" {
		tags = append(tags, "service:"+service)
	}

	p.probe.DispatchCustomEvent(
		events.NewCustomRule(events.AnomalyDetectionRuleID, events.AnomalyDetectionRuleDesc),
		events.NewCustomEventLazy(event.GetEventType(), p.EventMarshallerCtor(event), tags...),
	)
}

// AddActivityDumpHandler set the probe activity dump handler
func (p *EBPFProbe) AddActivityDumpHandler(handler dump.ActivityDumpHandler) {
	p.activityDumpHandler = handler
}

// DispatchEvent sends an event to the probe event handler
func (p *EBPFProbe) DispatchEvent(event *model.Event, notifyConsumers bool) {
	traceEvent("Dispatching event %s", func() ([]byte, model.EventType, error) {
		eventJSON, err := serializers.MarshalEvent(event, nil)
		return eventJSON, event.GetEventType(), err
	})

	// filter out event if already present on a profile
	if p.config.RuntimeSecurity.SecurityProfileEnabled {
		p.profileManagers.securityProfileManager.LookupEventInProfiles(event)
	}

	// mark the events that have an associated activity dump
	// this is needed for auto suppressions performed by the CWS rule engine
	if p.profileManagers.activityDumpManager != nil {
		if p.profileManagers.activityDumpManager.HasActiveActivityDump(event) {
			event.AddToFlags(model.EventFlagsHasActiveActivityDump)
		}
	}

	// send event to wildcard handlers, like the CWS rule engine, first
	p.probe.sendEventToHandlers(event)

	// send event to specific event handlers, like the event monitor consumers, subsequently
	if notifyConsumers {
		p.probe.sendEventToConsumers(event)
	}

	// handle anomaly detections
	if event.IsAnomalyDetectionEvent() {
		imageTag := utils.GetTagValue("image_tag", event.ContainerContext.Tags)
		p.profileManagers.securityProfileManager.FillProfileContextFromContainerID(event.FieldHandlers.ResolveContainerID(event, event.ContainerContext), &event.SecurityProfileContext, imageTag)
		if p.config.RuntimeSecurity.AnomalyDetectionEnabled {
			p.sendAnomalyDetection(event)
		}
	} else if event.Error == nil {
		// Process event after evaluation because some monitors need the DentryResolver to have been called first.
		if p.profileManagers.activityDumpManager != nil {
			p.profileManagers.activityDumpManager.ProcessEvent(event)
		}
	}
	p.monitors.ProcessEvent(event)
}

// SendStats sends statistics about the probe to Datadog
func (p *EBPFProbe) SendStats() error {
	p.Resolvers.TCResolver.SendTCProgramsStats(p.statsdClient)

	p.processKiller.SendStats(p.statsdClient)

	if err := p.profileManagers.SendStats(); err != nil {
		return err
	}

	return p.monitors.SendStats()
}

// GetMonitors returns the monitor of the probe
func (p *EBPFProbe) GetMonitors() *EBPFMonitors {
	return p.monitors
}

// EventMarshallerCtor returns the event marshaller ctor
func (p *EBPFProbe) EventMarshallerCtor(event *model.Event) func() events.EventMarshaler {
	return func() events.EventMarshaler {
		return serializers.NewEventSerializer(event, nil)
	}
}

func (p *EBPFProbe) unmarshalContexts(data []byte, event *model.Event) (int, error) {
	read, err := model.UnmarshalBinary(data, &event.PIDContext, &event.SpanContext, event.ContainerContext, &event.CGroupContext)
	if err != nil {
		return 0, err
	}

	// TODO(lebauce): fix this
	event.CGroupContext.CGroupID, event.ContainerContext.ContainerID = containerutils.GetCGroupContext(event.ContainerContext.ContainerID, event.CGroupContext.CGroupFlags)

	return read, nil
}

func eventWithNoProcessContext(eventType model.EventType) bool {
	return eventType == model.DNSEventType || eventType == model.IMDSEventType || eventType == model.RawPacketEventType || eventType == model.LoadModuleEventType || eventType == model.UnloadModuleEventType
}

func (p *EBPFProbe) unmarshalProcessCacheEntry(ev *model.Event, data []byte) (int, error) {
	var sc model.SyscallContext

	n, err := sc.UnmarshalBinary(data)
	if err != nil {
		return n, err
	}

	// don't provide a syscall context for Fork event for now
	if ev.BaseEvent.Type == uint32(model.ExecEventType) {
		ev.Exec.SyscallContext.ID = sc.ID
	}

	entry := p.Resolvers.ProcessResolver.NewProcessCacheEntry(ev.PIDContext)
	ev.ProcessCacheEntry = entry

	n, err = entry.Process.UnmarshalBinary(data[n:])
	if err != nil {
		return n, err
	}

	entry.Process.CGroup.CGroupID, entry.Process.ContainerID = containerutils.GetCGroupContext(ev.ContainerContext.ContainerID, ev.CGroupContext.CGroupFlags)
	entry.Process.CGroup.CGroupFlags = ev.CGroupContext.CGroupFlags
	entry.Process.CGroup.CGroupFile = ev.CGroupContext.CGroupFile
	entry.Source = model.ProcessCacheEntryFromEvent

	return n, nil
}

func (p *EBPFProbe) onEventLost(perfMapName string, perEvent map[string]uint64) {
	if p.config.RuntimeSecurity.InternalMonitoringEnabled {
		p.probe.DispatchCustomEvent(
			NewEventLostWriteEvent(perfMapName, perEvent),
		)
	}

	// snapshot traced cgroups if a CgroupTracing event was lost
	if p.probe.IsActivityDumpEnabled() && perEvent[model.CgroupTracingEventType.String()] > 0 {
		p.profileManagers.SnapshotTracedCgroups()
	}
}

// setProcessContext set the process context, should return false if the event shouldn't be dispatched
func (p *EBPFProbe) setProcessContext(eventType model.EventType, event *model.Event) bool {
	entry, isResolved := p.fieldHandlers.ResolveProcessCacheEntry(event)
	event.ProcessCacheEntry = entry
	if event.ProcessCacheEntry == nil {
		panic("should always return a process cache entry")
	}

	// use ProcessCacheEntry process context as process context
	event.ProcessContext = &event.ProcessCacheEntry.ProcessContext
	if event.ProcessContext == nil {
		panic("should always return a process context")
	}

	if process.IsKThread(event.ProcessContext.PPid, event.ProcessContext.Pid) {
		return false
	}

	if !eventWithNoProcessContext(eventType) {
		if !isResolved {
			event.Error = model.ErrNoProcessContext
		} else if _, err := entry.HasValidLineage(); err != nil {
			event.Error = &model.ErrProcessBrokenLineage{Err: err}
			p.Resolvers.ProcessResolver.CountBrokenLineage()
		}
	}

	// flush exited process
	p.Resolvers.ProcessResolver.DequeueExited()

	return true
}

func (p *EBPFProbe) zeroEvent() *model.Event {
	p.event.Zero()
	p.event.FieldHandlers = p.fieldHandlers
	p.event.Origin = EBPFOrigin
	return p.event
}

func (p *EBPFProbe) handleEvent(CPU int, data []byte) {
	// handle play snapshot
	if p.playSnapShotState.Swap(false) {
		// do not notify consumers as we are replaying the snapshot after a ruleset reload
		p.playSnapshot(false)
	}

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
	if eventType > model.MaxKernelEventType {
		p.monitors.eventStreamMonitor.CountInvalidEvent(eventstream.EventStreamMap, eventstream.InvalidType, dataLen)
		seclog.Errorf("unsupported event type %d", eventType)
		return
	}

	p.monitors.eventStreamMonitor.CountEvent(eventType, event.TimestampRaw, 1, dataLen, eventstream.EventStreamMap, CPU)

	// no need to dispatch events
	switch eventType {
	case model.MountReleasedEventType:
		if _, err = event.MountReleased.UnmarshalBinary(data[offset:]); err != nil {
			seclog.Errorf("failed to decode mount released event: %s (offset %d, len %d)", err, offset, dataLen)
			return
		}

		// Remove all dentry entries belonging to the mountID
		p.Resolvers.DentryResolver.DelCacheEntries(event.MountReleased.MountID)

		// Delete new mount point from cache
		if err = p.Resolvers.MountResolver.Delete(event.MountReleased.MountID); err != nil {
			seclog.Tracef("failed to delete mount point %d from cache: %s", event.MountReleased.MountID, err)
		}
		return
	case model.ArgsEnvsEventType:
		if _, err = event.ArgsEnvs.UnmarshalBinary(data[offset:]); err != nil {
			seclog.Errorf("failed to decode args envs event: %s (offset %d, len %d)", err, offset, dataLen)
			return
		}

		p.Resolvers.ProcessResolver.UpdateArgsEnvs(&event.ArgsEnvs)

		return
	case model.CgroupTracingEventType:
		if !p.config.RuntimeSecurity.ActivityDumpEnabled {
			seclog.Errorf("shouldn't receive Cgroup event if activity dumps are disabled")
			return
		}

		if _, err = event.CgroupTracing.UnmarshalBinary(data[offset:]); err != nil {
			seclog.Errorf("failed to decode cgroup tracing event: %s (offset %d, len %d)", err, offset, dataLen)
			return
		}

		p.profileManagers.activityDumpManager.HandleCGroupTracingEvent(&event.CgroupTracing)
		return
	case model.CgroupWriteEventType:
		if _, err = event.CgroupWrite.UnmarshalBinary(data[offset:]); err != nil {
			seclog.Errorf("failed to decode cgroup write released event: %s (offset %d, len %d)", err, offset, dataLen)
			return
		}
		pce := p.Resolvers.ProcessResolver.Resolve(event.CgroupWrite.Pid, event.CgroupWrite.Pid, 0, false)
		if pce != nil {
			path, err := p.Resolvers.DentryResolver.Resolve(event.CgroupWrite.File.PathKey, true)
			if err == nil && path != "" {
				path = filepath.Dir(string(path))
				pce.CGroup.CGroupID = containerutils.CGroupID(path)
				pce.Process.CGroup.CGroupID = containerutils.CGroupID(path)
				cgroupFlags := containerutils.CGroupFlags(event.CgroupWrite.CGroupFlags)
				if cgroupFlags.IsContainer() {
					containerID, _ := containerutils.GetContainerFromCgroup(path)
					pce.ContainerID = containerutils.ContainerID(containerID)
					pce.Process.ContainerID = containerutils.ContainerID(containerID)
				}
				pce.CGroup.CGroupFlags = cgroupFlags
				pce.Process.CGroup = pce.CGroup
			} else {
				seclog.Debugf("failed to resolve cgroup file %v", event.CgroupWrite.File)
			}
		} else {
			seclog.Debugf("failed to resolve process of cgroup write event: %s", err)
		}
		return
	case model.UnshareMountNsEventType:
		if _, err = event.UnshareMountNS.UnmarshalBinary(data[offset:]); err != nil {
			seclog.Errorf("failed to decode unshare mnt ns event: %s (offset %d, len %d)", err, offset, dataLen)
			return
		}
		if err := p.handleNewMount(event, &event.UnshareMountNS.Mount); err != nil {
			seclog.Debugf("failed to handle new mount from unshare mnt ns event: %s", err)
		}
		return
	}

	read, err = p.unmarshalContexts(data[offset:], event)
	if err != nil {
		seclog.Errorf("failed to decode event `%s`: %s", eventType, err)
		return
	}
	offset += read

	// save netns handle if applicable
	_, _ = p.Resolvers.NamespaceResolver.SaveNetworkNamespaceHandleLazy(event.PIDContext.NetNS, func() *utils.NetNSPath {
		return utils.NetNSPathFromPid(event.PIDContext.Pid)
	})

	// handle exec and fork before process context resolution as they modify the process context resolution
	switch eventType {
	case model.ForkEventType:
		if _, err = p.unmarshalProcessCacheEntry(event, data[offset:]); err != nil {
			seclog.Errorf("failed to decode fork event: %s (offset %d, len %d)", err, offset, dataLen)
			return
		}

		if process.IsKThread(event.ProcessCacheEntry.PPid, event.ProcessCacheEntry.Pid) {
			return
		}

		p.Resolvers.ProcessResolver.ApplyBootTime(event.ProcessCacheEntry)
		event.ProcessCacheEntry.SetSpan(event.SpanContext.SpanID, event.SpanContext.TraceID)

		p.Resolvers.ProcessResolver.AddForkEntry(event.ProcessCacheEntry, event.PIDContext.ExecInode)
	case model.ExecEventType:
		// unmarshal and fill event.processCacheEntry
		if _, err = p.unmarshalProcessCacheEntry(event, data[offset:]); err != nil {
			seclog.Errorf("failed to decode exec event: %s (offset %d, len %d)", err, offset, len(data))
			return
		}

		if err = p.Resolvers.ProcessResolver.ResolveNewProcessCacheEntry(event.ProcessCacheEntry, event.ContainerContext); err != nil {
			seclog.Debugf("failed to resolve new process cache entry context for pid %d: %s", event.PIDContext.Pid, err)

			var errResolution *path.ErrPathResolution
			if errors.As(err, &errResolution) {
				event.SetPathResolutionError(&event.ProcessCacheEntry.FileEvent, err)
			}
		} else {
			p.Resolvers.ProcessResolver.AddExecEntry(event.ProcessCacheEntry, event.PIDContext.ExecInode)
		}

		event.Exec.Process = &event.ProcessCacheEntry.Process
	}

	if !p.setProcessContext(eventType, event) {
		return
	}

	switch eventType {

	case model.FileMountEventType:
		if _, err = event.Mount.UnmarshalBinary(data[offset:]); err != nil {
			seclog.Errorf("failed to decode mount event: %s (offset %d, len %d)", err, offset, dataLen)
			return
		}
		if err := p.handleNewMount(event, &event.Mount.Mount); err != nil {
			seclog.Debugf("failed to handle new mount from mount event: %s\n", err)
			return
		}

		// TODO: this should be moved in the resolver itself in order to handle the fallbacks
		if event.Mount.GetFSType() == "nsfs" {
			nsid := uint32(event.Mount.RootPathKey.Inode)
			mountPath, _, _, err := p.Resolvers.MountResolver.ResolveMountPath(event.Mount.MountID, event.Mount.Device, event.PIDContext.Pid, string(event.ContainerContext.ContainerID))
			if err != nil {
				seclog.Debugf("failed to get mount path: %v", err)
			} else {
				mountNetNSPath := utils.NetNSPathFromPath(mountPath)
				_, _ = p.Resolvers.NamespaceResolver.SaveNetworkNamespaceHandle(nsid, mountNetNSPath)
			}
		}

	case model.FileUmountEventType:
		if _, err = event.Umount.UnmarshalBinary(data[offset:]); err != nil {
			seclog.Errorf("failed to decode umount event: %s (offset %d, len %d)", err, offset, dataLen)
			return
		}

		// we can skip this error as this is for the umount only and there is no impact on the filepath resolution
		mount, _, _, _ := p.Resolvers.MountResolver.ResolveMount(event.Umount.MountID, 0, event.PIDContext.Pid, string(event.ContainerContext.ContainerID))
		if mount != nil && mount.GetFSType() == "nsfs" {
			nsid := uint32(mount.RootPathKey.Inode)
			if namespace := p.Resolvers.NamespaceResolver.ResolveNetworkNamespace(nsid); namespace != nil {
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
	case model.FileUnlinkEventType:
		if _, err = event.Unlink.UnmarshalBinary(data[offset:]); err != nil {
			seclog.Errorf("failed to decode unlink event: %s (offset %d, len %d)", err, offset, dataLen)
			return
		}
	case model.FileRenameEventType:
		if _, err = event.Rename.UnmarshalBinary(data[offset:]); err != nil {
			seclog.Errorf("failed to decode rename event: %s (offset %d, len %d)", err, offset, dataLen)
			return
		}
	case model.FileChdirEventType:
		if _, err = event.Chdir.UnmarshalBinary(data[offset:]); err != nil {
			seclog.Errorf("failed to decode chdir event: %s (offset %d, len %d)", err, offset, dataLen)
			return
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
	case model.ExitEventType:
		if _, err = event.Exit.UnmarshalBinary(data[offset:]); err != nil {
			seclog.Errorf("failed to decode exit event: %s (offset %d, len %d)", err, offset, len(data))
			return
		}

		var exists bool
		event.ProcessCacheEntry, exists = p.fieldHandlers.GetProcessCacheEntry(event)
		if !exists {
			// no need to dispatch an exit event that don't have the corresponding cache entry
			return
		}

		// Use the event timestamp as exit time
		// The local process cache hasn't been updated yet with the exit time when the exit event is first seen
		// The pid_cache kernel map has the exit_time but it's only accessed if there's a local miss
		event.ProcessCacheEntry.Process.ExitTime = p.fieldHandlers.ResolveEventTime(event, &event.BaseEvent)
		event.Exit.Process = &event.ProcessCacheEntry.Process

		// update mount pid mapping
		p.Resolvers.MountResolver.DelPid(event.Exit.Pid)

		// update action reports
		p.processKiller.HandleProcessExited(event)
		p.fileHasher.HandleProcessExited(event)
	case model.SetuidEventType:
		// the process context may be incorrect, do not modify it
		if event.Error != nil {
			break
		}

		if _, err = event.SetUID.UnmarshalBinary(data[offset:]); err != nil {
			seclog.Errorf("failed to decode setuid event: %s (offset %d, len %d)", err, offset, len(data))
			return
		}
		defer p.Resolvers.ProcessResolver.UpdateUID(event.PIDContext.Pid, event)
	case model.SetgidEventType:
		// the process context may be incorrect, do not modify it
		if event.Error != nil {
			break
		}

		if _, err = event.SetGID.UnmarshalBinary(data[offset:]); err != nil {
			seclog.Errorf("failed to decode setgid event: %s (offset %d, len %d)", err, offset, len(data))
			return
		}
		defer p.Resolvers.ProcessResolver.UpdateGID(event.PIDContext.Pid, event)
	case model.CapsetEventType:
		// the process context may be incorrect, do not modify it
		if event.Error != nil {
			break
		}

		if _, err = event.Capset.UnmarshalBinary(data[offset:]); err != nil {
			seclog.Errorf("failed to decode capset event: %s (offset %d, len %d)", err, offset, len(data))
			return
		}
		defer p.Resolvers.ProcessResolver.UpdateCapset(event.PIDContext.Pid, event)
	case model.LoginUIDWriteEventType:
		if event.Error != nil {
			break
		}

		if _, err = event.LoginUIDWrite.UnmarshalBinary(data[offset:]); err != nil {
			seclog.Errorf("failed to decode login_uid_write event: %s (offset %d, len %d)", err, offset, len(data))
			return
		}
		defer p.Resolvers.ProcessResolver.UpdateLoginUID(event.PIDContext.Pid, event)
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
		var pce *model.ProcessCacheEntry
		if event.PTrace.PID == 0 { // pid can be 0 for a PTRACE_TRACEME request
			pce = newPlaceholderProcessCacheEntryPTraceMe()
		} else {
			pce = p.Resolvers.ProcessResolver.Resolve(event.PTrace.PID, event.PTrace.PID, 0, false)
			if pce == nil {
				pce = model.NewPlaceholderProcessCacheEntry(event.PTrace.PID, event.PTrace.PID, false)
			}
		}
		event.PTrace.Tracee = &pce.ProcessContext
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
		var pce *model.ProcessCacheEntry
		if event.Signal.PID > 0 { // Linux accepts a kill syscall with both negative and zero pid
			pce = p.Resolvers.ProcessResolver.Resolve(event.Signal.PID, event.Signal.PID, 0, false)
		}
		if pce == nil {
			pce = model.NewPlaceholderProcessCacheEntry(event.Signal.PID, event.Signal.PID, false)
		}
		event.Signal.Target = &pce.ProcessContext
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
		p.pushNewTCClassifierRequest(event.NetDevice.Device)
	case model.VethPairEventType:
		if _, err = event.VethPair.UnmarshalBinary(data[offset:]); err != nil {
			seclog.Errorf("failed to decode veth_pair event: %s (offset %d, len %d)", err, offset, len(data))
			return
		}
		p.pushNewTCClassifierRequest(event.VethPair.PeerDevice)
	case model.DNSEventType:
		if read, err = event.NetworkContext.UnmarshalBinary(data[offset:]); err != nil {
			seclog.Errorf("failed to decode Network Context")
		}
		offset += read

		if _, err = event.DNS.UnmarshalBinary(data[offset:]); err != nil {
			if errors.Is(err, model.ErrDNSNameMalformatted) {
				seclog.Debugf("failed to validate DNS event: %s", event.DNS.Name)
			} else if errors.Is(err, model.ErrDNSNamePointerNotSupported) {
				seclog.Tracef("failed to decode DNS event: %s (offset %d, len %d, data %s)", err, offset, len(data), string(data[offset:]))
			} else {
				seclog.Errorf("failed to decode DNS event: %s (offset %d, len %d, data %s))", err, offset, len(data), string(data[offset:]))
			}

			return
		}
	case model.IMDSEventType:
		if read, err = event.NetworkContext.UnmarshalBinary(data[offset:]); err != nil {
			seclog.Errorf("failed to decode Network Context")
		}
		offset += read

		if _, err = event.IMDS.UnmarshalBinary(data[offset:]); err != nil {
			// it's very possible we can't parse the IMDS body, as such let's put it as debug for now
			seclog.Debugf("failed to decode IMDS event: %s (offset %d, len %d)", err, offset, len(data))
			return
		}
		defer p.Resolvers.ProcessResolver.UpdateAWSSecurityCredentials(event.PIDContext.Pid, event)
	case model.RawPacketEventType:
		if _, err = event.RawPacket.UnmarshalBinary(data[offset:]); err != nil {
			seclog.Errorf("failed to decode RawPacket event: %s (offset %d, len %d)", err, offset, len(data))
			return
		}
	case model.BindEventType:
		if _, err = event.Bind.UnmarshalBinary(data[offset:]); err != nil {
			seclog.Errorf("failed to decode bind event: %s (offset %d, len %d)", err, offset, len(data))
			return
		}
	case model.ConnectEventType:
		if _, err = event.Connect.UnmarshalBinary(data[offset:]); err != nil {
			seclog.Errorf("failed to decode connect event: %s (offset %d, len %d)", err, offset, len(data))
			return
		}
	case model.SyscallsEventType:
		if _, err = event.Syscalls.UnmarshalBinary(data[offset:]); err != nil {
			seclog.Errorf("failed to decode syscalls event: %s (offset %d, len %d)", err, offset, len(data))
			return
		}
	case model.OnDemandEventType:
		if !p.onDemandRateLimiter.Allow() {
			seclog.Errorf("on-demand event rate limit reached, disabling on-demand probes to protect the system")
			p.onDemandManager.disable()
			return
		}

		if _, err = event.OnDemand.UnmarshalBinary(data[offset:]); err != nil {
			seclog.Errorf("failed to decode on-demand event for syscall event: %s (offset %d, len %d)", err, offset, len(data))
			return
		}
	}

	// resolve the container context
	event.ContainerContext, _ = p.fieldHandlers.ResolveContainerContext(event)

	p.DispatchEvent(event, true)

	if eventType == model.ExitEventType {
		p.Resolvers.ProcessResolver.DeleteEntry(event.ProcessContext.Pid, event.ResolveEventTime())
	}

	// flush pending actions
	p.processKiller.FlushPendingReports()
	p.fileHasher.FlushPendingReports()
}

// AddDiscarderPushedCallback add a callback to the list of func that have to be called when a discarder is pushed to kernel
func (p *EBPFProbe) AddDiscarderPushedCallback(cb DiscarderPushedCallback) {
	p.discarderPushedCallbacksLock.Lock()
	defer p.discarderPushedCallbacksLock.Unlock()

	p.discarderPushedCallbacks = append(p.discarderPushedCallbacks, cb)
}

// GetEventTags returns the event tags
func (p *EBPFProbe) GetEventTags(containerID string) []string {
	return p.Resolvers.TagsResolver.Resolve(containerID)
}

// OnNewDiscarder handles new discarders
func (p *EBPFProbe) OnNewDiscarder(rs *rules.RuleSet, ev *model.Event, field eval.Field, eventType eval.EventType) {
	// discarders disabled
	if !p.config.Probe.EnableDiscarders {
		return
	}

	if p.isRuntimeDiscarded {
		fakeTime := time.Unix(0, int64(ev.TimestampRaw))
		if !p.discarderRateLimiter.AllowN(fakeTime, 1) {
			return
		}
	}

	seclog.Tracef("New discarder of type %s for field %s", eventType, field)

	if handlers, ok := allDiscarderHandlers[eventType]; ok {
		for _, handler := range handlers {
			discarderPushed, _ := handler(rs, ev, p, Discarder{Field: field})

			if discarderPushed {
				p.discarderPushedCallbacksLock.RLock()
				defer p.discarderPushedCallbacksLock.RUnlock()
				for _, cb := range p.discarderPushedCallbacks {
					cb(eventType, ev, field)
				}
			}
		}
	}
}

// ApplyFilterPolicy is called when a passing policy for an event type is applied
func (p *EBPFProbe) ApplyFilterPolicy(eventType eval.EventType, mode kfilters.PolicyMode) error {
	seclog.Infof("Setting in-kernel filter policy to `%s` for `%s`", mode, eventType)
	table, err := managerhelper.Map(p.Manager, "filter_policy")
	if err != nil {
		return fmt.Errorf("unable to find policy table: %w", err)
	}

	et := config.ParseEvalEventType(eventType)
	if et == model.UnknownEventType {
		return fmt.Errorf("unable to parse the eval event type: %s", eventType)
	}

	policy := &kfilters.FilterPolicy{Mode: mode}

	return table.Put(ebpf.Uint32MapItem(et), policy)
}

// setApprovers applies approvers and removes the unused ones
func (p *EBPFProbe) setApprovers(eventType eval.EventType, approvers rules.Approvers) error {
	kfiltersGetter, exists := kfilters.KFilterGetters[eventType]
	if !exists {
		return nil
	}

	newKFilters, fieldHandled, err := kfiltersGetter(approvers)
	if err != nil {
		return err
	}

	if len(approvers) != len(fieldHandled) {
		return fmt.Errorf("all the approvers should be handled : %v vs %v", approvers, fieldHandled)
	}

	type tag struct {
		eventType    eval.EventType
		approverType string
	}
	approverAddedMetricCounter := make(map[tag]float64)

	for _, newKFilter := range newKFilters {
		seclog.Tracef("Applying kfilter %+v for event type %s", newKFilter, eventType)
		if err := newKFilter.Apply(p.Manager); err != nil {
			return err
		}

		approverType := newKFilter.GetApproverType()
		approverAddedMetricCounter[tag{eventType, approverType}]++
	}

	if previousKFilters, exist := p.kfilters[eventType]; exist {
		previousKFilters.Sub(newKFilters)
		for _, previousKFilter := range previousKFilters {
			seclog.Tracef("Removing previous kfilter %+v for event type %s", previousKFilter, eventType)
			if err := previousKFilter.Remove(p.Manager); err != nil {
				return err
			}

			approverType := previousKFilter.GetApproverType()
			approverAddedMetricCounter[tag{eventType, approverType}]--
			if approverAddedMetricCounter[tag{eventType, approverType}] <= 0 {
				delete(approverAddedMetricCounter, tag{eventType, approverType})
			}
		}
	}

	for tags, count := range approverAddedMetricCounter {
		tags := []string{
			fmt.Sprintf("approver_type:%s", tags.approverType),
			fmt.Sprintf("event_type:%s", tags.eventType),
		}

		if err := p.statsdClient.Gauge(metrics.MetricApproverAdded, count, tags, 1.0); err != nil {
			seclog.Tracef("couldn't set MetricApproverAdded metric: %s", err)
		}
	}

	p.kfilters[eventType] = newKFilters
	return nil
}

func (p *EBPFProbe) isNeededForActivityDump(eventType eval.EventType) bool {
	if p.config.RuntimeSecurity.ActivityDumpEnabled {
		for _, e := range p.profileManagers.GetActivityDumpTracedEventTypes() {
			if e.String() == eventType {
				return true
			}
		}
	}
	return false
}

func (p *EBPFProbe) isNeededForSecurityProfile(eventType eval.EventType) bool {
	if p.config.RuntimeSecurity.SecurityProfileEnabled {
		for _, e := range p.config.RuntimeSecurity.AnomalyDetectionEventTypes {
			if e.String() == eventType {
				return true
			}
		}
	}
	return false
}

func (p *EBPFProbe) validEventTypeForConfig(eventType string) bool {
	switch eventType {
	case "dns":
		return p.probe.IsNetworkEnabled()
	case "imds":
		return p.probe.IsNetworkEnabled() && p.config.Probe.NetworkIngressEnabled
	case "packet":
		return p.probe.IsNetworkRawPacketEnabled()
	}
	return true
}

// updateProbes applies the loaded set of rules and returns a report
// of the applied approvers for it.
func (p *EBPFProbe) updateProbes(ruleEventTypes []eval.EventType, needRawSyscalls bool) error {
	// event types enabled either by event handlers or by rules
	requestedEventTypes := append([]eval.EventType{}, defaultEventTypes...)
	requestedEventTypes = append(requestedEventTypes, ruleEventTypes...)
	for eventType, handlers := range p.probe.eventHandlers {
		if len(handlers) == 0 {
			continue
		}
		if slices.Contains(requestedEventTypes, model.EventType(eventType).String()) {
			continue
		}
		if eventType != int(model.UnknownEventType) && eventType != int(model.MaxAllEventType) {
			requestedEventTypes = append(requestedEventTypes, model.EventType(eventType).String())
		}
	}

	activatedProbes := probes.SnapshotSelectors()

	// extract probe to activate per the event types
	for eventType, selectors := range probes.GetSelectorsPerEventType(p.useFentry) {
		if (eventType == "*" || slices.Contains(requestedEventTypes, eventType) || p.isNeededForActivityDump(eventType) || p.isNeededForSecurityProfile(eventType) || p.config.Probe.EnableAllProbes) && p.validEventTypeForConfig(eventType) {
			activatedProbes = append(activatedProbes, selectors...)

			// to ensure the `enabled_events` map is correctly set with events that are enabled because of ADs
			if !slices.Contains(requestedEventTypes, eventType) {
				requestedEventTypes = append(requestedEventTypes, eventType)
			}
		}
	}

	// if we are using tracepoints to probe syscall exits, i.e. if we are using an old kernel version (< 4.12)
	// we need to use raw_syscall tracepoints for exits, as syscall are not trace when running an ia32 userspace
	// process
	if probes.ShouldUseSyscallExitTracepoints() {
		activatedProbes = append(activatedProbes, &manager.ProbeSelector{ProbeIdentificationPair: manager.ProbeIdentificationPair{UID: probes.SecurityAgentUID, EBPFFuncName: "sys_exit"}})
	}

	activatedProbes = append(activatedProbes, p.Resolvers.TCResolver.SelectTCProbes())

	// on-demand probes
	if p.config.RuntimeSecurity.OnDemandEnabled {
		p.onDemandManager.updateProbes()
		activatedProbes = append(activatedProbes, p.onDemandManager.selectProbes())
	}

	if needRawSyscalls {
		activatedProbes = append(activatedProbes, probes.SyscallMonitorSelectors...)
	} else {
		// ActivityDumps
		if p.config.RuntimeSecurity.ActivityDumpEnabled {
			for _, e := range p.profileManagers.GetActivityDumpTracedEventTypes() {
				if e == model.SyscallsEventType {
					activatedProbes = append(activatedProbes, probes.SyscallMonitorSelectors...)
					break
				}
			}
		}
		// SecurityProfiles
		if p.config.RuntimeSecurity.AnomalyDetectionEnabled {
			for _, e := range p.profileManagers.GetAnomalyDetectionEventTypes() {
				if e == model.SyscallsEventType {
					activatedProbes = append(activatedProbes, probes.SyscallMonitorSelectors...)
					break
				}
			}
		}
	}

	// Print the list of unique probe identification IDs that are registered
	var selectedIDs []manager.ProbeIdentificationPair
	for _, selector := range activatedProbes {
		for _, id := range selector.GetProbesIdentificationPairList() {
			var exists bool
			for _, selectedID := range selectedIDs {
				if selectedID == id {
					exists = true
				}
			}
			if !exists {
				selectedIDs = append(selectedIDs, id)
				seclog.Tracef("probe %s selected", id)
			}
		}
	}

	enabledEventsMap, err := managerhelper.Map(p.Manager, "enabled_events")
	if err != nil {
		return err
	}

	enabledEvents := uint64(0)
	for _, eventName := range requestedEventTypes {
		if eventName != "*" {
			eventType := config.ParseEvalEventType(eventName)
			if eventType == model.UnknownEventType {
				return fmt.Errorf("unknown event type '%s'", eventName)
			}
			enabledEvents |= 1 << (eventType - 1)
		}
	}

	if err := enabledEventsMap.Put(ebpf.ZeroUint32MapItem, enabledEvents); err != nil {
		return fmt.Errorf("failed to set enabled events: %w", err)
	}

	return p.Manager.UpdateActivatedProbes(activatedProbes)
}

// GetDiscarders retrieve the discarders
func (p *EBPFProbe) GetDiscarders() (*DiscardersDump, error) {
	inodeMap, err := managerhelper.Map(p.Manager, "inode_discarders")
	if err != nil {
		return nil, err
	}

	statsFB, err := managerhelper.Map(p.Manager, "fb_discarder_stats")
	if err != nil {
		return nil, err
	}

	statsBB, err := managerhelper.Map(p.Manager, "bb_discarder_stats")
	if err != nil {
		return nil, err
	}

	dump, err := dumpDiscarders(p.Resolvers.DentryResolver, inodeMap, statsFB, statsBB)
	if err != nil {
		return nil, err
	}
	return &dump, nil
}

// DumpDiscarders dump the discarders
func (p *EBPFProbe) DumpDiscarders() (string, error) {
	dump, err := p.GetDiscarders()
	if err != nil {
		return "", err
	}

	fp, err := os.CreateTemp("/tmp", "discarder-dump-")
	if err != nil {
		return "", err
	}
	defer fp.Close()

	if err := os.Chmod(fp.Name(), 0400); err != nil {
		return "", err
	}

	encoder := yaml.NewEncoder(fp)
	defer encoder.Close()

	if err := encoder.Encode(dump); err != nil {
		return "", err
	}
	err = fp.Close()
	if err != nil {
		return "", fmt.Errorf("could not close file [%s]: %w", fp.Name(), err)
	}
	return fp.Name(), err
}

// FlushDiscarders flush the discarders
func (p *EBPFProbe) FlushDiscarders() error {
	return bumpDiscardersRevision(p.Erpc)
}

// RefreshUserCache refreshes the user cache
func (p *EBPFProbe) RefreshUserCache(containerID string) error {
	return p.Resolvers.UserGroupResolver.RefreshCache(containerID)
}

// Snapshot runs the different snapshot functions of the resolvers that
// require to sync with the current state of the system
func (p *EBPFProbe) Snapshot() error {
	// the snapshot for the read of a lot of file which can allocate a lot of memory.
	defer runtime.GC()
	return p.Resolvers.Snapshot()
}

// Stop the probe
func (p *EBPFProbe) Stop() {
	_ = p.Manager.StopReaders(manager.CleanAll)
}

// Close the probe
func (p *EBPFProbe) Close() error {
	// Cancelling the context will stop the reorderer = we won't dequeue events anymore and new events from the
	// perf map reader are ignored
	p.cancelFnc()

	close(p.newTCNetDevices)

	// we wait until both the reorderer and the monitor are stopped
	p.wg.Wait()

	ddebpf.RemoveNameMappings(p.Manager)
	ebpftelemetry.UnregisterTelemetry(p.Manager)
	// Stopping the manager will stop the perf map reader and unload eBPF programs
	if err := p.Manager.Stop(manager.CleanAll); err != nil {
		return err
	}

	if err := p.Erpc.Close(); err != nil {
		return err
	}

	// when we reach this point, we do not generate nor consume events anymore, we can close the resolvers
	return p.Resolvers.Close()
}

// QueuedNetworkDeviceError is used to indicate that the new network device was queued until its namespace handle is
// resolved.
type QueuedNetworkDeviceError struct {
	msg string
}

func (err QueuedNetworkDeviceError) Error() string {
	return err.msg
}

func (p *EBPFProbe) pushNewTCClassifierRequest(device model.NetDevice) {
	select {
	case <-p.ctx.Done():
		// the probe is stopping, do not push the new tc classifier request
		return
	case p.newTCNetDevices <- device:
		// do nothing
	default:
		seclog.Errorf("failed to slot new tc classifier request: %v", device)
	}
}

func (p *EBPFProbe) startSetupNewTCClassifierLoop() {
	for {
		select {
		case <-p.ctx.Done():
			return
		case netDevice, ok := <-p.newTCNetDevices:
			if !ok {
				return
			}

			if err := p.setupNewTCClassifier(netDevice); err != nil {
				var qnde QueuedNetworkDeviceError
				if errors.As(err, &qnde) {
					seclog.Debugf("%v", err)
				} else {
					seclog.Errorf("error setting up new tc classifier: %v", err)
				}
			}
		}
	}
}

func (p *EBPFProbe) setupNewTCClassifier(device model.NetDevice) error {
	// select netns handle
	var handle *os.File
	var err error
	netns := p.Resolvers.NamespaceResolver.ResolveNetworkNamespace(device.NetNS)
	if netns != nil {
		handle, err = netns.GetNamespaceHandleDup()
	}
	if err != nil {
		defer handle.Close()
	}
	if netns == nil || err != nil || handle == nil {
		// queue network device so that a TC classifier can be added later
		p.Resolvers.NamespaceResolver.QueueNetworkDevice(device)
		return QueuedNetworkDeviceError{msg: fmt.Sprintf("device %s is queued until %d is resolved", device.Name, device.NetNS)}
	}
	err = p.Resolvers.TCResolver.SetupNewTCClassifierWithNetNSHandle(device, handle, p.Manager)
	if err != nil {
		return err
	}
	if err := handle.Close(); err != nil {
		return fmt.Errorf("could not close file [%s]: %w", handle.Name(), err)
	}

	return nil
}

// FlushNetworkNamespace removes all references and stops all TC programs in the provided network namespace. This method
// flushes the network namespace in the network namespace resolver as well.
func (p *EBPFProbe) FlushNetworkNamespace(namespace *netns.NetworkNamespace) {
	p.Resolvers.NamespaceResolver.FlushNetworkNamespace(namespace)

	// cleanup internal structures
	p.Resolvers.TCResolver.FlushNetworkNamespaceID(namespace.ID(), p.Manager)
}

func (p *EBPFProbe) handleNewMount(ev *model.Event, m *model.Mount) error {
	// There could be entries of a previous mount_id in the cache for instance,
	// runc does the following : it bind mounts itself (using /proc/exe/self),
	// opens a file descriptor on the new file with O_CLOEXEC then umount the bind mount using
	// MNT_DETACH. It then does an exec syscall, that will cause the fd to be closed.
	// Our dentry resolution of the exec event causes the inode/mount_id to be put in cache,
	// so we remove all dentry entries belonging to the mountID.
	p.Resolvers.DentryResolver.DelCacheEntries(m.MountID)

	// Resolve mount point
	if err := p.Resolvers.PathResolver.SetMountPoint(ev, m); err != nil {
		return fmt.Errorf("failed to set mount point: %w", err)
	}
	// Resolve root
	if err := p.Resolvers.PathResolver.SetMountRoot(ev, m); err != nil {
		return fmt.Errorf("failed to set mount root: %w", err)
	}

	// Insert new mount point in cache, passing it a copy of the mount that we got from the event
	if err := p.Resolvers.MountResolver.Insert(*m, 0); err != nil {
		return fmt.Errorf("failed to insert mount event: %w", err)
	}

	return nil
}

func (p *EBPFProbe) applyDefaultFilterPolicies() {
	if !p.config.Probe.EnableKernelFilters {
		seclog.Warnf("Forcing in-kernel filter policy to `pass`: filtering not enabled")
	}

	for eventType := model.FirstEventType; eventType <= model.LastEventType; eventType++ {
		var mode kfilters.PolicyMode

		if !p.config.Probe.EnableKernelFilters {
			mode = kfilters.PolicyModeNoFilter
		} else if len(p.probe.eventHandlers[eventType]) > 0 {
			mode = kfilters.PolicyModeAccept
		} else {
			mode = kfilters.PolicyModeDeny
		}

		if err := p.ApplyFilterPolicy(eventType.String(), mode); err != nil {
			seclog.Debugf("unable to apply to filter policy `%s` for `%s`", eventType, mode)
		}
	}
}

func isKillActionPresent(rs *rules.RuleSet) bool {
	for _, rule := range rs.GetRules() {
		for _, action := range rule.Def.Actions {
			if action.Kill != nil {
				return true
			}
		}
	}
	return false
}

// ApplyRuleSet apply the required update to handle the new ruleset
func (p *EBPFProbe) ApplyRuleSet(rs *rules.RuleSet) (*kfilters.ApplyRuleSetReport, error) {
	if p.opts.SyscallsMonitorEnabled {
		if err := p.monitors.syscallsMonitor.Disable(); err != nil {
			return nil, err
		}
	}

	ars, err := kfilters.NewApplyRuleSetReport(p.config.Probe, rs)
	if err != nil {
		return nil, err
	}

	for eventType, report := range ars.Policies {
		if err := p.setApprovers(eventType, report.Approvers); err != nil {
			seclog.Errorf("Error while adding approvers fallback in-kernel policy to `%s` for `%s`: %s", kfilters.PolicyModeAccept, eventType, err)

			if err := p.ApplyFilterPolicy(eventType, kfilters.PolicyModeAccept); err != nil {
				return nil, err
			}
		} else {
			if err := p.ApplyFilterPolicy(eventType, report.Mode); err != nil {
				return nil, err
			}
		}
	}

	eventTypes := rs.GetEventTypes()

	// activity dump & security profiles
	needRawSyscalls := p.isNeededForActivityDump(model.SyscallsEventType.String())

	// kill action
	if p.config.RuntimeSecurity.EnforcementEnabled && isKillActionPresent(rs) {
		if !p.config.RuntimeSecurity.EnforcementRawSyscallEnabled {
			// force FIM and Process category so that we can catch most of the activity
			categories := model.GetEventTypePerCategory(model.FIMCategory, model.ProcessCategory)
			for _, list := range categories {
				for _, eventType := range list {
					if !slices.Contains(eventTypes, eventType) {
						eventTypes = append(eventTypes, eventType)
					}
				}
			}
		} else {
			needRawSyscalls = true
		}
	}

	if p.config.RuntimeSecurity.OnDemandEnabled {
		p.onDemandManager.setHookPoints(rs.GetOnDemandHookPoints())
	}

	if err := p.updateProbes(eventTypes, needRawSyscalls); err != nil {
		return nil, fmt.Errorf("failed to select probes: %w", err)
	}

	if p.opts.SyscallsMonitorEnabled {
		if err := p.monitors.syscallsMonitor.Flush(); err != nil {
			return nil, err
		}
		if err := p.monitors.syscallsMonitor.Enable(); err != nil {
			return nil, err
		}
	}

	// do not replay the snapshot if we are in the first rule set version, this was already done in the start method
	if p.ruleSetVersion != 0 {
		p.playSnapShotState.Store(true)
	}

	p.ruleSetVersion++

	return ars, nil
}

// OnNewRuleSetLoaded resets statistics and states once a new rule set is loaded
func (p *EBPFProbe) OnNewRuleSetLoaded(rs *rules.RuleSet) {
	p.processKiller.Reset(rs)
}

// NewEvent returns a new event
func (p *EBPFProbe) NewEvent() *model.Event {
	return NewEBPFEvent(p.fieldHandlers)
}

// GetFieldHandlers returns the field handlers
func (p *EBPFProbe) GetFieldHandlers() model.FieldHandlers {
	return p.fieldHandlers
}

// DumpProcessCache dumps the process cache
func (p *EBPFProbe) DumpProcessCache(withArgs bool) (string, error) {
	return p.Resolvers.ProcessResolver.ToDot(withArgs)
}

// EnableEnforcement sets the enforcement mode
func (p *EBPFProbe) EnableEnforcement(state bool) {
	p.processKiller.SetState(state)
}

// NewEBPFProbe instantiates a new runtime security agent probe
func NewEBPFProbe(probe *Probe, config *config.Config, opts Opts, telemetry telemetry.Component) (*EBPFProbe, error) {
	nerpc, err := erpc.NewERPC()
	if err != nil {
		return nil, err
	}

	onDemandRate := rate.Limit(math.Inf(1))
	if config.RuntimeSecurity.OnDemandRateLimiterEnabled {
		onDemandRate = MaxOnDemandEventsPerSecond
	}

	processKiller, err := NewProcessKiller(config)
	if err != nil {
		return nil, err
	}

	ctx, cancelFnc := context.WithCancel(context.Background())

	p := &EBPFProbe{
		probe:                probe,
		config:               config,
		opts:                 opts,
		statsdClient:         opts.StatsdClient,
		discarderRateLimiter: rate.NewLimiter(rate.Every(time.Second/5), 100),
		kfilters:             make(map[eval.EventType]kfilters.ActiveKFilters),
		managerOptions:       ebpf.NewDefaultOptions(),
		Erpc:                 nerpc,
		erpcRequest:          erpc.NewERPCRequest(0),
		isRuntimeDiscarded:   !probe.Opts.DontDiscardRuntime,
		ctx:                  ctx,
		cancelFnc:            cancelFnc,
		newTCNetDevices:      make(chan model.NetDevice, 16),
		processKiller:        processKiller,
		onDemandRateLimiter:  rate.NewLimiter(onDemandRate, 1),
		playSnapShotState:    atomic.NewBool(false),
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

	p.selectFentryMode()

	useRingBuffers := p.UseRingBuffers()
	useMmapableMaps := p.kernelVersion.HaveMmapableMaps()

	p.Manager = ebpf.NewRuntimeSecurityManager(useRingBuffers, p.useFentry)

	p.supportsBPFSendSignal = p.kernelVersion.SupportBPFSendSignal()

	p.ensureConfigDefaults()

	p.monitors = NewEBPFMonitors(p)

	numCPU, err := utils.NumCPU()
	if err != nil {
		return nil, fmt.Errorf("failed to parse CPU count: %w", err)
	}

	p.managerOptions.MapSpecEditors = probes.AllMapSpecEditors(numCPU, probes.MapSpecEditorOpts{
		TracedCgroupSize:        config.RuntimeSecurity.ActivityDumpTracedCgroupsCount,
		UseRingBuffers:          useRingBuffers,
		UseMmapableMaps:         useMmapableMaps,
		RingBufferSize:          uint32(config.Probe.EventStreamBufferSize),
		PathResolutionEnabled:   probe.Opts.PathResolutionEnabled,
		SecurityProfileMaxCount: config.RuntimeSecurity.SecurityProfileMaxCount,
	})

	if config.RuntimeSecurity.ActivityDumpEnabled {
		for _, e := range config.RuntimeSecurity.ActivityDumpTracedEventTypes {
			if e == model.SyscallsEventType {
				// Add syscall monitor probes
				p.managerOptions.ActivatedProbes = append(p.managerOptions.ActivatedProbes, probes.SyscallMonitorSelectors...)
				break
			}
		}
	}
	if config.RuntimeSecurity.AnomalyDetectionEnabled {
		for _, e := range config.RuntimeSecurity.AnomalyDetectionEventTypes {
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

	p.managerOptions.ConstantEditors = append(p.managerOptions.ConstantEditors, constantfetch.CreateConstantEditors(p.constantOffsets)...)

	p.managerOptions.ConstantEditors = append(p.managerOptions.ConstantEditors,
		manager.ConstantEditor{
			Name:  constantfetch.OffsetNameSchedProcessForkChildPid,
			Value: constantfetch.ReadTracepointFieldOffsetWithFallback("sched/sched_process_fork", "child_pid", 44),
		},
		manager.ConstantEditor{
			Name:  constantfetch.OffsetNameSchedProcessForkParentPid,
			Value: constantfetch.ReadTracepointFieldOffsetWithFallback("sched/sched_process_fork", "parent_pid", 24),
		},
	)

	areCGroupADsEnabled := config.RuntimeSecurity.ActivityDumpTracedCgroupsCount > 0

	// Add global constant editors
	p.managerOptions.ConstantEditors = append(p.managerOptions.ConstantEditors,
		manager.ConstantEditor{
			Name:  "runtime_pid",
			Value: uint64(utils.Getpid()),
		},
		manager.ConstantEditor{
			Name:  "do_fork_input",
			Value: getDoForkInput(p.kernelVersion),
		},
		manager.ConstantEditor{
			Name:  "do_dentry_open_without_inode",
			Value: getDoDentryOpenWithoutInode(p.kernelVersion),
		},
		manager.ConstantEditor{
			Name:  "has_usernamespace_first_arg",
			Value: getHasUsernamespaceFirstArg(p.kernelVersion),
		},
		manager.ConstantEditor{
			Name:  "ovl_path_in_ovl_inode",
			Value: getOvlPathInOvlInode(p.kernelVersion),
		},
		manager.ConstantEditor{
			Name:  "getattr2",
			Value: getAttr2(p.kernelVersion),
		},
		manager.ConstantEditor{
			Name:  "vfs_unlink_dentry_position",
			Value: mount.GetVFSLinkDentryPosition(p.kernelVersion),
		},
		manager.ConstantEditor{
			Name:  "vfs_mkdir_dentry_position",
			Value: mount.GetVFSMKDirDentryPosition(p.kernelVersion),
		},
		manager.ConstantEditor{
			Name:  "vfs_link_target_dentry_position",
			Value: mount.GetVFSLinkTargetDentryPosition(p.kernelVersion),
		},
		manager.ConstantEditor{
			Name:  "vfs_setxattr_dentry_position",
			Value: mount.GetVFSSetxattrDentryPosition(p.kernelVersion),
		},
		manager.ConstantEditor{
			Name:  "vfs_removexattr_dentry_position",
			Value: mount.GetVFSRemovexattrDentryPosition(p.kernelVersion),
		},
		manager.ConstantEditor{
			Name:  "vfs_rename_input_type",
			Value: getVFSRenameRegisterArgsOrStruct(p.kernelVersion),
		},
		manager.ConstantEditor{
			Name:  "check_helper_call_input",
			Value: getCheckHelperCallInputType(p.kernelVersion),
		},
		manager.ConstantEditor{
			Name:  "cgroup_activity_dumps_enabled",
			Value: utils.BoolTouint64(config.RuntimeSecurity.ActivityDumpEnabled && areCGroupADsEnabled),
		},
		manager.ConstantEditor{
			Name:  "net_struct_type",
			Value: getNetStructType(p.kernelVersion),
		},
		manager.ConstantEditor{
			Name:  "syscall_monitor_event_period",
			Value: uint64(config.RuntimeSecurity.ActivityDumpSyscallMonitorPeriod.Nanoseconds()),
		},
		manager.ConstantEditor{
			Name:  "send_signal",
			Value: utils.BoolTouint64(p.kernelVersion.SupportBPFSendSignal()),
		},
		manager.ConstantEditor{
			Name:  "anomaly_syscalls",
			Value: utils.BoolTouint64(slices.Contains(config.RuntimeSecurity.AnomalyDetectionEventTypes, model.SyscallsEventType)),
		},
		manager.ConstantEditor{
			Name:  "monitor_syscalls_map_enabled",
			Value: utils.BoolTouint64(probe.Opts.SyscallsMonitorEnabled),
		},
		manager.ConstantEditor{
			Name:  "imds_ip",
			Value: uint64(config.RuntimeSecurity.IMDSIPv4),
		},
	)

	p.managerOptions.ConstantEditors = append(p.managerOptions.ConstantEditors, DiscarderConstants...)
	p.managerOptions.ConstantEditors = append(p.managerOptions.ConstantEditors, getCGroupWriteConstants())

	p.managerOptions.ConstantEditors = append(p.managerOptions.ConstantEditors,
		manager.ConstantEditor{
			Name:  "use_ring_buffer",
			Value: utils.BoolTouint64(useRingBuffers),
		},
	)

	if p.kernelVersion.HavePIDLinkStruct() {
		p.managerOptions.ConstantEditors = append(p.managerOptions.ConstantEditors,
			manager.ConstantEditor{
				Name:  "kernel_has_pid_link_struct",
				Value: utils.BoolTouint64(true),
			},
		)
	}

	if p.kernelVersion.HaveLegacyPipeInodeInfoStruct() {
		p.managerOptions.ConstantEditors = append(p.managerOptions.ConstantEditors,
			manager.ConstantEditor{
				Name:  "kernel_has_legacy_pipe_inode_info",
				Value: utils.BoolTouint64(true),
			},
		)
	}

	// tail calls
	p.managerOptions.TailCallRouter = probes.AllTailRoutes(config.Probe.ERPCDentryResolutionEnabled, config.Probe.NetworkEnabled, useMmapableMaps)
	if !config.Probe.ERPCDentryResolutionEnabled || useMmapableMaps {
		// exclude the programs that use the bpf_probe_write_user helper
		p.managerOptions.ExcludedFunctions = probes.AllBPFProbeWriteUserProgramFunctions()
	}

	if !config.Probe.NetworkEnabled {
		// prevent all TC classifiers from loading
		p.managerOptions.ExcludedFunctions = append(p.managerOptions.ExcludedFunctions, probes.GetAllTCProgramFunctions()...)
	}

	if p.useFentry {
		afBasedExcluder, err := newAvailableFunctionsBasedExcluder()
		if err != nil {
			return nil, err
		}

		p.managerOptions.AdditionalExcludedFunctionCollector = afBasedExcluder
	}

	resolversOpts := resolvers.Opts{
		PathResolutionEnabled:    probe.Opts.PathResolutionEnabled,
		EnvVarsResolutionEnabled: probe.Opts.EnvsVarResolutionEnabled,
		TagsResolver:             probe.Opts.TagsResolver,
		UseRingBuffer:            useRingBuffers,
		TTYFallbackEnabled:       probe.Opts.TTYFallbackEnabled,
	}

	p.Resolvers, err = resolvers.NewEBPFResolvers(config, p.Manager, probe.StatsdClient, probe.scrubber, p.Erpc, resolversOpts, telemetry)
	if err != nil {
		return nil, err
	}

	p.fileHasher = NewFileHasher(config, p.Resolvers.HashResolver)

	hostname, err := utils.GetHostname()
	if err != nil || hostname == "" {
		hostname = "unknown"
	}

	if config.RuntimeSecurity.OnDemandEnabled {
		p.onDemandManager = &OnDemandProbesManager{
			probe:   p,
			manager: p.Manager,
		}
	}

	p.fieldHandlers = &EBPFFieldHandlers{config: config, resolvers: p.Resolvers, hostname: hostname, onDemand: p.onDemandManager}

	if useRingBuffers {
		p.eventStream = ringbuffer.New(p.handleEvent)
		p.managerOptions.SkipRingbufferReaderStartup = map[string]bool{
			eventstream.EventStreamMap: true,
		}
	} else {
		p.eventStream, err = reorderer.NewOrderedPerfMap(p.ctx, p.handleEvent, probe.StatsdClient)
		if err != nil {
			return nil, err
		}
		p.managerOptions.SkipPerfMapReaderStartup = map[string]bool{
			eventstream.EventStreamMap: true,
		}
	}

	p.event = p.NewEvent()

	// be sure to zero the probe event before everything else
	p.zeroEvent()

	return p, nil
}

// GetProfileManagers returns the security profile managers
func (p *EBPFProbe) GetProfileManagers() *SecurityProfileManagers {
	return p.profileManagers
}

func (p *EBPFProbe) ensureConfigDefaults() {
	// enable runtime compiled constants on COS by default
	if !p.config.Probe.RuntimeCompiledConstantsIsSet && p.kernelVersion.IsCOSKernel() {
		p.config.Probe.RuntimeCompiledConstantsEnabled = true
	}
}

const (
	netStructHasProcINum uint64 = 0
	netStructHasNS       uint64 = 1
)

// getNetStructType returns whether the net structure has a namespace attribute
func getNetStructType(kv *kernel.Version) uint64 {
	if kv.IsRH7Kernel() {
		return netStructHasProcINum
	}
	return netStructHasNS
}

const (
	doForkListInput uint64 = iota
	doForkStructInput
)

func getAttr2(kernelVersion *kernel.Version) uint64 {
	if kernelVersion.IsRH7Kernel() {
		return 1
	}
	return 0
}

// getDoForkInput returns the expected input type of _do_fork, do_fork and kernel_clone
func getDoForkInput(kernelVersion *kernel.Version) uint64 {
	if kernelVersion.Code != 0 && kernelVersion.Code >= kernel.Kernel5_3 {
		return doForkStructInput
	}
	return doForkListInput
}

func getDoDentryOpenWithoutInode(kernelversion *kernel.Version) uint64 {
	if kernelversion.Code != 0 && kernelversion.Code >= kernel.Kernel6_10 {
		return 1
	}
	return 0
}

func getHasUsernamespaceFirstArg(kernelVersion *kernel.Version) uint64 {
	if val, err := constantfetch.GetHasUsernamespaceFirstArgWithBtf(); err == nil {
		if val {
			return 1
		}
		return 0
	}

	switch {
	case kernelVersion.Code != 0 && kernelVersion.Code >= kernel.Kernel6_0:
		return 1
	case kernelVersion.IsInRangeCloseOpen(kernel.Kernel5_14, kernel.Kernel5_15) && kernelVersion.IsRH9_3Kernel():
		return 1
	default:
		return 0
	}
}

func getVFSRenameRegisterArgsOrStruct(kernelVersion *kernel.Version) uint64 {
	if val, err := constantfetch.GetHasVFSRenameStructArgs(); err == nil {
		if val {
			return 2
		}
		return 1
	}

	if kernelVersion.Code >= kernel.Kernel5_12 {
		return 2
	}
	if kernelVersion.IsInRangeCloseOpen(kernel.Kernel5_10, kernel.Kernel5_11) && kernelVersion.Code.Patch() >= 220 {
		return 2
	}

	return 1
}

func getOvlPathInOvlInode(kernelVersion *kernel.Version) uint64 {
	// https://github.com/torvalds/linux/commit/0af950f57fefabab628f1963af881e6b9bfe7f38
	if kernelVersion.Code != 0 && kernelVersion.Code >= kernel.Kernel6_5 {
		return 2
	}

	// https://github.com/torvalds/linux/commit/ffa5723c6d259b3191f851a50a98d0352b345b39
	// changes a bit how the lower dentry/inode is stored in `ovl_inode`. To check if we
	// are in this configuration we first probe the kernel version, then we check for the
	// presence of the function introduced in the same patch.
	const patchSentinel = "ovl_i_path_real"

	if kernelVersion.Code != 0 && kernelVersion.Code >= kernel.Kernel5_19 {
		return 1
	}

	check, err := ddebpf.VerifyKernelFuncs(patchSentinel)
	if err != nil {
		return 0
	}

	// VerifyKernelFuncs returns the missing functions
	if _, ok := check[patchSentinel]; !ok {
		return 1
	}

	return 0
}

// getCGroupWriteConstants returns the value of the constant used to determine how cgroups should be captured in kernel
// space
func getCGroupWriteConstants() manager.ConstantEditor {
	cgroupWriteConst := uint64(1)
	kv, err := kernel.NewKernelVersion()
	if err == nil {
		if kv.IsRH7Kernel() {
			cgroupWriteConst = 2
		}
	}

	return manager.ConstantEditor{
		Name:  "cgroup_write_type",
		Value: cgroupWriteConst,
	}
}

// GetOffsetConstants returns the offsets and struct sizes constants
func (p *EBPFProbe) GetOffsetConstants() (map[string]uint64, error) {
	constantFetcher := constantfetch.ComposeConstantFetchers(constantfetch.GetAvailableConstantFetchers(p.config.Probe, p.kernelVersion, p.statsdClient))
	AppendProbeRequestsToFetcher(constantFetcher, p.kernelVersion)
	return constantFetcher.FinishAndGetResults()
}

// GetConstantFetcherStatus returns the status of the constant fetcher associated with this probe
func (p *EBPFProbe) GetConstantFetcherStatus() (*constantfetch.ConstantFetcherStatus, error) {
	constantFetcher := constantfetch.ComposeConstantFetchers(constantfetch.GetAvailableConstantFetchers(p.config.Probe, p.kernelVersion, p.statsdClient))
	AppendProbeRequestsToFetcher(constantFetcher, p.kernelVersion)
	return constantFetcher.FinishAndGetStatus()
}

// AppendProbeRequestsToFetcher returns the offsets and struct sizes constants, from a constant fetcher
func AppendProbeRequestsToFetcher(constantFetcher constantfetch.ConstantFetcher, kv *kernel.Version) {
	constantFetcher.AppendSizeofRequest(constantfetch.SizeOfInode, "struct inode", "linux/fs.h")
	constantFetcher.AppendOffsetofRequest(constantfetch.OffsetNameSuperBlockStructSFlags, "struct super_block", "s_flags", "linux/fs.h")
	constantFetcher.AppendOffsetofRequest(constantfetch.OffsetNameSuperBlockStructSMagic, "struct super_block", "s_magic", "linux/fs.h")
	constantFetcher.AppendOffsetofRequest(constantfetch.OffsetNameDentryStructDSB, "struct dentry", "d_sb", "linux/dcache.h")
	constantFetcher.AppendOffsetofRequest(constantfetch.OffsetNameSignalStructStructTTY, "struct signal_struct", "tty", "linux/sched/signal.h")
	constantFetcher.AppendOffsetofRequest(constantfetch.OffsetNameTTYStructStructName, "struct tty_struct", "name", "linux/tty.h")
	constantFetcher.AppendOffsetofRequest(constantfetch.OffsetNameCredStructUID, "struct cred", "uid", "linux/cred.h")
	constantFetcher.AppendOffsetofRequest(constantfetch.OffsetNameCredStructCapInheritable, "struct cred", "cap_inheritable", "linux/cred.h")
	constantFetcher.AppendOffsetofRequest(constantfetch.OffsetNameLinuxBinprmP, "struct linux_binprm", "p", "linux/binfmts.h")
	constantFetcher.AppendOffsetofRequest(constantfetch.OffsetNameLinuxBinprmArgc, "struct linux_binprm", "argc", "linux/binfmts.h")
	constantFetcher.AppendOffsetofRequest(constantfetch.OffsetNameLinuxBinprmEnvc, "struct linux_binprm", "envc", "linux/binfmts.h")
	constantFetcher.AppendOffsetofRequest(constantfetch.OffsetNameVMAreaStructFlags, "struct vm_area_struct", "vm_flags", "linux/mm_types.h")
	constantFetcher.AppendOffsetofRequest(constantfetch.OffsetNameFileFinode, "struct file", "f_inode", "linux/fs.h")
	constantFetcher.AppendOffsetofRequest(constantfetch.OffsetNameFileFpath, "struct file", "f_path", "linux/fs.h")
	constantFetcher.AppendOffsetofRequest(constantfetch.OffsetNameMountMntID, "struct mount", "mnt_id", "")
	if kv.Code >= kernel.Kernel5_3 {
		constantFetcher.AppendOffsetofRequest(constantfetch.OffsetNameKernelCloneArgsExitSignal, "struct kernel_clone_args", "exit_signal", "linux/sched/task.h")
	}

	// inode time offsets
	// no runtime compilation for those, we expect them to default to 0 if not there and use the fallback
	// in the eBPF code itself in that case
	if kv.Code >= kernel.Kernel6_11 {
		constantFetcher.AppendOffsetofRequest(constantfetch.OffsetNameInodeCtimeSec, "struct inode", "i_ctime_sec", "")
		constantFetcher.AppendOffsetofRequest(constantfetch.OffsetNameInodeCtimeNsec, "struct inode", "i_ctime_nsec", "")
		constantFetcher.AppendOffsetofRequest(constantfetch.OffsetNameInodeMtimeSec, "struct inode", "i_mtime_sec", "")
		constantFetcher.AppendOffsetofRequest(constantfetch.OffsetNameInodeMtimeNsec, "struct inode", "i_mtime_nsec", "")
	}

	// rename offsets
	if kv.Code >= kernel.Kernel5_12 || (kv.IsInRangeCloseOpen(kernel.Kernel5_10, kernel.Kernel5_11) && kv.Code.Patch() >= 220) {
		constantFetcher.AppendOffsetofRequest(constantfetch.OffsetNameRenameStructOldDentry, "struct renamedata", "old_dentry", "linux/fs.h")
		constantFetcher.AppendOffsetofRequest(constantfetch.OffsetNameRenameStructNewDentry, "struct renamedata", "new_dentry", "linux/fs.h")
	}

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
	if kv.HavePIDLinkStruct() {
		constantFetcher.AppendOffsetofRequest(constantfetch.OffsetNameTaskStructPIDLink, "struct task_struct", "pids", "linux/sched.h")
		constantFetcher.AppendOffsetofRequest(constantfetch.OffsetNamePIDLinkStructPID, "struct pid_link", "pid", "linux/pid.h")
	} else {
		constantFetcher.AppendOffsetofRequest(constantfetch.OffsetNameTaskStructPID, "struct task_struct", "thread_pid", "linux/sched.h")
	}

	// splice event
	constantFetcher.AppendOffsetofRequest(constantfetch.OffsetNamePipeInodeInfoStructBufs, "struct pipe_inode_info", "bufs", "linux/pipe_fs_i.h")
	if kv.HaveLegacyPipeInodeInfoStruct() {
		constantFetcher.AppendOffsetofRequest(constantfetch.OffsetNamePipeInodeInfoStructNrbufs, "struct pipe_inode_info", "nrbufs", "linux/pipe_fs_i.h")
		constantFetcher.AppendOffsetofRequest(constantfetch.OffsetNamePipeInodeInfoStructCurbuf, "struct pipe_inode_info", "curbuf", "linux/pipe_fs_i.h")
		constantFetcher.AppendOffsetofRequest(constantfetch.OffsetNamePipeInodeInfoStructBuffers, "struct pipe_inode_info", "buffers", "linux/pipe_fs_i.h")
	} else {
		constantFetcher.AppendOffsetofRequest(constantfetch.OffsetNamePipeInodeInfoStructHead, "struct pipe_inode_info", "head", "linux/pipe_fs_i.h")
		constantFetcher.AppendOffsetofRequest(constantfetch.OffsetNamePipeInodeInfoStructRingsize, "struct pipe_inode_info", "ring_size", "linux/pipe_fs_i.h")
	}

	// network related constants
	constantFetcher.AppendOffsetofRequest(constantfetch.OffsetNameNetDeviceStructIfIndex, "struct net_device", "ifindex", "linux/netdevice.h")
	constantFetcher.AppendOffsetofRequest(constantfetch.OffsetNameNetDeviceStructName, "struct net_device", "name", "linux/netdevice.h")
	constantFetcher.AppendOffsetofRequest(constantfetch.OffsetNameDeviceStructNdNet, "struct net_device", "nd_net", "linux/netdevice.h")
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

// HandleActions handles the rule actions
func (p *EBPFProbe) HandleActions(ctx *eval.Context, rule *rules.Rule) {
	ev := ctx.Event.(*model.Event)

	for _, action := range rule.Actions {
		if !action.IsAccepted(ctx) {
			continue
		}

		switch {
		case action.InternalCallback != nil && rule.ID == bundled.RefreshUserCacheRuleID:
			_ = p.RefreshUserCache(string(ev.ContainerContext.ContainerID))

		case action.InternalCallback != nil && rule.ID == bundled.RefreshSBOMRuleID && p.Resolvers.SBOMResolver != nil && len(ev.ContainerContext.ContainerID) > 0:
			if err := p.Resolvers.SBOMResolver.RefreshSBOM(string(ev.ContainerContext.ContainerID)); err != nil {
				seclog.Warnf("failed to refresh SBOM for container %s, triggered by %s: %s", ev.ContainerContext.ContainerID, ev.ProcessContext.Comm, err)
			}

		case action.Def.Kill != nil:
			// do not handle kill action on event with error
			if ev.Error != nil {
				return
			}

			if p.processKiller.KillAndReport(action.Def.Kill, rule, ev, func(pid uint32, sig uint32) error {
				if p.supportsBPFSendSignal {
					if err := p.killListMap.Put(uint32(pid), uint32(sig)); err != nil {
						seclog.Warnf("failed to kill process with eBPF %d: %s", pid, err)
					}
				}
				return p.processKiller.KillFromUserspace(pid, sig, ev)
			}) {
				p.probe.onRuleActionPerformed(rule, action.Def)
			}

		case action.Def.CoreDump != nil:
			if p.config.RuntimeSecurity.InternalMonitoringEnabled {
				dump := NewCoreDump(action.Def.CoreDump, p.Resolvers, serializers.NewEventSerializer(ev, nil))
				rule := events.NewCustomRule(events.InternalCoreDumpRuleID, events.InternalCoreDumpRuleDesc)
				event := events.NewCustomEvent(model.UnknownEventType, dump)

				p.probe.DispatchCustomEvent(rule, event)
				p.probe.onRuleActionPerformed(rule, action.Def)
			}
		case action.Def.Hash != nil:
			if p.fileHasher.HashAndReport(rule, ev) {
				p.probe.onRuleActionPerformed(rule, action.Def)
			}
		}
	}
}

// newPlaceholderProcessCacheEntryPTraceMe returns a new empty process cache entry for PTRACE_TRACEME calls,
// it's similar to model.NewPlaceholderProcessCacheEntry with pid = tid = 0, and isKworker = false
var newPlaceholderProcessCacheEntryPTraceMe = sync.OnceValue(func() *model.ProcessCacheEntry {
	return model.NewPlaceholderProcessCacheEntry(0, 0, false)
})
