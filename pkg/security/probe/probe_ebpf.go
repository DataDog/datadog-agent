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
	"net"
	"net/netip"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/google/gopacket"
	"github.com/google/gopacket/layers"

	lib "github.com/cilium/ebpf"
	"github.com/hashicorp/go-multierror"
	"github.com/moby/sys/mountinfo"
	"github.com/vishvananda/netlink"
	"go.uber.org/atomic"
	"golang.org/x/sys/unix"
	"golang.org/x/time/rate"
	"gopkg.in/yaml.v3"

	"github.com/DataDog/datadog-go/v5/statsd"
	manager "github.com/DataDog/ebpf-manager"
	"github.com/DataDog/ebpf-manager/tracefs"

	ipc "github.com/DataDog/datadog-agent/comp/core/ipc/def"
	"github.com/DataDog/datadog-agent/pkg/config/env"
	ddebpf "github.com/DataDog/datadog-agent/pkg/ebpf"
	bugs "github.com/DataDog/datadog-agent/pkg/ebpf/kernelbugs"
	ebpftelemetry "github.com/DataDog/datadog-agent/pkg/ebpf/telemetry"
	"github.com/DataDog/datadog-agent/pkg/security/config"
	"github.com/DataDog/datadog-agent/pkg/security/ebpf"
	"github.com/DataDog/datadog-agent/pkg/security/ebpf/kernel"
	"github.com/DataDog/datadog-agent/pkg/security/ebpf/probes"
	"github.com/DataDog/datadog-agent/pkg/security/ebpf/probes/rawpacket"
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
	"github.com/DataDog/datadog-agent/pkg/security/probe/sysctl"
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
	securityprofile "github.com/DataDog/datadog-agent/pkg/security/security_profile"
	"github.com/DataDog/datadog-agent/pkg/security/security_profile/storage/backend"
	"github.com/DataDog/datadog-agent/pkg/security/serializers"
	"github.com/DataDog/datadog-agent/pkg/security/utils"
	"github.com/DataDog/datadog-agent/pkg/security/utils/hostnameutils"
	utilkernel "github.com/DataDog/datadog-agent/pkg/util/kernel"
	ddsync "github.com/DataDog/datadog-agent/pkg/util/sync"
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
		model.TracerMemfdSealEventType.String(),
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
	event          *model.Event
	dnsLayer       *layers.DNS
	monitors       *EBPFMonitors
	profileManager *securityprofile.Manager
	fieldHandlers  *EBPFFieldHandlers
	eventPool      *ddsync.TypedPool[model.Event]
	numCPU         int

	ctx       context.Context
	cancelFnc context.CancelFunc
	wg        sync.WaitGroup
	ipc       ipc.Component

	// TC Classifier & raw packets
	tcRequests                chan tcClassifierRequest
	rawPacketFilterCollection *lib.Collection
	rawPacketActionCollection *lib.Collection

	// Ring
	eventStream EventStream

	// ActivityDumps section
	activityDumpHandler backend.ActivityDumpHandler

	// Approvers / discarders section
	Erpc                     *erpc.ERPC
	erpcRequest              *erpc.Request
	inodeDiscarders          *inodeDiscarders
	discarderPushedCallbacks []DiscarderPushedCallback
	kfilters                 map[eval.EventType]kfilters.KFilters

	// Approvers / discarders section
	discarderPushedCallbacksLock sync.RWMutex
	discarderRateLimiter         *rate.Limiter

	// kill action
	killListMap           *lib.Map
	supportsBPFSendSignal bool
	processKiller         *ProcessKiller

	isRuntimeDiscarded bool
	constantOffsets    *constantfetch.ConstantFetcherStatus
	runtimeCompiled    bool
	useSyscallWrapper  bool
	useFentry          bool
	useRingBuffers     bool
	useMmapableMaps    bool
	cgroup2MountPath   string

	// On demand1
	onDemandManager     *OnDemandProbesManager
	onDemandRateLimiter *rate.Limiter

	// hash action
	fileHasher *FileHasher

	// snapshot
	ruleSetVersion    uint64
	playSnapShotState *atomic.Bool

	// Setsockopt and BPF Filter
	BPFFilterTruncated *atomic.Uint64

	// raw packet filter for actions
	rawPacketActionFilters []rawpacket.Filter

	// PrCtl and name truncation
	MetricNameTruncated *atomic.Uint64
}

// GetUseRingBuffers returns p.useRingBuffers
func (p *EBPFProbe) GetUseRingBuffers() bool {
	return p.useRingBuffers
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

// selectRingBuffersMode initializes p.useRingBuffers
func (p *EBPFProbe) selectRingBuffersMode() {
	if !p.config.Probe.EventStreamUseRingBuffer {
		p.useRingBuffers = false
		return
	}

	if !p.kernelVersion.HaveRingBuffers() {
		p.useRingBuffers = false
		seclog.Warnf("ringbuffers enabled but not supported on this kernel version, falling back to perf event")
		return
	}

	p.useRingBuffers = true
}

// initCgroup2MountPath initiatlizses p.cgroup2MountPath
func (p *EBPFProbe) initCgroup2MountPath() {
	var err error
	p.cgroup2MountPath, err = utils.GetCgroup2MountPoint()
	if err != nil {
		seclog.Warnf("%v", err)
	}
	if len(p.cgroup2MountPath) == 0 {
		seclog.Debugf("cgroup v2 not found on the host")
	}
}

// GetUseFentry returns true if fentry is used
func (p *EBPFProbe) GetUseFentry() bool {
	return p.useFentry
}

var fentrySupportCache struct {
	sync.Mutex
	previousErr error
	kv          *kernel.Version
}

func isFentrySupported(kernelVersion *kernel.Version) error {
	fentrySupportCache.Lock()
	defer fentrySupportCache.Unlock()

	if fentrySupportCache.kv == kernelVersion {
		return fentrySupportCache.previousErr
	}

	err := isFentrySupportedImpl(kernelVersion)
	fentrySupportCache.kv = kernelVersion
	fentrySupportCache.previousErr = err
	return err
}

func isFentrySupportedImpl(kernelVersion *kernel.Version) error {
	if kernelVersion.Code < kernel.Kernel6_1 {
		return errors.New("fentry enabled but not fully supported on this kernel version (< 6.1)")
	}

	if !kernelVersion.HaveFentrySupport() {
		return errors.New("fentry enabled but not supported")
	}

	tailCallsBroken, err := constantfetch.AreFentryTailCallsBroken()
	if err != nil {
		return fmt.Errorf("fentry enabled but failed to verify tail call support: %w", err)
	}

	if tailCallsBroken {
		return errors.New("fentry disabled on kernels >= 6.11 (or with breaking tail calls patch backported)")
	}

	if !kernelVersion.HaveFentrySupportWithStructArgs() {
		return errors.New("fentry enabled but not supported with struct args")
	}

	if !kernelVersion.HaveFentryNoDuplicatedWeakSymbols() {
		return errors.New("fentry enabled but not supported with duplicated weak symbols")
	}

	hasPotentialFentryDeadlock, err := bugs.HasTasksRCUExitLockSymbol()
	if err != nil {
		return errors.New("fentry enabled but failed to verify kernel symbols")
	}

	if hasPotentialFentryDeadlock {
		return errors.New("fentry enabled but lock responsible for deadlock was found in kernel symbols")
	}

	return nil
}

func (p *EBPFProbe) selectFentryMode() {
	if !p.config.Probe.EventStreamUseFentry {
		p.useFentry = false
		return
	}

	if err := isFentrySupported(p.kernelVersion); err != nil {
		p.useFentry = false
		seclog.Warnf("disabling fentry and falling back to kprobe mode: %v", err)
		return
	}

	p.useFentry = true
}

func (p *EBPFProbe) isCgroupSysCtlNotSupported() bool {
	return IsCgroupSysCtlNotSupported(p.kernelVersion, p.cgroup2MountPath)
}

func (p *EBPFProbe) isNetworkNotSupported() bool {
	return IsNetworkNotSupported(p.kernelVersion)
}

func (p *EBPFProbe) isRawPacketNotSupported() bool {
	return IsRawPacketNotSupported(p.kernelVersion)
}

func (p *EBPFProbe) isNetworkFlowMonitorNotSupported() bool {
	return IsNetworkFlowMonitorNotSupported(p.kernelVersion)
}

func (p *EBPFProbe) isCapabilitiesMonitoringSupported() bool {
	return IsCapabilitiesMonitoringSupported(p.kernelVersion)
}

func (p *EBPFProbe) sanityChecks() error {
	// make sure debugfs is mounted
	if _, err := tracefs.Root(); err != nil {
		return err
	}

	if utilkernel.GetLockdownMode() == utilkernel.Confidentiality {
		return errors.New("eBPF not supported in lockdown `confidentiality` mode")
	}

	if p.config.Probe.NetworkEnabled && p.isNetworkNotSupported() {
		seclog.Warnf("the network feature of CWS isn't supported on this kernel version")
		p.config.Probe.NetworkEnabled = false
	}

	if p.config.Probe.NetworkRawPacketEnabled && p.isRawPacketNotSupported() {
		seclog.Warnf("the raw packet feature of CWS isn't supported on this kernel version")
		p.config.Probe.NetworkRawPacketEnabled = false
	}

	if p.config.Probe.NetworkRawPacketEnabled && !p.config.Probe.NetworkEnabled {
		seclog.Warnf("the raw packet feature of CWS requires event_monitoring_config.network.enabled to be true, setting event_monitoring_config.network.raw_packet.enabled to false")
		p.config.Probe.NetworkRawPacketEnabled = false
	}

	if p.config.Probe.NetworkFlowMonitorEnabled && !p.config.Probe.NetworkEnabled {
		seclog.Warnf("The network flow monitor feature of CWS requires event_monitoring_config.network.enabled to be true, setting event_monitoring_config.network.flow_monitor.enabled to false")
		p.config.Probe.NetworkFlowMonitorEnabled = false
	}

	if p.config.Probe.NetworkFlowMonitorEnabled && p.isNetworkFlowMonitorNotSupported() {
		seclog.Warnf("The network flow monitor feature of CWS requires a more recent kernel (at least 5.13) with support for the bpf_for_each_elem map helper, setting event_monitoring_config.network.flow_monitor.enabled to false")
		p.config.Probe.NetworkFlowMonitorEnabled = false
	}

	if p.config.RuntimeSecurity.IsSysctlEventEnabled() && p.isCgroupSysCtlNotSupported() {
		seclog.Warnf("The sysctl tracking feature of CWS requires a more recent kernel with support for the cgroup/sysctl program type, setting runtime_security_config.sysctl.enabled to false")
		p.config.RuntimeSecurity.SysCtlEBPFEnabled = false
	}

	if p.config.Probe.CapabilitiesMonitoringEnabled && !p.isCapabilitiesMonitoringSupported() {
		seclog.Warnf("The capabilities monitoring feature of CWS requires a more recent kernel (at least 5.17), setting event_monitoring_config.capabilities_monitoring.enabled to false")
		p.config.Probe.CapabilitiesMonitoringEnabled = false
	}

	if p.config.Probe.CapabilitiesMonitoringEnabled && p.config.Probe.CapabilitiesMonitoringPeriod < 1*time.Second {
		seclog.Warnf("The capabilities monitoring period is too short (minimum is 1 second), setting event_monitoring_config.capabilities_monitoring.period to 1 second")
		p.config.Probe.CapabilitiesMonitoringPeriod = 1 * time.Second
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
			err = multierror.Append(err, fmt.Errorf("%s doesn't seem to be a mountpoint", utilkernel.ProcFSRoot()))
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

func (p *EBPFProbe) initEBPFManager() error {
	loader := ebpf.NewProbeLoader(p.config.Probe, p.useSyscallWrapper, p.useRingBuffers, p.useFentry)
	defer loader.Close()

	bytecodeReader, runtimeCompiled, err := loader.Load()
	if err != nil {
		return err
	}
	defer bytecodeReader.Close()

	p.runtimeCompiled = runtimeCompiled

	if err := p.initManagerOptions(); err != nil {
		seclog.Warnf("managerOptions init failed: %v", err)
		return err
	}

	p.Manager.Probes = probes.AllProbes(p.useFentry, p.cgroup2MountPath)

	if err := p.Manager.InitWithOptions(bytecodeReader, p.managerOptions); err != nil {
		return fmt.Errorf("failed to init manager: %w", err)
	}

	if err := p.Manager.Start(); err != nil {
		return err
	}

	p.applyDefaultFilterPolicies()

	needRawSyscalls := p.isNeededForActivityDump(model.SyscallsEventType.String())

	if err := p.updateProbes(defaultEventTypes, needRawSyscalls); err != nil {
		return err
	}

	return nil

}

// CgroupMountIDNoFilter defines the value used to disable cgroup filter on cgroup v2
// MUST match the ebpf/c/include/hooks/cgroup.h value of CGROUP_MOUNT_ID_NO_FILTER
const CgroupMountIDNoFilter = math.MaxUint32

func (p *EBPFProbe) initCgroupMountIDFilter() error {
	// get mount id of /sys/fs/cgroup

	cgroupMountIDMap, _, err := p.Manager.GetMap("cgroup_mount_id")
	if err != nil {
		return nil
	} else if cgroupMountIDMap == nil {
		return errors.New("cgroup_mount_id map not found")
	}

	sysfs := utils.GetFSTypeFromFilePath("/sys/fs/cgroup")
	if sysfs == "" { // error
		return errors.New("failed to retrieve cgroup version")
	} else if sysfs == "cgroup2" { // cgroup v2
		return cgroupMountIDMap.Put(uint32(0), uint32(CgroupMountIDNoFilter))
	} else if sysfs == "tmpfs" { // cgroup v1
		mountID, err := utils.GetHostMountPathID("/sys/fs/cgroup/systemd")
		if err != nil {
			return err
		}
		return cgroupMountIDMap.Put(uint32(0), uint32(mountID))
	}
	return errors.New("failed to retrieve cgroup version")
}

// Init initializes the probe
func (p *EBPFProbe) Init() error {
	useSyscallWrapper, err := ebpf.IsSyscallWrapperRequired()
	if err != nil {
		return err
	}
	p.useSyscallWrapper = useSyscallWrapper

	if err := p.eventStream.Init(p.Manager, p.config.Probe); err != nil {
		return err
	}

	if err := p.initEBPFManager(); err != nil {
		if !p.useFentry || !p.config.Probe.EventStreamUseKprobeFallback {
			return err
		}

		seclog.Warnf("fentry not supported, fallback to kprobes: %v", err)
		p.useFentry = false

		if err := p.Manager.Stop(manager.CleanAll); err != nil {
			seclog.Errorf("failed to clean manager while deactivating fentry mode: %v", err)
		}

		if err = p.initEBPFManager(); err != nil {
			return err
		}
	}

	p.inodeDiscarders = newInodeDiscarders(p.Erpc, p.Resolvers.DentryResolver)

	if err := p.Resolvers.Start(p.ctx); err != nil {
		return err
	}

	err = p.monitors.Init()
	if err != nil {
		return err
	}

	p.profileManager, err = securityprofile.NewManager(p.config, p.statsdClient, p.Manager, p.Resolvers, p.kernelVersion, p.NewEvent, p.activityDumpHandler, p.ipc)
	if err != nil {
		return err
	}

	p.eventStream.SetMonitor(p.monitors.eventStreamMonitor)

	p.killListMap, err = managerhelper.Map(p.Manager, "kill_list")
	if err != nil {
		return err
	}

	p.processKiller.Start(p.ctx, &p.wg)

	if p.config.RuntimeSecurity.ActivityDumpEnabled || p.config.RuntimeSecurity.SecurityProfileEnabled {
		p.wg.Add(1)
		go func() {
			defer p.wg.Done()
			p.profileManager.Start(p.ctx)
		}()
	}

	if err := p.initCgroupMountIDFilter(); err != nil {
		seclog.Errorf("initCgroupMountIDFilter error: %s\n", err)
		return err
	}

	return nil
}

// IsRuntimeCompiled returns true if the eBPF programs where successfully runtime compiled
func (p *EBPFProbe) IsRuntimeCompiled() bool {
	return p.runtimeCompiled
}

func (p *EBPFProbe) getRawPacketMaps() (rawPacketEventMap, routerMap *lib.Map, err error) {
	rawPacketEventMap, _, err = p.Manager.GetMap("raw_packet_event")
	if err != nil {
		return nil, nil, err
	}
	if rawPacketEventMap == nil {
		return nil, nil, errors.New("unable to find `rawpacket_event` map")
	}

	routerMap, _, err = p.Manager.GetMap("raw_packet_classifier_router")
	if err != nil {
		return nil, nil, err
	}
	if routerMap == nil {
		return nil, nil, errors.New("unable to find `classifier_router` map")
	}

	return rawPacketEventMap, routerMap, nil
}

func (p *EBPFProbe) enableRawPacket(enable bool) error {
	enabledMap, _, err := p.Manager.GetMap("raw_packet_enabled")
	if err != nil {
		return err
	}
	if enabledMap == nil {
		return errors.New("unable to find `raw_packet_enabled` map")
	}

	enabledData := make([]uint32, p.numCPU)
	if enable {
		for i := range enabledData {
			enabledData[i] = 1
		}
	}
	return enabledMap.Put(uint32(0), enabledData)
}

func (p *EBPFProbe) setupRawPacketProgs(progSpecs []*lib.ProgramSpec, progKey uint32, maxProgs int, collectionPtr **lib.Collection) error {
	collection := *collectionPtr

	// unload the previews one
	if collection != nil {
		collection.Close()
		ddebpf.RemoveNameMappingsCollection(collection)
		collection = nil
	}

	if len(progSpecs) > 0 {
		if err := p.enableRawPacket(true); err != nil {
			return err
		}
	} else {
		return nil
	}

	colSpec := lib.CollectionSpec{
		Programs: make(map[string]*lib.ProgramSpec),
	}
	for _, progSpec := range progSpecs {
		colSpec.Programs[progSpec.Name] = progSpec
	}

	// verify that the programs are using the TC_ACT_UNSPEC return code
	if err := probes.CheckUnspecReturnCode(colSpec.Programs); err != nil {
		return fmt.Errorf("programs are not using the TC_ACT_UNSPEC return code: %w", err)
	}

	col, err := lib.NewCollection(&colSpec)
	if err != nil {
		return fmt.Errorf("failed to load program: %w", err)
	}
	*collectionPtr = col

	if len(progSpecs) > maxProgs {
		return fmt.Errorf("too many programs, max is %d", maxProgs)
	}

	// setup tail calls
	for i, progSpec := range progSpecs {
		if err := p.Manager.UpdateTailCallRoutes(manager.TailCallRoute{
			Program:       col.Programs[progSpec.Name],
			Key:           progKey + uint32(i),
			ProgArrayName: "raw_packet_classifier_router",
		}); err != nil {
			return err
		}
	}

	return nil
}

func (p *EBPFProbe) setupRawPacketFilters(rs *rules.RuleSet) error {
	var rawPacketFilters []rawpacket.Filter
	for id, rule := range rs.GetRules() {
		for _, field := range rule.GetFieldValues("packet.filter") {
			rawPacketFilters = append(rawPacketFilters, rawpacket.Filter{
				RuleID:    id,
				BPFFilter: field.Value.(string),
				Policy:    rawpacket.PolicyAllow,
			})
		}
	}

	opts := rawpacket.DefaultProgOpts()
	opts.WithProgPrefix("raw_packet_filter_")

	// adapt max instruction limits depending of the kernel version
	if p.kernelVersion.Code >= kernel.Kernel5_2 {
		opts.MaxProgSize = 1_000_000
	}

	seclog.Debugf("generate rawpacket filter programs with a limit of %d max instructions", opts.MaxProgSize)

	rawPacketEventMap, routerMap, err := p.getRawPacketMaps()
	if err != nil {
		return err
	}

	var progSpecs []*lib.ProgramSpec
	if len(rawPacketFilters) > 0 {
		progSpecs, err = rawpacket.FiltersToProgramSpecs(rawPacketEventMap.FD(), routerMap.FD(), rawPacketFilters, opts)
		if err != nil {
			return err
		}
	}

	// add or close if none
	return p.setupRawPacketProgs(progSpecs, probes.TCRawPacketFilterKey, probes.RawPacketMaxTailCall, &p.rawPacketFilterCollection)
}

func (p *EBPFProbe) applyRawPacketActionFilters() error {
	// TODO check cgroupv2

	opts := rawpacket.DefaultProgOpts()
	opts.WithProgPrefix("raw_packet_drop_action_")
	opts.WithGetCurrentCgroupID(p.kernelVersion.HasBpfGetCurrentPidTgidForSchedCLS())

	// adapt max instruction limits depending of the kernel version
	if p.kernelVersion.Code >= kernel.Kernel5_2 {
		opts.MaxProgSize = 1_000_000
	}

	seclog.Debugf("generate rawpacket filter programs with a limit of %d max instructions", opts.MaxProgSize)

	rawPacketEventMap, routerMap, err := p.getRawPacketMaps()
	if err != nil {
		return err
	}

	var progSpecs []*lib.ProgramSpec
	if len(p.rawPacketActionFilters) > 0 {
		progSpecs, err = rawpacket.DropActionsToProgramSpecs(rawPacketEventMap.FD(), routerMap.FD(), p.rawPacketActionFilters, opts)
		if err != nil {
			return err
		}
	}

	// add or close if none
	return p.setupRawPacketProgs(progSpecs, probes.TCRawPacketDropActionKey, probes.RawPacketMaxTailCall, &p.rawPacketActionCollection)
}

func (p *EBPFProbe) addRawPacketActionFilter(actionFilter rawpacket.Filter) error {
	seclog.Infof("add raw packet action filter: %+v\n", actionFilter)

	if slices.ContainsFunc(p.rawPacketActionFilters, func(af rawpacket.Filter) bool {
		return actionFilter.Key() == af.Key()
	}) {
		return nil
	}
	p.rawPacketActionFilters = append(p.rawPacketActionFilters, actionFilter)

	return p.applyRawPacketActionFilters()
}

// Start the probe
func (p *EBPFProbe) Start() error {
	// Apply rules to the snapshotted data before starting the event stream to avoid concurrency issues
	p.playSnapshot(true)

	// start new tc classifier loop
	go p.startSetupNewTCClassifierLoop()

	if p.config.RuntimeSecurity.IsSysctlSnapshotEnabled() {
		// start sysctl snapshot loop
		go p.startSysCtlSnapshotLoop()
	}

	return p.eventStream.Start(&p.wg)
}

// PlaySnapshot plays a snapshot
func (p *EBPFProbe) playSnapshot(notifyConsumers bool) {
	seclog.Debugf("playing the snapshot")

	var events []*model.Event

	entryToEvent := func(entry *model.ProcessCacheEntry) {
		if entry.Source != model.ProcessCacheEntryFromSnapshot {
			return
		}
		entry.Retain()

		event := p.newEBPFPooledEventFromPCE(entry)

		if _, err := entry.HasValidLineage(); err != nil {
			event.Error = &model.ErrProcessBrokenLineage{Err: err}
		}

		event.AddToFlags(model.EventFlagsIsSnapshot)

		events = append(events, event)

		snapshotBoundSockets, ok := p.Resolvers.ProcessResolver.SnapshottedBoundSockets[event.ProcessContext.Pid]
		if ok {
			for _, s := range snapshotBoundSockets {
				entry.Retain()
				bindEvent := p.newBindEventFromSnapshot(entry, s)
				events = append(events, bindEvent)
			}
		}

	}

	p.Walk(entryToEvent)

	// order events so that they're dispatched in creation time order
	sort.Slice(events, func(i, j int) bool {
		eventA := events[i]
		eventB := events[j]

		tsA := eventA.ProcessContext.ExecTime
		tsB := eventB.ProcessContext.ExecTime
		if tsA.IsZero() || tsB.IsZero() || tsA.Equal(tsB) {
			return eventA.ProcessContext.Pid < eventB.ProcessContext.Pid
		}

		return tsA.Before(tsB)
	})

	for _, event := range events {
		p.DispatchEvent(event, notifyConsumers)
		event.ProcessCacheEntry.Release()
		p.eventPool.Put(event)
	}
}

func (p *EBPFProbe) sendAnomalyDetection(event *model.Event) {
	tags := p.probe.GetEventTags(event.ProcessContext.Process.ContainerContext.ContainerID)
	if service := p.probe.GetService(event); service != "" {
		tags = append(tags, "service:"+service)
	}

	p.probe.DispatchCustomEvent(
		events.NewCustomRule(events.AnomalyDetectionRuleID, events.AnomalyDetectionRuleDesc),
		events.NewCustomEventLazy(event.GetEventType(), p.EventMarshallerCtor(event), tags...),
	)
}

// AddActivityDumpHandler set the probe activity dump handler
func (p *EBPFProbe) AddActivityDumpHandler(handler backend.ActivityDumpHandler) {
	p.activityDumpHandler = handler
}

// DispatchEvent sends an event to the probe event handler
func (p *EBPFProbe) DispatchEvent(event *model.Event, notifyConsumers bool) {
	logTraceEvent(event.GetEventType(), event)

	// filter out event if already present on a profile
	p.profileManager.LookupEventInProfiles(event)

	// mark the events that have an associated activity dump
	// this is needed for auto suppressions performed by the CWS rule engine
	if p.profileManager.HasActiveActivityDump(event) {
		event.AddToFlags(model.EventFlagsHasActiveActivityDump)
	}

	// send event to wildcard handlers, like the CWS rule engine, first
	p.probe.sendEventToHandlers(event)

	// send event to specific event handlers, like the event monitor consumers, subsequently
	if notifyConsumers {
		p.probe.sendEventToConsumers(event)
	}

	// handle anomaly detections
	if event.IsAnomalyDetectionEvent() {
		var workloadID containerutils.WorkloadID
		var imageTag string
		if containerID := event.FieldHandlers.ResolveContainerID(event, &event.ProcessContext.Process.ContainerContext); containerID != "" {
			workloadID = containerID
			imageTag = utils.GetTagValue("image_tag", event.ProcessContext.Process.ContainerContext.Tags)
		} else if cgroupID := event.FieldHandlers.ResolveCGroupID(event, &event.ProcessContext.Process.CGroup); cgroupID != "" {
			workloadID = containerutils.CGroupID(cgroupID)
			tags, err := p.Resolvers.TagsResolver.ResolveWithErr(workloadID)
			if err != nil {
				seclog.Errorf("failed to resolve tags for cgroup %s: %v", workloadID, err)
				return
			}
			imageTag = utils.GetTagValue("version", tags)
		}

		if workloadID != nil {
			p.profileManager.FillProfileContextFromWorkloadID(workloadID, &event.SecurityProfileContext, imageTag)
		}

		if p.config.RuntimeSecurity.AnomalyDetectionEnabled {
			p.sendAnomalyDetection(event)
		}
	} else if event.Error == nil {
		// Process event after evaluation because some monitors need the DentryResolver to have been called first.
		p.profileManager.ProcessEvent(event)
	}
	p.monitors.ProcessEvent(event)
}

// SendStats sends statistics about the probe to Datadog
func (p *EBPFProbe) SendStats() error {
	p.Resolvers.TCResolver.SendTCProgramsStats(p.statsdClient)

	p.processKiller.SendStats(p.statsdClient)

	if err := p.profileManager.SendStats(); err != nil {
		return err
	}

	value := p.BPFFilterTruncated.Swap(0)
	if err := p.statsdClient.Count(metrics.MetricBPFFilterTruncated, int64(value), []string{}, 1.0); err != nil {
		return err
	}

	valueNameTruncated := p.MetricNameTruncated.Swap(0)
	if err := p.statsdClient.Count(metrics.MetricNameTruncated, int64(valueNameTruncated), []string{}, 1.0); err != nil {
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
	read, err := model.UnmarshalBinary(data, &event.PIDContext, &event.SpanContext, &event.ProcessContext.Process.CGroup)
	if err != nil {
		return 0, err
	}
	return read, nil
}

func eventWithNoProcessContext(eventType model.EventType) bool {
	switch eventType {
	case model.ShortDNSResponseEventType,
		model.DNSEventType,
		model.IMDSEventType,
		model.RawPacketFilterEventType,
		model.LoadModuleEventType,
		model.UnloadModuleEventType,
		model.NetworkFlowMonitorEventType:
		return true
	default:
		return false
	}
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

	entry.Process.ContainerContext.ContainerID = ev.ProcessContext.Process.ContainerContext.ContainerID

	entry.Process.CGroup.Merge(&ev.ProcessContext.Process.CGroup)

	entry.Source = model.ProcessCacheEntryFromEvent

	return n, nil
}

func (p *EBPFProbe) onEventLost(_ string, perEvent map[string]uint64) {
	// snapshot traced cgroups if a CgroupTracing event was lost
	if p.probe.IsActivityDumpEnabled() && perEvent[model.CgroupTracingEventType.String()] > 0 {
		p.profileManager.SyncTracedCgroups()
	}
}

// setProcessContext set the process context, should return false if the event shouldn't be dispatched
func (p *EBPFProbe) setProcessContext(eventType model.EventType, event *model.Event, newEntryCb func(entry *model.ProcessCacheEntry, err error)) bool {
	entry, isResolved := p.fieldHandlers.ResolveProcessCacheEntry(event, newEntryCb)
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
	probeEventZeroer(p.event)
	p.event.FieldHandlers = p.fieldHandlers
	p.event.Origin = EBPFOrigin
	p.event.ProcessContext = &model.ProcessContext{}
	return p.event
}

func (p *EBPFProbe) resolveCGroup(pid uint32, cgroupPathKey model.PathKey, newEntryCb func(entry *model.ProcessCacheEntry, err error)) (*model.CGroupContext, error) {
	cgroupContext, _, err := p.Resolvers.ResolveCGroupContext(cgroupPathKey)
	if err != nil {
		return nil, fmt.Errorf("failed to resolve cgroup for pid %d: %w", pid, err)
	}
	updated := p.Resolvers.ProcessResolver.UpdateProcessCGroupContext(pid, cgroupContext, newEntryCb)
	if !updated {
		return nil, fmt.Errorf("failed to update cgroup for pid %d", pid)
	}

	return cgroupContext, nil
}

// BinaryUnmarshaler is the interface used to unmarshal binary data
type BinaryUnmarshaler interface {
	UnmarshalBinary(data []byte) (int, error)
}

// regularUnmarshalEvent do the regular unmarshaling common to all events.
// It returns false if an error occurs during the unmarshaling, true otherwise.
func (p *EBPFProbe) regularUnmarshalEvent(bu BinaryUnmarshaler, eventType model.EventType, offset int, dataLen uint64, data []byte) bool {
	if _, err := bu.UnmarshalBinary(data[offset:]); err != nil {
		seclog.Errorf("failed to decode %s event: %s (offset %d, len %d)", eventType.String(), err, offset, dataLen)
		return false
	}
	return true
}

// handleEvent processes raw eBPF events received from the kernel, unmarshaling and dispatching them appropriately.
func (p *EBPFProbe) handleEvent(CPU int, data []byte) {
	// handle play snapshot
	if p.playSnapShotState.Swap(false) {
		// do not notify consumers as we are replaying the snapshot after a ruleset reload
		p.playSnapshot(false)
	}

	var (
		offset        = 0
		event         = p.zeroEvent()
		dataLen       = uint64(len(data))
		relatedEvents []*model.Event
		newEntryCb    = func(entry *model.ProcessCacheEntry, err error) {
			// all Execs will be forwarded since used by AD. Forks will be forwarded all if there are consumers
			if !entry.IsExec && p.probe.eventConsumers[model.ForkEventType] == nil {
				return
			}

			relatedEvent := p.newEBPFPooledEventFromPCE(entry)

			if err != nil {
				var errResolution *path.ErrPathResolution
				if errors.As(err, &errResolution) {
					relatedEvent.SetPathResolutionError(&relatedEvent.ProcessCacheEntry.FileEvent, err)
				} else {
					return
				}
			}

			relatedEvents = append(relatedEvents, relatedEvent)
		}
	)

	read, err := event.UnmarshalBinary(data)
	if err != nil {
		seclog.Errorf("failed to decode event: %s", err)
		return
	}
	offset += read

	eventType := event.GetEventType()
	if eventType > model.MaxKernelEventType {
		p.monitors.eventStreamMonitor.CountInvalidEvent(dataLen)
		seclog.Errorf("unsupported event type %d", eventType)
		return
	}

	p.monitors.eventStreamMonitor.CountEvent(eventType, event, dataLen, CPU, !p.useRingBuffers)

	// some events don't need to be dispatched and return early after unmarshaling
	if !p.handleEarlyReturnEvents(event, offset, dataLen, data, newEntryCb) {
		return
	}
	// unmarshall contexts
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
	if !p.handleBeforeProcessContext(event, data, offset, dataLen, newEntryCb) {
		return
	}
	// resolve process context
	if !p.setProcessContext(eventType, event, newEntryCb) {
		return
	}

	// resolve the container context
	_, _ = p.fieldHandlers.ResolveContainerContext(event)

	// handle regular events
	if !p.handleRegularEvent(event, offset, dataLen, data, newEntryCb) {
		return
	}

	// send related events
	for _, relatedEvent := range relatedEvents {
		p.DispatchEvent(relatedEvent, true)
		p.eventPool.Put(relatedEvent)
	}
	relatedEvents = relatedEvents[0:0]

	p.DispatchEvent(event, true)

	if eventType == model.ExitEventType {
		p.Resolvers.ProcessResolver.DeleteEntry(event.ProcessContext.Pid, event.ResolveEventTime())
	}

	// flush pending actions
	p.processKiller.FlushPendingReports()
	p.fileHasher.FlushPendingReports()
}

// handleRegularEvent performs the standard unmarshaling process common to all events.
// It returns false if an error occurs during processing, indicating the event should be dropped.
func (p *EBPFProbe) handleRegularEvent(event *model.Event, offset int, dataLen uint64, data []byte, newEntryCb func(entry *model.ProcessCacheEntry, err error)) bool {
	var err error
	var read int
	eventType := event.GetEventType()
	switch eventType {

	case model.FileMountEventType, model.FileMoveMountEventType:
		if !p.regularUnmarshalEvent(&event.Mount, eventType, offset, dataLen, data) {
			return false
		}

		if err := p.handleNewMount(event, &event.Mount.Mount); err != nil {
			seclog.Debugf("failed to handle new mount from mount event: %s\n", err)
			return false
		}

		// TODO: this should be moved in the resolver itself in order to handle the fallbacks
		if event.Mount.GetFSType() == "nsfs" {
			nsid := uint32(event.Mount.RootPathKey.Inode)
			mountPath, _, _, err := p.Resolvers.MountResolver.ResolveMountPath(event.Mount.MountID, event.Mount.Device, event.PIDContext.Pid, event.ProcessContext.Process.ContainerContext.ContainerID)
			if err != nil {
				seclog.Debugf("failed to get mount path: %v", err)
			} else {
				mountNetNSPath := utils.NetNSPathFromPath(mountPath)
				_, _ = p.Resolvers.NamespaceResolver.SaveNetworkNamespaceHandle(nsid, mountNetNSPath)
			}
		}

	case model.FileUmountEventType:
		if !p.regularUnmarshalEvent(&event.Umount, eventType, offset, dataLen, data) {
			return false
		}

		// we can skip this error as this is for the umount only and there is no impact on the filepath resolution
		mount, _, _, _ := p.Resolvers.MountResolver.ResolveMount(event.Umount.MountID, 0, event.PIDContext.Pid, event.ProcessContext.Process.ContainerContext.ContainerID)
		if mount != nil && mount.GetFSType() == "nsfs" {
			nsid := uint32(mount.RootPathKey.Inode)
			if namespace := p.Resolvers.NamespaceResolver.ResolveNetworkNamespace(nsid); namespace != nil {
				p.FlushNetworkNamespace(namespace)
			}
		}

	case model.FileOpenEventType:
		if !p.regularUnmarshalEvent(&event.Open, eventType, offset, dataLen, data) {
			return false
		}
	case model.FileMkdirEventType:
		if !p.regularUnmarshalEvent(&event.Mkdir, eventType, offset, dataLen, data) {
			return false
		}
	case model.FileRmdirEventType:
		if !p.regularUnmarshalEvent(&event.Rmdir, eventType, offset, dataLen, data) {
			return false
		}
	case model.FileUnlinkEventType:
		if !p.regularUnmarshalEvent(&event.Unlink, eventType, offset, dataLen, data) {
			return false
		}
	case model.FileRenameEventType:
		if !p.regularUnmarshalEvent(&event.Rename, eventType, offset, dataLen, data) {
			return false
		}
	case model.FileChdirEventType:
		if !p.regularUnmarshalEvent(&event.Chdir, eventType, offset, dataLen, data) {
			return false
		}
	case model.FileChmodEventType:
		if !p.regularUnmarshalEvent(&event.Chmod, eventType, offset, dataLen, data) {
			return false
		}
	case model.FileChownEventType:
		if !p.regularUnmarshalEvent(&event.Chown, eventType, offset, dataLen, data) {
			return false
		}
	case model.FileUtimesEventType:
		if !p.regularUnmarshalEvent(&event.Utimes, eventType, offset, dataLen, data) {
			return false
		}
	case model.FileLinkEventType:
		if !p.regularUnmarshalEvent(&event.Link, eventType, offset, dataLen, data) {
			return false
		}
	case model.FileSetXAttrEventType:
		if !p.regularUnmarshalEvent(&event.SetXAttr, eventType, offset, dataLen, data) {
			return false
		}
	case model.FileRemoveXAttrEventType:
		if !p.regularUnmarshalEvent(&event.RemoveXAttr, eventType, offset, dataLen, data) {
			return false
		}
	case model.ExitEventType:
		if !p.regularUnmarshalEvent(&event.Exit, eventType, offset, dataLen, data) {
			return false
		}
		exists := p.Resolvers.ProcessResolver.ApplyExitEntry(event, newEntryCb)
		if exists {
			p.Resolvers.MountResolver.DelPid(event.Exit.Pid)
			// update action reports
			p.processKiller.HandleProcessExited(event)
			p.fileHasher.HandleProcessExited(event)
		}
	case model.SetuidEventType:
		// the process context may be incorrect, do not modify it
		if event.Error != nil {
			break
		}

		if !p.regularUnmarshalEvent(&event.SetUID, eventType, offset, dataLen, data) {
			return false
		}
		defer p.Resolvers.ProcessResolver.UpdateUID(event.PIDContext.Pid, event)
	case model.SetgidEventType:
		// the process context may be incorrect, do not modify it
		if event.Error != nil {
			break
		}

		if !p.regularUnmarshalEvent(&event.SetGID, eventType, offset, dataLen, data) {
			return false
		}
		defer p.Resolvers.ProcessResolver.UpdateGID(event.PIDContext.Pid, event)
	case model.CapsetEventType:
		// the process context may be incorrect, do not modify it
		if event.Error != nil {
			break
		}

		if !p.regularUnmarshalEvent(&event.Capset, eventType, offset, dataLen, data) {
			return false
		}
		defer p.Resolvers.ProcessResolver.UpdateCapset(event.PIDContext.Pid, event)
	case model.LoginUIDWriteEventType:
		if event.Error != nil {
			break
		}

		if !p.regularUnmarshalEvent(&event.LoginUIDWrite, eventType, offset, dataLen, data) {
			return false
		}
		defer p.Resolvers.ProcessResolver.UpdateLoginUID(event.PIDContext.Pid, event)
	case model.SELinuxEventType:
		if !p.regularUnmarshalEvent(&event.SELinux, eventType, offset, dataLen, data) {
			return false
		}
	case model.BPFEventType:
		if !p.regularUnmarshalEvent(&event.BPF, eventType, offset, dataLen, data) {
			return false
		}
	case model.PTraceEventType:
		if !p.regularUnmarshalEvent(&event.PTrace, eventType, offset, dataLen, data) {
			return false
		}
		ok := resolveTraceProcessContext(event, p, newEntryCb)
		if !ok {
			return false
		}
	case model.MMapEventType:
		if !p.regularUnmarshalEvent(&event.MMap, eventType, offset, dataLen, data) {
			return false
		}

		if event.MMap.Flags&unix.MAP_ANONYMOUS != 0 {
			// no need to trigger a dentry resolver, not backed by any file
			event.MMap.File.SetPathnameStr("")
			event.MMap.File.SetBasenameStr("")
		}
	case model.MProtectEventType:
		if !p.regularUnmarshalEvent(&event.MProtect, eventType, offset, dataLen, data) {
			return false
		}
	case model.LoadModuleEventType:
		if !p.regularUnmarshalEvent(&event.LoadModule, eventType, offset, dataLen, data) {
			return false
		}

		if event.LoadModule.LoadedFromMemory {
			// no need to trigger a dentry resolver, not backed by any file
			event.LoadModule.File.SetPathnameStr("")
			event.LoadModule.File.SetBasenameStr("")
		}
	case model.UnloadModuleEventType:
		if !p.regularUnmarshalEvent(&event.UnloadModule, eventType, offset, dataLen, data) {
			return false
		}
	case model.SignalEventType:
		if !p.regularUnmarshalEvent(&event.Signal, eventType, offset, dataLen, data) {
			return false
		}
		resolveTargetProcessContext(event, p, newEntryCb)
	case model.SpliceEventType:
		if !p.regularUnmarshalEvent(&event.Splice, eventType, offset, dataLen, data) {
			return false
		}
	case model.NetDeviceEventType:
		if !p.regularUnmarshalEvent(&event.NetDevice, eventType, offset, dataLen, data) {
			return false
		}

		request := tcClassifierRequest{
			requestType: tcNewDeviceRequestType,
			device:      event.NetDevice.Device,
		}
		p.pushNewTCClassifierRequest(request)
	case model.VethPairEventType, model.VethPairNsEventType:
		if !p.regularUnmarshalEvent(&event.VethPair, eventType, offset, dataLen, data) {
			return false
		}

		request := tcClassifierRequest{
			device: event.VethPair.PeerDevice,
		}

		if eventType == model.VethPairEventType {
			request.requestType = tcNewDeviceRequestType
		} else {
			request.requestType = tcDeviceUpdateRequestType
		}

		p.pushNewTCClassifierRequest(request)
	case model.DNSEventType:
		if read, err = event.NetworkContext.UnmarshalBinary(data[offset:]); err != nil {
			seclog.Errorf("failed to decode Network Context")
			return false
		}
		offset += read

		if _, err = event.DNS.UnmarshalBinary(data[offset:]); err != nil {
			if errors.Is(err, model.ErrDNSNameMalformatted) {
				seclog.Debugf("failed to validate DNS request event: %s", event.DNS.Question.Name)
				return false
			} else if errors.Is(err, model.ErrDNSNamePointerNotSupported) {
				seclog.Tracef("failed to decode DNS request: %s (offset %d, len %d, data %s)", err, offset, len(data), string(data[offset:]))
				return false
			}
			seclog.Warnf("failed to decode DNS request: %s", err)
			event.Error = model.ErrFailedDNSPacketDecoding
			event.FailedDNS = model.FailedDNSEvent{
				Payload: trimRightZeros(data[offset:]),
			}
		}

	case model.FullDNSResponseEventType:
		if p.config.Probe.DNSResolutionEnabled {
			if read, err = event.NetworkContext.UnmarshalBinary(data[offset:]); err != nil {
				seclog.Errorf("failed to decode Network Context")
				return false
			}
			offset += read

			if err := p.dnsLayer.DecodeFromBytes(data[offset:], gopacket.NilDecodeFeedback); err != nil {
				seclog.Warnf("failed to decode the full DNS response: %s", err)
				event.Error = model.ErrFailedDNSPacketDecoding
				event.FailedDNS = model.FailedDNSEvent{
					Payload: trimRightZeros(data[offset:]),
				}
			} else {
				p.addToDNSResolver(p.dnsLayer)
				event.Type = uint32(model.DNSEventType) // remap to regular DNS event type
				event.DNS = model.DNSEvent{
					ID: p.dnsLayer.ID,
					Response: &model.DNSResponse{
						ResponseCode: uint8(p.dnsLayer.ResponseCode),
					},
				}
				if len(p.dnsLayer.Questions) != 0 {
					event.DNS.Question = model.DNSQuestion{
						Name:  string(p.dnsLayer.Questions[0].Name),
						Class: uint16(p.dnsLayer.Questions[0].Class),
						Type:  uint16(p.dnsLayer.Questions[0].Type),
						Size:  uint16(len(data[offset:])),
					}
				}
			}
		}

	case model.IMDSEventType:
		if read, err = event.NetworkContext.UnmarshalBinary(data[offset:]); err != nil {
			seclog.Errorf("failed to decode Network Context")
			return false
		}
		offset += read

		if _, err = event.IMDS.UnmarshalBinary(data[offset:]); err != nil {
			if err != model.ErrNoUsefulData {
				// it's very possible we can't parse the IMDS body, as such let's put it as debug for now
				seclog.Debugf("failed to decode IMDS event: %s (offset %d, len %d)", err, offset, len(data))
			}
			return false
		}
		defer p.Resolvers.ProcessResolver.UpdateAWSSecurityCredentials(event.PIDContext.Pid, event)
	case model.RawPacketFilterEventType:
		if !p.regularUnmarshalEvent(&event.RawPacket, eventType, offset, dataLen, data) {
			return false
		}
		event.NetworkContext = event.RawPacket.NetworkContext
	case model.RawPacketActionEventType:
		if !p.regularUnmarshalEvent(&event.RawPacket, eventType, offset, dataLen, data) {
			return false
		}
		event.NetworkContext = event.RawPacket.NetworkContext

		tags := p.probe.GetEventTags(event.ProcessContext.Process.ContainerContext.ContainerID)
		if service := p.probe.GetService(event); service != "" {
			tags = append(tags, "service:"+service)
		}
		p.probe.DispatchCustomEvent(
			events.NewCustomRule(events.RawPacketActionRuleID, events.RulesetLoadedRuleDesc),
			events.NewCustomEventLazy(event.GetEventType(), p.EventMarshallerCtor(event), tags...),
		)
		return false
	case model.NetworkFlowMonitorEventType:
		if !p.regularUnmarshalEvent(&event.NetworkFlowMonitor, eventType, offset, dataLen, data) {
			return false
		}
	case model.AcceptEventType:
		if !p.regularUnmarshalEvent(&event.Accept, eventType, offset, dataLen, data) {
			return false
		}
	case model.BindEventType:
		if !p.regularUnmarshalEvent(&event.Bind, eventType, offset, dataLen, data) {
			return false
		}
	case model.ConnectEventType:
		if !p.regularUnmarshalEvent(&event.Connect, eventType, offset, dataLen, data) {
			return false
		}
	case model.SyscallsEventType:
		if !p.regularUnmarshalEvent(&event.Syscalls, eventType, offset, dataLen, data) {
			return false
		}
	case model.OnDemandEventType:
		if p.onDemandManager.isDisabled() {
			seclog.Debugf("on-demand event received but on-demand probes are disabled")
			return false
		}

		if !p.onDemandRateLimiter.Allow() {
			seclog.Errorf("on-demand event rate limit reached, disabling on-demand probes to protect the system")
			p.onDemandManager.disable()
			return false
		}

		if !p.regularUnmarshalEvent(&event.OnDemand, eventType, offset, dataLen, data) {
			return false
		}

	case model.SysCtlEventType:
		if !p.regularUnmarshalEvent(&event.SysCtl, eventType, offset, dataLen, data) {
			return false
		}

	case model.SetSockOptEventType:
		if !p.regularUnmarshalEvent(&event.SetSockOpt, eventType, offset, dataLen, data) {
			return false
		}
		if event.SetSockOpt.IsFilterTruncated {
			p.BPFFilterTruncated.Add(1)
		}

	case model.SocketEventType:
		if !p.regularUnmarshalEvent(&event.Socket, eventType, offset, dataLen, data) {
			return false
		}

	case model.SetrlimitEventType:
		if !p.regularUnmarshalEvent(&event.Setrlimit, eventType, offset, dataLen, data) {
			return false
		}
		// resolve target process context
		var pce *model.ProcessCacheEntry
		if event.Setrlimit.TargetPid > 0 {
			pce = p.Resolvers.ProcessResolver.Resolve(event.Setrlimit.TargetPid, event.Setrlimit.TargetPid, 0, false, newEntryCb)
		}
		if pce == nil {
			pce = model.NewPlaceholderProcessCacheEntry(event.Setrlimit.TargetPid, event.Setrlimit.TargetPid, false)
		}
		event.Setrlimit.Target = &pce.ProcessContext
	case model.CapabilitiesEventType:
		if !p.regularUnmarshalEvent(&event.CapabilitiesUsage, eventType, offset, dataLen, data) {
			return false
		}
		if event.CapabilitiesUsage.Attempted == 0 && event.CapabilitiesUsage.Used == 0 {
			seclog.Debugf("capabilities usage event with no attempted or used capabilities, skipping")
			return false
		}
		// is this thread-safe?
		event.ProcessCacheEntry.CapsAttempted |= event.CapabilitiesUsage.Attempted
		event.ProcessCacheEntry.CapsUsed |= event.CapabilitiesUsage.Used
	case model.PrCtlEventType:
		if !p.regularUnmarshalEvent(&event.PrCtl, eventType, offset, dataLen, data) {
			return false
		}
		if event.PrCtl.IsNameTruncated {
			p.MetricNameTruncated.Add(1)
		}
	case model.TracerMemfdSealEventType:
		if !p.regularUnmarshalEvent(&event.TracerMemfdSeal, eventType, offset, dataLen, data) {
			return false
		}
		if err := p.Resolvers.ProcessResolver.AddTracerMetadata(event.PIDContext.Pid, event); err != nil {
			seclog.Debugf("failed to add tracer metadata: %s (pid %d, fd %d)", err, event.PIDContext.Pid, event.TracerMemfdSeal.Fd)
		}
		// Second handle for exec event because the context is required to detect a ssh session
	case model.ExecEventType:
		p.HandleSSHUserSession(event)

	case model.ForkEventType:
		p.HandleSSHUserSession(event)
	}
	return true
}

// handleBeforeProcessContext unmarshals and populates the process cache entry for fork and exec events before setting the process context.
// It returns false if the event should be dropped due to processing errors.
func (p *EBPFProbe) handleBeforeProcessContext(event *model.Event, data []byte, offset int, dataLen uint64, newEntryCb func(entry *model.ProcessCacheEntry, err error)) bool {
	var err error
	eventType := event.GetEventType()
	switch eventType {
	case model.ForkEventType:
		if _, err = p.unmarshalProcessCacheEntry(event, data[offset:]); err != nil {
			seclog.Errorf("failed to decode fork event: %s (offset %d, len %d)", err, offset, dataLen)
			return false
		}

		if err := p.Resolvers.ProcessResolver.AddForkEntry(event, newEntryCb); err != nil {
			seclog.Errorf("failed to insert fork event: %s (pid %d, offset %d, len %d)", err, event.PIDContext.Pid, offset, len(data))
			return false
		}
	case model.ExecEventType:
		// unmarshal and fill event.processCacheEntry
		if _, err = p.unmarshalProcessCacheEntry(event, data[offset:]); err != nil {
			seclog.Errorf("failed to decode exec event: %s (offset %d, len %d)", err, offset, len(data))
			return false
		}

		err = p.Resolvers.ProcessResolver.AddExecEntry(event)
		if err != nil {
			seclog.Errorf("failed to insert exec event: %s (pid %d, offset %d, len %d)", err, event.PIDContext.Pid, offset, len(data))
			return false
		}
	}
	return true
}

// handleEarlyReturnEvents processes events that may require early termination of the event handling pipeline.
// It returns false if an error occurs or if the event should not be dispatched further, true otherwise
func (p *EBPFProbe) handleEarlyReturnEvents(event *model.Event, offset int, dataLen uint64, data []byte, newEntryCb func(entry *model.ProcessCacheEntry, err error)) bool {
	var err error
	eventType := event.GetEventType()
	switch eventType {
	case model.MountReleasedEventType:
		if !p.regularUnmarshalEvent(&event.MountReleased, eventType, offset, dataLen, data) {
			return false
		}

		// Remove all dentry entries belonging to the mountID
		p.Resolvers.DentryResolver.DelCacheEntries(event.MountReleased.MountID)

		// Delete new mount point from cache
		if err = p.Resolvers.MountResolver.Delete(event.MountReleased.MountID); err != nil {
			seclog.Tracef("failed to delete mount point %d from cache: %s", event.MountReleased.MountID, err)
		}
		return false
	case model.ArgsEnvsEventType:
		if !p.regularUnmarshalEvent(&event.ArgsEnvs, eventType, offset, dataLen, data) {
			return false
		}
		p.Resolvers.ProcessResolver.UpdateArgsEnvs(&event.ArgsEnvs)
		return false
	case model.CgroupTracingEventType:
		if !p.config.RuntimeSecurity.ActivityDumpEnabled {
			seclog.Errorf("shouldn't receive Cgroup event if activity dumps are disabled")
			return false
		}
		if !p.regularUnmarshalEvent(&event.CgroupTracing, eventType, offset, dataLen, data) {
			return false
		}
		if cgroupContext, err := p.resolveCGroup(event.CgroupTracing.Pid, event.CgroupTracing.CGroupContext.CGroupFile, newEntryCb); err != nil {
			seclog.Debugf("Failed to resolve cgroup: %s", err.Error())
		} else {
			event.CgroupTracing.CGroupContext = *cgroupContext
			event.ProcessContext.Process.CGroup = *cgroupContext
			containerID := containerutils.FindContainerID(cgroupContext.CGroupID)
			if containerID != "" {
				event.CgroupTracing.ContainerContext.ContainerID = containerID
			}

			p.profileManager.HandleCGroupTracingEvent(&event.CgroupTracing)
		}
		return false
	case model.CgroupWriteEventType:
		if _, err = event.CgroupWrite.UnmarshalBinary(data[offset:]); err != nil {
			seclog.Errorf("failed to decode cgroup write released event: %s (offset %d, len %d)", err, offset, dataLen)
			return false
		}
		if _, err := p.resolveCGroup(event.CgroupWrite.Pid, event.CgroupWrite.File.PathKey, newEntryCb); err != nil {
			seclog.Debugf("Failed to resolve cgroup: %s", err.Error())
		}
		return false
	case model.UnshareMountNsEventType:
		if _, err = event.UnshareMountNS.UnmarshalBinary(data[offset:]); err != nil {
			seclog.Errorf("failed to decode unshare mnt ns event: %s (offset %d, len %d)", err, offset, dataLen)
			return false
		}
		if err := p.handleNewMount(event, &event.UnshareMountNS.Mount); err != nil {
			seclog.Debugf("failed to handle new mount from unshare mnt ns event: %s", err)
		}
		return false
	case model.ShortDNSResponseEventType:
		if p.config.Probe.DNSResolutionEnabled {
			if err := p.dnsLayer.DecodeFromBytes(data[offset:], gopacket.NilDecodeFeedback); err == nil {
				p.addToDNSResolver(p.dnsLayer)
				return false
			}

			seclog.Warnf("failed to decode the short DNS response: %s", err)
			event.Error = model.ErrFailedDNSPacketDecoding
			event.FailedDNS = model.FailedDNSEvent{
				Payload: trimRightZeros(data[offset:]),
			}
		}
	}
	return true
}

// resolveTraceProcessContext resolves the process context of a ptrace event.
// It returns false if an error occurs, true otherwise.
func resolveTraceProcessContext(event *model.Event, p *EBPFProbe, newEntryCb func(entry *model.ProcessCacheEntry, err error)) bool {
	var pce *model.ProcessCacheEntry
	if event.PTrace.Request == unix.PTRACE_TRACEME { // pid can be 0 for a PTRACE_TRACEME request
		pce = newPlaceholderProcessCacheEntryPTraceMe()
	} else if event.PTrace.PID == 0 && event.PTrace.NSPID == 0 {
		seclog.Errorf("ptrace event without any PID to resolve")
		return false
	} else {
		pidToResolve := event.PTrace.PID

		if pidToResolve == 0 { // resolve the PID given as argument instead
			containerID := p.fieldHandlers.ResolveContainerID(event, &event.ProcessContext.Process.ContainerContext)
			if containerID == "" && event.PTrace.Request != unix.PTRACE_ATTACH {
				pidToResolve = event.PTrace.NSPID
			} else {
				nsid, err := p.fieldHandlers.ResolveProcessNSID(event)
				if err != nil {
					seclog.Debugf("PTrace NSID resolution error for process %s in container %s: %v",
						event.ProcessContext.Process.FileEvent.PathnameStr, containerID, err)
					return false
				}

				pid, err := utils.TryToResolveTraceePid(event.ProcessContext.Process.Pid, nsid, event.PTrace.NSPID)
				if err != nil {
					seclog.Debugf("PTrace tracee resolution error for process %s in container %s: %v",
						event.ProcessContext.Process.FileEvent.PathnameStr, containerID, err)
					return false
				}
				pidToResolve = pid
			}
		}

		pce = p.Resolvers.ProcessResolver.Resolve(pidToResolve, pidToResolve, 0, false, newEntryCb)
		if pce == nil {
			pce = model.NewPlaceholderProcessCacheEntry(pidToResolve, pidToResolve, false)
		}
	}
	event.PTrace.Tracee = &pce.ProcessContext
	return true
}

func resolveTargetProcessContext(event *model.Event, p *EBPFProbe, newEntryCb func(entry *model.ProcessCacheEntry, err error)) {
	var pce *model.ProcessCacheEntry
	if event.Signal.PID > 0 { // Linux accepts a kill syscall with both negative and zero pid
		pce = p.Resolvers.ProcessResolver.Resolve(event.Signal.PID, event.Signal.PID, 0, false, newEntryCb)
	}
	if pce == nil {
		pce = model.NewPlaceholderProcessCacheEntry(event.Signal.PID, event.Signal.PID, false)
	}
	event.Signal.Target = &pce.ProcessContext
}

// AddDiscarderPushedCallback add a callback to the list of func that have to be called when a discarder is pushed to kernel
func (p *EBPFProbe) AddDiscarderPushedCallback(cb DiscarderPushedCallback) {
	p.discarderPushedCallbacksLock.Lock()
	defer p.discarderPushedCallbacksLock.Unlock()

	p.discarderPushedCallbacks = append(p.discarderPushedCallbacks, cb)
}

// GetEventTags returns the event tags
func (p *EBPFProbe) GetEventTags(containerID containerutils.ContainerID) []string {
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

	if handler, ok := allDiscarderHandlers[field]; ok {
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

type filterPolicyBlock struct {
	eventTypes []ebpf.Uint32MapItem
	policies   ebpf.SliceBinaryMarshaller[*kfilters.FilterPolicy]
}

func newFilterPolicyBlock() *filterPolicyBlock {
	return &filterPolicyBlock{}
}

func (b *filterPolicyBlock) add(eventType eval.EventType, mode kfilters.PolicyMode) error {
	et, err := model.ParseEvalEventType(eventType)
	if err != nil {
		return err
	}

	b.addRaw(et, mode)
	return nil
}

func (b *filterPolicyBlock) addRaw(eventType model.EventType, mode kfilters.PolicyMode) {
	b.eventTypes = append(b.eventTypes, ebpf.Uint32MapItem(eventType))
	policy := &kfilters.FilterPolicy{Mode: mode}
	b.policies = append(b.policies, policy)
}

func (b *filterPolicyBlock) apply(m *manager.Manager) error {
	// log part
	var sb strings.Builder
	for i := range b.eventTypes {
		if i > 0 {
			sb.WriteString(", ")
		}
		sb.WriteString(fmt.Sprintf("`%s`: `%s`", model.EventType(b.eventTypes[i]), b.policies[i].Mode))
	}
	seclog.Infof("Setting in-kernel filter policy to [%s]", sb.String())

	table, err := managerhelper.Map(m, "filter_policy")
	if err != nil {
		return fmt.Errorf("unable to find policy table: %w", err)
	}

	if ebpf.BatchAPISupported() {
		_, err := table.BatchUpdate(b.eventTypes, b.policies, &lib.BatchOptions{ElemFlags: uint64(lib.UpdateAny)})
		return err
	}

	for i := range b.eventTypes {
		if err := table.Put(b.eventTypes[i], b.policies[i]); err != nil {
			return fmt.Errorf("unable to set policy for event type `%s`: %w", model.EventType(b.eventTypes[i]), err)
		}
	}

	return nil
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
		for _, e := range p.config.RuntimeSecurity.ActivityDumpTracedEventTypes {
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
	case model.DNSEventType.String():
		return p.probe.IsNetworkEnabled()
	case model.IMDSEventType.String():
		return p.probe.IsNetworkEnabled()
	case model.RawPacketFilterEventType.String():
		return p.probe.IsNetworkRawPacketEnabled()
	case model.RawPacketActionEventType.String():
		return p.probe.IsNetworkRawPacketEnabled()
	case model.NetworkFlowMonitorEventType.String():
		return p.probe.IsNetworkFlowMonitorEnabled()
	case model.SyscallsEventType.String():
		return p.config.RuntimeSecurity.IsSysctlEventEnabled()
	}
	return true
}

// updateProbes applies the loaded set of rules and returns a report
// of the applied approvers for it.
func (p *EBPFProbe) updateProbes(ruleSetEventTypes []eval.EventType, needRawSyscalls bool) error {
	// event types enabled either by event handlers or by rules
	requestedEventTypes := append([]eval.EventType{}, defaultEventTypes...)
	requestedEventTypes = append(requestedEventTypes, ruleSetEventTypes...)
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

	activatedProbes := probes.SnapshotSelectors(p.useFentry)

	if p.config.Probe.CapabilitiesMonitoringEnabled {
		activatedProbes = append(activatedProbes, probes.GetCapabilitiesMonitoringSelectors()...)
	}

	// extract probe to activate per the event types
	for eventType, selectors := range probes.GetSelectorsPerEventType(p.useFentry, p.kernelVersion.HasBpfGetSocketCookieForCgroupSocket()) {
		if (eventType == "*" || slices.Contains(requestedEventTypes, eventType) ||
			p.isNeededForActivityDump(eventType) ||
			p.isNeededForSecurityProfile(eventType) ||
			p.config.Probe.EnableAllProbes) && p.validEventTypeForConfig(eventType) {
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
		activatedProbes = append(activatedProbes, probes.SyscallMonitorSelectors()...)
	} else {
		// ActivityDumps
		if p.config.RuntimeSecurity.ActivityDumpEnabled {
			for _, e := range p.config.RuntimeSecurity.ActivityDumpTracedEventTypes {
				if e == model.SyscallsEventType {
					activatedProbes = append(activatedProbes, probes.SyscallMonitorSelectors()...)
					break
				}
			}
		}
		// SecurityProfiles
		if p.config.RuntimeSecurity.AnomalyDetectionEnabled {
			for _, e := range p.config.RuntimeSecurity.AnomalyDetectionEventTypes {
				if e == model.SyscallsEventType {
					activatedProbes = append(activatedProbes, probes.SyscallMonitorSelectors()...)
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
			eventType, err := model.ParseEvalEventType(eventName)
			if err != nil {
				return err
			}
			enabledEvents |= 1 << (eventType - 1)
		}
	}

	if err := enabledEventsMap.Put(ebpf.ZeroUint32MapItem, enabledEvents); err != nil {
		return fmt.Errorf("failed to set enabled events: %w", err)
	}

	if err = p.Manager.UpdateActivatedProbes(activatedProbes); err != nil {
		return err
	}

	p.updateEBPFCheckMapping()
	return nil
}

func (p *EBPFProbe) updateEBPFCheckMapping() {
	ddebpf.ClearProgramIDMappings("cws")
	ddebpf.AddNameMappings(p.Manager, "cws")
	ddebpf.AddProbeFDMappings(p.Manager)
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
func (p *EBPFProbe) RefreshUserCache(containerID containerutils.ContainerID) error {
	return p.Resolvers.UserGroupResolver.RefreshCache(containerID)
}

// Snapshot runs the different snapshot functions of the resolvers that
// require to sync with the current state of the system
func (p *EBPFProbe) Snapshot() error {
	return p.Resolvers.Snapshot()
}

// Walk iterates through the entire tree and call the provided callback on each entry
func (p *EBPFProbe) Walk(callback func(*model.ProcessCacheEntry)) {
	p.Resolvers.ProcessResolver.Walk(callback)
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

	// we wait until both the reorderer and the monitor are stopped
	p.wg.Wait()

	if p.rawPacketFilterCollection != nil {
		p.rawPacketFilterCollection.Close()
	}

	if p.rawPacketActionCollection != nil {
		p.rawPacketActionCollection.Close()
	}

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
	close(p.tcRequests)
	return p.Resolvers.Close()
}

func (p *EBPFProbe) startSysCtlSnapshotLoop() {
	ticker := time.NewTicker(p.config.RuntimeSecurity.SysCtlSnapshotPeriod)
	defer ticker.Stop()
	for {
		select {
		case <-p.ctx.Done():
			return
		case <-ticker.C:
			// create the sysctl snapshot
			event, err := sysctl.NewSnapshotEvent(p.config.RuntimeSecurity.SysCtlSnapshotIgnoredBaseNames, p.config.RuntimeSecurity.SysCtlSnapshotKernelCompilationFlags)
			if err != nil {
				seclog.Warnf("sysctl snapshot failed: %v", err)
				continue
			}

			// send a sysctl snapshot event
			rule := events.NewCustomRule(events.SysCtlSnapshotRuleID, events.SysCtlSnapshotRuleDesc)
			customEvent := events.NewCustomEvent(model.CustomEventType, event)

			p.probe.DispatchCustomEvent(rule, customEvent)
			seclog.Tracef("sysctl snapshot sent !")
		}
	}
}

// QueuedNetworkDeviceError is used to indicate that the new network device was queued until its namespace handle is
// resolved.
type QueuedNetworkDeviceError struct {
	msg string
}

func (err QueuedNetworkDeviceError) Error() string {
	return err.msg
}

type tcClassifierRequestType int

const (
	tcNewDeviceRequestType tcClassifierRequestType = iota
	tcDeviceUpdateRequestType
)

type tcClassifierRequest struct {
	requestType tcClassifierRequestType
	device      model.NetDevice
}

func (p *EBPFProbe) pushNewTCClassifierRequest(request tcClassifierRequest) {
	select {
	case <-p.ctx.Done():
		// the probe is stopping, do not push the new tc classifier request
		return
	case p.tcRequests <- request:
		// do nothing
	default:
		seclog.Errorf("failed to slot new tc classifier request: %+v", request)
	}
}

func (p *EBPFProbe) startSetupNewTCClassifierLoop() {
	for {
		select {
		case <-p.ctx.Done():
			return
		case request, ok := <-p.tcRequests:
			if !ok {
				return
			}

			if err := p.setupNewTCClassifier(request.device); err != nil {
				var qnde QueuedNetworkDeviceError
				var linkNotFound netlink.LinkNotFoundError

				if errors.As(err, &qnde) {
					seclog.Debugf("%v", err)
				} else if errors.As(err, &linkNotFound) {
					seclog.Debugf("link not found while setting up new tc classifier: %v", err)
				} else if errors.Is(err, manager.ErrIdentificationPairInUse) {
					if request.requestType != tcDeviceUpdateRequestType {
						seclog.Errorf("tc classifier already exists: %v", err)
					} else {
						seclog.Debugf("tc classifier already exists: %v", err)
					}
				} else {
					seclog.Errorf("error setting up new tc classifier on %+v: %v", request.device, err)
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
	defer handle.Close()
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

	if !m.Detached && ev.GetEventType() != model.FileMoveMountEventType {
		// Resolve mount point
		if err := p.Resolvers.PathResolver.SetMountPoint(ev, m); err != nil {
			return fmt.Errorf("failed to set mount point: %w", err)
		}

		// Resolve root
		if err := p.Resolvers.PathResolver.SetMountRoot(ev, m); err != nil {
			return fmt.Errorf("failed to set mount root: %w", err)
		}
	}

	var err error
	if ev.GetEventType() == model.FileMoveMountEventType {
		err = p.Resolvers.MountResolver.InsertMoved(*m)
	} else {
		err = p.Resolvers.MountResolver.Insert(*m, 0)
	}

	if err != nil {
		return fmt.Errorf("failed to insert mount event: %w", err)
	}

	return nil
}

func (p *EBPFProbe) applyDefaultFilterPolicies() {
	if !p.config.Probe.EnableKernelFilters {
		seclog.Warnf("Forcing in-kernel filter policy to `pass`: filtering not enabled")
	}

	fpb := newFilterPolicyBlock()

	for eventType := model.FirstEventType; eventType <= model.LastEventType; eventType++ {
		var mode kfilters.PolicyMode

		if !p.config.Probe.EnableKernelFilters {
			mode = kfilters.PolicyModeNoFilter
		} else if len(p.probe.eventHandlers[eventType]) > 0 {
			mode = kfilters.PolicyModeAccept
		} else {
			mode = kfilters.PolicyModeDeny
		}

		fpb.addRaw(eventType, mode)
	}

	if err := fpb.apply(p.Manager); err != nil {
		seclog.Debugf("unable to apply to filter policy: %v", err)
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

func isRawPacketActionPresent(rs *rules.RuleSet) bool {
	for _, rule := range rs.GetRules() {
		for _, action := range rule.Def.Actions {
			if action.NetworkFilter != nil {
				return true
			}
		}
	}
	return false
}

// ApplyRuleSet apply the required update to handle the new ruleset
func (p *EBPFProbe) ApplyRuleSet(rs *rules.RuleSet) (*kfilters.FilterReport, error) {
	if p.opts.SyscallsMonitorEnabled {
		if err := p.monitors.syscallsMonitor.Disable(); err != nil {
			return nil, err
		}
	}

	filterReport, err := kfilters.ComputeFilters(p.config.Probe, rs)
	if err != nil {
		return nil, err
	}

	if err := applyDNSDefaultDropMaskFromRules(p.Manager, rs); err != nil {
		seclog.Warnf("failed to apply DNS default-drop mask: %v", err)
	}

	fpb := newFilterPolicyBlock()

	for eventType, report := range filterReport.ApproverReports {
		if err := p.setApprovers(eventType, report.Approvers); err != nil {
			seclog.Errorf("Error while adding approvers fallback in-kernel policy to `%s` for `%s`: %s", kfilters.PolicyModeAccept, eventType, err)
			if err := fpb.add(eventType, kfilters.PolicyModeAccept); err != nil {
				return nil, err
			}
		} else {
			if err := fpb.add(eventType, report.Mode); err != nil {
				return nil, err
			}
		}
	}
	if err := fpb.apply(p.Manager); err != nil {
		return nil, fmt.Errorf("unable to apply to filter policy: %w", err)
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
		hookPoints, err := rs.GetOnDemandHookPoints()
		if err != nil {
			seclog.Errorf("failed to get on-demand hook points from ruleset: %v", err)
		}
		p.onDemandManager.setHookPoints(hookPoints)
	}

	// check if there is a network packet action
	if isRawPacketActionPresent(rs) && p.config.RuntimeSecurity.EnforcementEnabled {
		if !slices.Contains(eventTypes, model.RawPacketFilterEventType.String()) {
			eventTypes = append(eventTypes, model.RawPacketFilterEventType.String())
		}
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

	if p.probe.IsNetworkRawPacketEnabled() {
		// disable first, and let the following code enable it if needed
		if err := p.enableRawPacket(false); err != nil {
			seclog.Errorf("unable to disable raw packet filter programs: %v", err)
		}

		if err := p.setupRawPacketFilters(rs); err != nil {
			seclog.Errorf("unable to load raw packet filter programs: %v", err)
		}

		// reset action filter
		if p.config.RuntimeSecurity.EnforcementEnabled {
			p.rawPacketActionFilters = p.rawPacketActionFilters[0:0]
			if err := p.applyRawPacketActionFilters(); err != nil {
				seclog.Errorf("unable to load raw packet action programs: %v", err)
			}
		}
	}

	// do not replay the snapshot if we are in the first rule set version, this was already done in the start method
	if p.ruleSetVersion != 0 {
		p.playSnapShotState.Store(true)
	}

	p.ruleSetVersion++

	return filterReport, nil
}

// OnNewRuleSetLoaded resets statistics and states once a new rule set is loaded
func (p *EBPFProbe) OnNewRuleSetLoaded(rs *rules.RuleSet) {
	p.processKiller.Reset(rs)
}

// NewEvent returns a new event
func (p *EBPFProbe) NewEvent() *model.Event {
	return newEBPFEvent(p.fieldHandlers)
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

// initManagerOptionsTailCalls initializes the eBPF manager tail calls
func (p *EBPFProbe) initManagerOptionsTailCalls() {
	p.managerOptions.TailCallRouter = probes.AllTailRoutes(
		p.config.Probe.ERPCDentryResolutionEnabled,
		p.config.Probe.NetworkEnabled,
		p.config.Probe.NetworkFlowMonitorEnabled,
		p.config.Probe.NetworkRawPacketEnabled,
		p.useMmapableMaps,
	)
}

// initManagerOptionsConstants initiatilizes the eBPF manager constants
func (p *EBPFProbe) initManagerOptionsConstants() {
	areCGroupADsEnabled := p.config.RuntimeSecurity.ActivityDumpTracedCgroupsCount > 0

	// Add global constant editors
	p.managerOptions.ConstantEditors = append(p.managerOptions.ConstantEditors, constantfetch.CreateConstantEditors(p.constantOffsets)...)
	p.managerOptions.ConstantEditors = append(p.managerOptions.ConstantEditors, DiscarderConstants...)
	p.managerOptions.ConstantEditors = append(p.managerOptions.ConstantEditors, getCGroupWriteConstants())
	p.managerOptions.ConstantEditors = append(p.managerOptions.ConstantEditors,
		manager.ConstantEditor{
			Name:  constantfetch.OffsetNameSchedProcessForkChildPid,
			Value: constantfetch.ReadTracepointFieldOffsetWithFallback("sched/sched_process_fork", "child_pid", 44),
		},
		manager.ConstantEditor{
			Name:  constantfetch.OffsetNameSchedProcessForkParentPid,
			Value: constantfetch.ReadTracepointFieldOffsetWithFallback("sched/sched_process_fork", "parent_pid", 24),
		},
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
			Value: utils.BoolTouint64(p.config.RuntimeSecurity.ActivityDumpEnabled && areCGroupADsEnabled),
		},
		manager.ConstantEditor{
			Name:  "net_struct_type",
			Value: getNetStructType(p.kernelVersion),
		},
		manager.ConstantEditor{
			Name:  "syscall_monitor_event_period",
			Value: uint64(p.config.RuntimeSecurity.ActivityDumpSyscallMonitorPeriod.Nanoseconds()),
		},
		manager.ConstantEditor{
			Name:  "network_monitor_period",
			Value: uint64(p.config.Probe.NetworkFlowMonitorPeriod.Nanoseconds()),
		},
		manager.ConstantEditor{
			Name:  "is_sk_storage_supported",
			Value: utils.BoolTouint64(p.isSKStorageSupported()),
		},
		manager.ConstantEditor{
			Name:  "is_network_flow_monitor_enabled",
			Value: utils.BoolTouint64(p.config.Probe.NetworkFlowMonitorEnabled),
		},
		manager.ConstantEditor{
			Name:  "send_signal",
			Value: utils.BoolTouint64(p.kernelVersion.SupportBPFSendSignal()),
		},
		manager.ConstantEditor{
			Name:  "anomaly_syscalls",
			Value: utils.BoolTouint64(slices.Contains(p.config.RuntimeSecurity.AnomalyDetectionEventTypes, model.SyscallsEventType)),
		},
		manager.ConstantEditor{
			Name:  "monitor_syscalls_map_enabled",
			Value: utils.BoolTouint64(p.probe.Opts.SyscallsMonitorEnabled),
		},
		manager.ConstantEditor{
			Name:  "imds_ip",
			Value: uint64(p.config.RuntimeSecurity.IMDSIPv4),
		},
		manager.ConstantEditor{
			Name:  "dns_port",
			Value: uint64(utils.HostToNetworkShort(p.probe.Opts.DNSPort)),
		},
		manager.ConstantEditor{
			Name:  "use_ring_buffer",
			Value: utils.BoolTouint64(p.useRingBuffers),
		},
		manager.ConstantEditor{
			Name: "fentry_func_argc",
			ValueCallback: func(prog *lib.ProgramSpec) interface{} {
				// use a separate function to make sure we always return a uint64
				return getFuncArgCount(prog)
			},
		},
		manager.ConstantEditor{
			Name:  "tracing_helpers_in_cgroup_sysctl",
			Value: utils.BoolTouint64(p.kernelVersion.HasTracingHelpersInCgroupSysctlPrograms()),
		},
		manager.ConstantEditor{
			Name:  "raw_packet_limiter_rate",
			Value: uint64(p.config.Probe.NetworkRawPacketLimiterRate),
		},
		manager.ConstantEditor{
			Name:  "raw_packet_filter",
			Value: utils.BoolTouint64(p.config.Probe.NetworkRawPacketFilter != "none"),
		},
		manager.ConstantEditor{
			Name:  "sched_cls_has_current_pid_tgid_helper",
			Value: utils.BoolTouint64(p.kernelVersion.HasBpfGetCurrentPidTgidForSchedCLS()),
		},
		manager.ConstantEditor{
			Name:  "sched_cls_has_current_cgroup_id_helper",
			Value: utils.BoolTouint64(p.kernelVersion.HasBpfGetCurrentCgroupIDForSchedCLS()),
		},
		manager.ConstantEditor{
			Name:  "capabilities_monitoring_enabled",
			Value: utils.BoolTouint64(p.config.Probe.CapabilitiesMonitoringEnabled),
		},
		manager.ConstantEditor{
			Name:  "capabilities_monitoring_period",
			Value: uint64(p.config.Probe.CapabilitiesMonitoringPeriod.Nanoseconds()),
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

	// mostly used for testing purpose
	if p.isRuntimeDiscarded {
		p.managerOptions.ConstantEditors = append(p.managerOptions.ConstantEditors, manager.ConstantEditor{
			Name:  "runtime_discarded",
			Value: uint64(1),
		})
	}
}

func (p *EBPFProbe) isSKStorageSupported() bool {
	if !p.config.Probe.NetworkFlowMonitorSKStorageEnabled {
		return false
	}

	// BPF_SK_STORAGE is not supported for kprobes
	if !p.useFentry {
		return false
	}

	return p.kernelVersion.HasSKStorageInTracingPrograms()
}

// initManagerOptionsMaps initializes the eBPF manager map spec editors and map reader startup
func (p *EBPFProbe) initManagerOptionsMapSpecEditors() {
	opts := probes.MapSpecEditorOpts{
		TracedCgroupSize:              p.config.RuntimeSecurity.ActivityDumpTracedCgroupsCount,
		UseRingBuffers:                p.useRingBuffers,
		UseMmapableMaps:               p.useMmapableMaps,
		RingBufferSize:                uint32(p.config.Probe.EventStreamBufferSize),
		PathResolutionEnabled:         p.probe.Opts.PathResolutionEnabled,
		SecurityProfileMaxCount:       p.config.RuntimeSecurity.SecurityProfileMaxCount,
		NetworkFlowMonitorEnabled:     p.config.Probe.NetworkFlowMonitorEnabled,
		NetworkSkStorageEnabled:       p.isSKStorageSupported(),
		SpanTrackMaxCount:             1,
		CapabilitiesMonitoringEnabled: p.config.Probe.CapabilitiesMonitoringEnabled,
		CgroupSocketEnabled:           p.kernelVersion.HasBpfGetSocketCookieForCgroupSocket(),
		SecurityProfileSyscallAnomaly: slices.Contains(p.config.RuntimeSecurity.AnomalyDetectionEventTypes, model.SyscallsEventType),
	}

	if p.config.Probe.SpanTrackingEnabled {
		opts.SpanTrackMaxCount = p.config.Probe.SpanTrackingCacheSize
	}

	p.managerOptions.MapSpecEditors = probes.AllMapSpecEditors(p.numCPU, opts, p.kernelVersion)

	if p.useRingBuffers {
		p.managerOptions.SkipRingbufferReaderStartup = map[string]bool{
			eventstream.EventStreamMap: true,
		}
	} else {
		p.managerOptions.SkipPerfMapReaderStartup = map[string]bool{
			eventstream.EventStreamMap: true,
		}
	}
}

// initManagerOptionsExcludedFunctions initializes the excluded functions of the eBPF manager
func (p *EBPFProbe) initManagerOptionsExcludedFunctions() error {
	if !p.config.Probe.ERPCDentryResolutionEnabled || p.useMmapableMaps {
		// exclude the programs that use the bpf_probe_write_user helper
		p.managerOptions.ExcludedFunctions = probes.AllBPFProbeWriteUserProgramFunctions()
	}

	// prevent some TC classifiers from loading
	if !p.config.Probe.NetworkEnabled {
		p.managerOptions.ExcludedFunctions = append(p.managerOptions.ExcludedFunctions, probes.GetAllTCProgramFunctions()...)
		p.managerOptions.ExcludedFunctions = append(p.managerOptions.ExcludedFunctions, probes.GetAllSocketProgramFunctions()...)
	} else if !p.config.Probe.NetworkRawPacketEnabled {
		p.managerOptions.ExcludedFunctions = append(p.managerOptions.ExcludedFunctions, probes.GetRawPacketTCProgramFunctions()...)
	}

	if !p.kernelVersion.HasBpfGetSocketCookieForCgroupSocket() {
		p.managerOptions.ExcludedFunctions = append(p.managerOptions.ExcludedFunctions, probes.GetAllSocketProgramFunctions()...)
	}

	// prevent some tal calls from loading
	if !p.config.Probe.NetworkFlowMonitorEnabled {
		p.managerOptions.ExcludedFunctions = append(p.managerOptions.ExcludedFunctions, probes.GetAllFlushNetworkStatsTaillCallFunctions()...)
	}

	// prevent some helpers from loading
	if !p.kernelVersion.HasBPFForEachMapElemHelper() {
		p.managerOptions.ExcludedFunctions = append(p.managerOptions.ExcludedFunctions, probes.AllBPFForEachMapElemProgramFunctions()...)
	}

	if p.useFentry {
		afBasedExcluder, err := newAvailableFunctionsBasedExcluder()
		if err != nil {
			return err
		}

		p.managerOptions.AdditionalExcludedFunctionCollector = afBasedExcluder
	}

	if !(p.config.RuntimeSecurity.SysCtlEnabled && p.config.RuntimeSecurity.SysCtlEBPFEnabled) {
		p.managerOptions.ExcludedFunctions = append(p.managerOptions.ExcludedFunctions, probes.SysCtlProbeFunctionName)
	}

	if !p.config.Probe.CapabilitiesMonitoringEnabled {
		p.managerOptions.ExcludedFunctions = append(p.managerOptions.ExcludedFunctions, probes.GetCapabilitiesMonitoringProgramFunctions()...)
	}

	// on kernel before 4.15, you can only attach one eBPF program per tracepoint
	// to prevent conflicts with other eBPF using programs, we exclude our sched_process_fork tracepoint program
	// and use the get_task_pid kretprobe fallback instead
	if p.kernelVersion.Code < kernel.Kernel4_15 {
		p.managerOptions.ExcludedFunctions = append(p.managerOptions.ExcludedFunctions, probes.SchedProcessForkTracepointName)
	}

	return nil
}

// initManagerOptionsActivatedProbes initializes the eBPF manager activated probes options
func (p *EBPFProbe) initManagerOptionsActivatedProbes() {
	if p.config.RuntimeSecurity.ActivityDumpEnabled {
		for _, e := range p.config.RuntimeSecurity.ActivityDumpTracedEventTypes {
			if e == model.SyscallsEventType {
				// Add syscall monitor probes
				p.managerOptions.ActivatedProbes = append(p.managerOptions.ActivatedProbes, probes.SyscallMonitorSelectors()...)
				break
			}
		}
	}
	if p.config.RuntimeSecurity.AnomalyDetectionEnabled {
		for _, e := range p.config.RuntimeSecurity.AnomalyDetectionEventTypes {
			if e == model.SyscallsEventType {
				// Add syscall monitor probes
				p.managerOptions.ActivatedProbes = append(p.managerOptions.ActivatedProbes, probes.SyscallMonitorSelectors()...)
				break
			}
		}
	}
	p.managerOptions.ActivatedProbes = append(p.managerOptions.ActivatedProbes, probes.SnapshotSelectors(p.useFentry)...)

	if p.config.Probe.CapabilitiesMonitoringEnabled {
		p.managerOptions.ActivatedProbes = append(p.managerOptions.ActivatedProbes, probes.GetCapabilitiesMonitoringSelectors()...)
	}
}

// initManagerOptions initializes the eBPF manager options
func (p *EBPFProbe) initManagerOptions() error {
	kretprobeMaxActive := p.config.Probe.EventStreamKretprobeMaxActive

	p.managerOptions = ebpf.NewDefaultOptions(kretprobeMaxActive)
	p.initManagerOptionsActivatedProbes()
	p.initManagerOptionsConstants()
	p.initManagerOptionsTailCalls()
	p.initManagerOptionsMapSpecEditors()
	return p.initManagerOptionsExcludedFunctions()
}

// NewEBPFProbe instantiates a new runtime security agent probe
func NewEBPFProbe(probe *Probe, config *config.Config, ipc ipc.Component, opts Opts) (*EBPFProbe, error) {
	nerpc, err := erpc.NewERPC()
	if err != nil {
		return nil, err
	}

	onDemandRate := rate.Inf
	onDemandBurst := 0 // if rate is infinite, burst is not used
	if config.RuntimeSecurity.OnDemandRateLimiterEnabled {
		onDemandRate = MaxOnDemandEventsPerSecond
		onDemandBurst = MaxOnDemandEventsPerSecond
	}

	ctx, cancelFnc := context.WithCancel(context.Background())

	p := &EBPFProbe{
		probe:                probe,
		config:               config,
		opts:                 opts,
		statsdClient:         opts.StatsdClient,
		discarderRateLimiter: rate.NewLimiter(rate.Every(time.Second/5), 100),
		kfilters:             make(map[eval.EventType]kfilters.KFilters),
		Erpc:                 nerpc,
		erpcRequest:          erpc.NewERPCRequest(0),
		isRuntimeDiscarded:   !probe.Opts.DontDiscardRuntime,
		ctx:                  ctx,
		cancelFnc:            cancelFnc,
		tcRequests:           make(chan tcClassifierRequest, 16),
		onDemandRateLimiter:  rate.NewLimiter(onDemandRate, onDemandBurst),
		playSnapShotState:    atomic.NewBool(false),
		dnsLayer:             new(layers.DNS),
		ipc:                  ipc,
		BPFFilterTruncated:   atomic.NewUint64(0),
		MetricNameTruncated:  atomic.NewUint64(0),
	}

	if err := p.detectKernelVersion(); err != nil {
		// we need the kernel version to start, fail if we can't get it
		return nil, err
	}

	p.initCgroup2MountPath()

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
	p.selectRingBuffersMode()
	p.useMmapableMaps = p.kernelVersion.HaveMmapableMaps()

	p.Manager = ebpf.NewRuntimeSecurityManager(p.useRingBuffers)

	p.supportsBPFSendSignal = p.kernelVersion.SupportBPFSendSignal()
	pkos := NewProcessKillerOS(func(sig, pid uint32) error {
		if p.supportsBPFSendSignal {
			err := p.killListMap.Put(uint32(pid), uint32(sig))
			if err != nil {
				seclog.Warnf("failed to kill process with eBPF %d: %s", pid, err)
				return err
			}
		}
		return nil
	})
	processKiller, err := NewProcessKiller(config, pkos)
	if err != nil {
		return nil, err
	}
	p.processKiller = processKiller

	p.monitors = NewEBPFMonitors(p)

	p.numCPU, err = utils.NumCPU()
	if err != nil {
		return nil, fmt.Errorf("failed to parse CPU count: %w", err)
	}

	p.constantOffsets, err = p.getOffsetConstants()
	if err != nil {
		seclog.Warnf("constant fetcher failed: %v", err)
		return nil, err
	}

	resolversOpts := resolvers.Opts{
		PathResolutionEnabled:    probe.Opts.PathResolutionEnabled,
		EnvVarsResolutionEnabled: probe.Opts.EnvsVarResolutionEnabled,
		Tagger:                   probe.Opts.Tagger,
		UseRingBuffer:            p.useRingBuffers,
		TTYFallbackEnabled:       probe.Opts.TTYFallbackEnabled,
	}

	p.Resolvers, err = resolvers.NewEBPFResolvers(config, p.Manager, probe.StatsdClient, probe.scrubber, p.Erpc, resolversOpts)
	if err != nil {
		return nil, err
	}

	p.fileHasher = NewFileHasher(config, p.Resolvers.HashResolver)

	hostname, err := hostnameutils.GetHostname(ipc)
	if err != nil || hostname == "" {
		hostname = "unknown"
	}

	if config.RuntimeSecurity.OnDemandEnabled {
		p.onDemandManager = &OnDemandProbesManager{
			probe:   p,
			manager: p.Manager,
		}
	}

	fh, err := NewEBPFFieldHandlers(config, p.Resolvers, hostname, p.onDemandManager)
	if err != nil {
		return nil, err
	}
	p.fieldHandlers = fh

	p.eventPool = ddsync.NewTypedPool(func() *model.Event {
		return newEBPFEvent(p.fieldHandlers)
	})

	if p.useRingBuffers {
		p.eventStream = ringbuffer.New(p.handleEvent)
	} else {
		p.eventStream, err = reorderer.NewOrderedPerfMap(p.ctx, p.handleEvent, probe.StatsdClient)
		if err != nil {
			return nil, err
		}
	}

	p.event = p.NewEvent()

	// be sure to zero the probe event before everything else
	p.zeroEvent()

	return p, nil
}

func getFuncArgCount(prog *lib.ProgramSpec) uint64 {
	if !strings.HasPrefix(prog.SectionName, "fexit/") {
		return 0 // this value should never be used
	}

	argc, err := constantfetch.GetBTFFunctionArgCount(prog.AttachTo)
	if err != nil {
		seclog.Errorf("failed to get function argument count for %s: %v", prog.AttachTo, err)
		return 0
	}

	return uint64(argc)
}

// GetProfileManager returns the security profile manager
func (p *EBPFProbe) GetProfileManager() *securityprofile.Manager {
	return p.profileManager
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

	if kernelVersion.IsInRangeCloseOpen(kernel.Kernel5_14, kernel.Kernel5_15) && kernelVersion.IsRH9_4Kernel() {
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

// getOffsetConstants returns the offsets and struct sizes constants
func (p *EBPFProbe) getOffsetConstants() (*constantfetch.ConstantFetcherStatus, error) {
	constantFetcher := constantfetch.ComposeConstantFetchers(constantfetch.GetAvailableConstantFetchers(p.config.Probe, p.kernelVersion))
	AppendProbeRequestsToFetcher(constantFetcher, p.kernelVersion)
	return constantFetcher.FinishAndGetStatus()
}

// GetConstantFetcherStatus returns the status of the constant fetcher associated with this probe
func (p *EBPFProbe) GetConstantFetcherStatus() *constantfetch.ConstantFetcherStatus {
	return p.constantOffsets
}

func appendOffsetofRequest(constantFetcher constantfetch.ConstantFetcher, name string, structName string, memberName string) {
	constantFetcher.AppendOffsetofRequestWithFallbacks(name, constantfetch.TypeFieldPair{
		TypeName:  structName,
		FieldName: memberName,
	})
}

// AppendProbeRequestsToFetcher returns the offsets and struct sizes constants, from a constant fetcher
func AppendProbeRequestsToFetcher(constantFetcher constantfetch.ConstantFetcher, kv *kernel.Version) {
	constantFetcher.AppendSizeofRequest(constantfetch.SizeOfInode, "struct inode")
	appendOffsetofRequest(constantFetcher, constantfetch.OffsetNameSuperBlockStructSFlags, "struct super_block", "s_flags")
	appendOffsetofRequest(constantFetcher, constantfetch.OffsetNameSuperBlockStructSMagic, "struct super_block", "s_magic")
	appendOffsetofRequest(constantFetcher, constantfetch.OffsetNameDentryStructDSB, "struct dentry", "d_sb")
	appendOffsetofRequest(constantFetcher, constantfetch.OffsetNameSignalStructStructTTY, "struct signal_struct", "tty")
	appendOffsetofRequest(constantFetcher, constantfetch.OffsetNameTTYStructStructName, "struct tty_struct", "name")
	appendOffsetofRequest(constantFetcher, constantfetch.OffsetNameCredStructUID, "struct cred", "uid")
	appendOffsetofRequest(constantFetcher, constantfetch.OffsetNameCredStructCapInheritable, "struct cred", "cap_inheritable")
	appendOffsetofRequest(constantFetcher, constantfetch.OffsetNameLinuxBinprmP, "struct linux_binprm", "p")
	appendOffsetofRequest(constantFetcher, constantfetch.OffsetNameLinuxBinprmArgc, "struct linux_binprm", "argc")
	appendOffsetofRequest(constantFetcher, constantfetch.OffsetNameLinuxBinprmEnvc, "struct linux_binprm", "envc")
	appendOffsetofRequest(constantFetcher, constantfetch.OffsetNameVMAreaStructFlags, "struct vm_area_struct", "vm_flags")
	appendOffsetofRequest(constantFetcher, constantfetch.OffsetNameFileFinode, "struct file", "f_inode")
	appendOffsetofRequest(constantFetcher, constantfetch.OffsetNameFileFpath, "struct file", "f_path")
	appendOffsetofRequest(constantFetcher, constantfetch.OffsetNameDentryDSb, "struct dentry", "d_sb")
	appendOffsetofRequest(constantFetcher, constantfetch.OffsetNameMountMntID, "struct mount", "mnt_id")
	if kv.Code >= kernel.Kernel5_3 {
		appendOffsetofRequest(constantFetcher, constantfetch.OffsetNameKernelCloneArgsExitSignal, "struct kernel_clone_args", "exit_signal")
	}

	// inode time offsets
	// no runtime compilation for those, we expect them to default to 0 if not there and use the fallback
	// in the eBPF code itself in that case
	if kv.Code >= kernel.Kernel6_11 {
		appendOffsetofRequest(constantFetcher, constantfetch.OffsetNameInodeCtimeSec, "struct inode", "i_ctime_sec")
		appendOffsetofRequest(constantFetcher, constantfetch.OffsetNameInodeCtimeNsec, "struct inode", "i_ctime_nsec")
		appendOffsetofRequest(constantFetcher, constantfetch.OffsetNameInodeMtimeSec, "struct inode", "i_mtime_sec")
		appendOffsetofRequest(constantFetcher, constantfetch.OffsetNameInodeMtimeNsec, "struct inode", "i_mtime_nsec")
	}

	// rename offsets
	if kv.Code >= kernel.Kernel5_12 || (kv.IsInRangeCloseOpen(kernel.Kernel5_10, kernel.Kernel5_11) && kv.Code.Patch() >= 220) {
		appendOffsetofRequest(constantFetcher, constantfetch.OffsetNameRenameStructOldDentry, "struct renamedata", "old_dentry")
		appendOffsetofRequest(constantFetcher, constantfetch.OffsetNameRenameStructNewDentry, "struct renamedata", "new_dentry")
	}

	// bpf offsets
	appendOffsetofRequest(constantFetcher, constantfetch.OffsetNameBPFMapStructID, "struct bpf_map", "id")
	if kv.Code != 0 && (kv.Code >= kernel.Kernel4_15 || kv.IsRH7Kernel()) {
		appendOffsetofRequest(constantFetcher, constantfetch.OffsetNameBPFMapStructName, "struct bpf_map", "name")
	}
	appendOffsetofRequest(constantFetcher, constantfetch.OffsetNameBPFMapStructMapType, "struct bpf_map", "map_type")
	appendOffsetofRequest(constantFetcher, constantfetch.OffsetNameBPFProgAuxStructID, "struct bpf_prog_aux", "id")
	if kv.Code != 0 && (kv.Code >= kernel.Kernel4_15 || kv.IsRH7Kernel()) {
		appendOffsetofRequest(constantFetcher, constantfetch.OffsetNameBPFProgAuxStructName, "struct bpf_prog_aux", "name")
	}
	appendOffsetofRequest(constantFetcher, constantfetch.OffsetNameBPFProgStructTag, "struct bpf_prog", "tag")
	appendOffsetofRequest(constantFetcher, constantfetch.OffsetNameBPFProgStructAux, "struct bpf_prog", "aux")
	appendOffsetofRequest(constantFetcher, constantfetch.OffsetNameBPFProgStructType, "struct bpf_prog", "type")

	if kv.Code != 0 && (kv.Code > kernel.Kernel4_16 || kv.IsSuse12Kernel() || kv.IsSuse15Kernel()) {
		appendOffsetofRequest(constantFetcher, constantfetch.OffsetNameBPFProgStructExpectedAttachType, "struct bpf_prog", "expected_attach_type")
	}
	// namespace nr offsets
	appendOffsetofRequest(constantFetcher, constantfetch.OffsetNamePIDStructLevel, "struct pid", "level")
	appendOffsetofRequest(constantFetcher, constantfetch.OffsetNamePIDStructNumbers, "struct pid", "numbers")
	constantFetcher.AppendSizeofRequest(constantfetch.SizeOfUPID, "struct upid")
	if kv.HavePIDLinkStruct() {
		appendOffsetofRequest(constantFetcher, constantfetch.OffsetNameTaskStructPIDLink, "struct task_struct", "pids")
		appendOffsetofRequest(constantFetcher, constantfetch.OffsetNamePIDLinkStructPID, "struct pid_link", "pid")
	} else {
		appendOffsetofRequest(constantFetcher, constantfetch.OffsetNameTaskStructPID, "struct task_struct", "thread_pid")
	}

	// splice event
	constantFetcher.AppendSizeofRequest(constantfetch.SizeOfPipeBuffer, "struct pipe_buffer")
	appendOffsetofRequest(constantFetcher, constantfetch.OffsetNamePipeInodeInfoStructBufs, "struct pipe_inode_info", "bufs")
	appendOffsetofRequest(constantFetcher, constantfetch.OffsetNamePipeBufferStructFlags, "struct pipe_buffer", "flags")
	if kv.HaveLegacyPipeInodeInfoStruct() {
		appendOffsetofRequest(constantFetcher, constantfetch.OffsetNamePipeInodeInfoStructNrbufs, "struct pipe_inode_info", "nrbufs")
		appendOffsetofRequest(constantFetcher, constantfetch.OffsetNamePipeInodeInfoStructCurbuf, "struct pipe_inode_info", "curbuf")
		appendOffsetofRequest(constantFetcher, constantfetch.OffsetNamePipeInodeInfoStructBuffers, "struct pipe_inode_info", "buffers")
	} else {
		appendOffsetofRequest(constantFetcher, constantfetch.OffsetNamePipeInodeInfoStructHead, "struct pipe_inode_info", "head")
		appendOffsetofRequest(constantFetcher, constantfetch.OffsetNamePipeInodeInfoStructRingsize, "struct pipe_inode_info", "ring_size")
	}

	// network related constants
	appendOffsetofRequest(constantFetcher, constantfetch.OffsetNameNetDeviceStructIfIndex, "struct net_device", "ifindex")
	appendOffsetofRequest(constantFetcher, constantfetch.OffsetNameNetDeviceStructName, "struct net_device", "name")
	appendOffsetofRequest(constantFetcher, constantfetch.OffsetNameDeviceStructNdNet, "struct net_device", "nd_net")
	appendOffsetofRequest(constantFetcher, constantfetch.OffsetNameSockCommonStructSKCNet, "struct sock_common", "skc_net")
	appendOffsetofRequest(constantFetcher, constantfetch.OffsetNameSockCommonStructSKCFamily, "struct sock_common", "skc_family")
	appendOffsetofRequest(constantFetcher, constantfetch.OffsetNameFlowI4StructSADDR, "struct flowi4", "saddr")
	appendOffsetofRequest(constantFetcher, constantfetch.OffsetNameFlowI4StructULI, "struct flowi4", "uli")
	appendOffsetofRequest(constantFetcher, constantfetch.OffsetNameFlowI6StructSADDR, "struct flowi6", "saddr")
	appendOffsetofRequest(constantFetcher, constantfetch.OffsetNameFlowI6StructULI, "struct flowi6", "uli")
	appendOffsetofRequest(constantFetcher, constantfetch.OffsetNameSocketStructSK, "struct socket", "sk")
	appendOffsetofRequest(constantFetcher, constantfetch.OffsetNameSockCommonStructSKCNum, "struct sock_common", "skc_num")
	appendOffsetofRequest(constantFetcher, constantfetch.OffsetNameSockStructSKProtocol, "struct sock", "sk_protocol")
	constantFetcher.AppendOffsetofRequestWithFallbacks(
		constantfetch.OffsetNameFlowI4StructProto,
		constantfetch.TypeFieldPair{
			TypeName:  "struct flowi4",
			FieldName: "flowi4_proto",
		},
		constantfetch.TypeFieldPair{
			TypeName:  "struct flowi_common",
			FieldName: "flowic_proto",
		},
	)
	constantFetcher.AppendOffsetofRequestWithFallbacks(
		constantfetch.OffsetNameFlowI6StructProto,
		constantfetch.TypeFieldPair{
			TypeName:  "struct flowi6",
			FieldName: "flowi6_proto",
		},
		constantfetch.TypeFieldPair{
			TypeName:  "struct flowi_common",
			FieldName: "flowic_proto",
		},
	)

	// Interpreter constants
	appendOffsetofRequest(constantFetcher, constantfetch.OffsetNameLinuxBinprmStructFile, "struct linux_binprm", "file")

	if !kv.IsRH7Kernel() {
		appendOffsetofRequest(constantFetcher, constantfetch.OffsetNameNFConnStructCTNet, "struct nf_conn", "ct_net")
	}

	if getNetStructType(kv) == netStructHasProcINum {
		appendOffsetofRequest(constantFetcher, constantfetch.OffsetNameNetStructProcInum, "struct net", "proc_inum")
	} else {
		appendOffsetofRequest(constantFetcher, constantfetch.OffsetNameNetStructNS, "struct net", "ns")
	}

	// iouring
	if kv.Code != 0 && (kv.Code >= kernel.Kernel5_1) {
		appendOffsetofRequest(constantFetcher, constantfetch.OffsetNameIoKiocbStructCtx, "struct io_kiocb", "ctx")
	}

	// inode
	appendOffsetofRequest(constantFetcher, constantfetch.OffsetInodeIno, "struct inode", "i_ino")
	appendOffsetofRequest(constantFetcher, constantfetch.OffsetInodeGid, "struct inode", "i_gid")
	appendOffsetofRequest(constantFetcher, constantfetch.OffsetInodeNlink, "struct inode", "i_nlink")
	constantFetcher.AppendOffsetofRequestWithFallbacks(
		constantfetch.OffsetInodeMtime,
		constantfetch.TypeFieldPair{
			TypeName:  "struct inode",
			FieldName: "i_mtime",
		},
		constantfetch.TypeFieldPair{
			TypeName:  "struct inode",
			FieldName: "__i_mtime",
		},
	)
	constantFetcher.AppendOffsetofRequestWithFallbacks(
		constantfetch.OffsetInodeCtime,
		constantfetch.TypeFieldPair{
			TypeName:  "struct inode",
			FieldName: "i_ctime",
		},
		constantfetch.TypeFieldPair{
			TypeName:  "struct inode",
			FieldName: "__i_ctime",
		},
	)

	// fs
	appendOffsetofRequest(constantFetcher, constantfetch.OffsetNameSbDev, "struct super_block", "s_dev")
	appendOffsetofRequest(constantFetcher, constantfetch.OffsetNameSuperblockSType, "struct super_block", "s_type")
	appendOffsetofRequest(constantFetcher, constantfetch.OffsetNameDentryDInode, "struct dentry", "d_inode")
	appendOffsetofRequest(constantFetcher, constantfetch.OffsetNameDentryDName, "struct dentry", "d_name")
	appendOffsetofRequest(constantFetcher, constantfetch.OffsetNamePathDentry, "struct path", "dentry")
	appendOffsetofRequest(constantFetcher, constantfetch.OffsetNamePathMnt, "struct path", "mnt")
	appendOffsetofRequest(constantFetcher, constantfetch.OffsetNameInodeSuperblock, "struct inode", "i_sb")
	appendOffsetofRequest(constantFetcher, constantfetch.OffsetNameMountMntMountpoint, "struct mount", "mnt_mountpoint")
	appendOffsetofRequest(constantFetcher, constantfetch.OffsetNameMountpointDentry, "struct mountpoint", "m_dentry")
	appendOffsetofRequest(constantFetcher, constantfetch.OffsetNameVfsmountMntFlags, "struct vfsmount", "mnt_flags")
	appendOffsetofRequest(constantFetcher, constantfetch.OffsetNameVfsmountMntRoot, "struct vfsmount", "mnt_root")
	appendOffsetofRequest(constantFetcher, constantfetch.OffsetNameVfsmountMntSb, "struct vfsmount", "mnt_sb")
	appendOffsetofRequest(constantFetcher, constantfetch.OffsetNameRtnlLinkOpsKind, "struct rtnl_link_ops", "kind")
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
			_ = p.RefreshUserCache(ev.ProcessContext.Process.ContainerContext.ContainerID)

		case action.InternalCallback != nil && rule.ID == bundled.RefreshSBOMRuleID && p.Resolvers.SBOMResolver != nil && len(ev.ProcessContext.Process.ContainerContext.ContainerID) > 0:
			if err := p.Resolvers.SBOMResolver.RefreshSBOM(ev.ProcessContext.Process.ContainerContext.ContainerID); err != nil {
				seclog.Warnf("failed to refresh SBOM for container %s, triggered by %s: %s", ev.ProcessContext.Process.ContainerContext.ContainerID, ev.ProcessContext.Comm, err)
			}

		case action.Def.Kill != nil:
			// do not handle kill action on event with error
			if ev.Error != nil {
				return
			}

			if p.processKiller.KillAndReport(action.Def.Kill, rule, ev) {
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
			if p.fileHasher.HashAndReport(rule, action.Def.Hash, ev) {
				p.probe.onRuleActionPerformed(rule, action.Def)
			}
		case action.Def.NetworkFilter != nil:
			if !p.config.RuntimeSecurity.EnforcementEnabled {
				return
			}

			var policy rawpacket.Policy
			policy.Parse(action.Def.NetworkFilter.Policy)

			if policy == rawpacket.PolicyDrop {
				dropActionFilter := rawpacket.Filter{
					RuleID:    rule.ID,
					BPFFilter: action.Def.NetworkFilter.BPFFilter,
					Policy:    policy,
				}

				if action.Def.NetworkFilter.Scope == "cgroup" {
					dropActionFilter.CGroupPathKey = ev.ProcessContext.Process.CGroup.CGroupFile
				} else {
					dropActionFilter.Pid = ev.ProcessContext.Pid
				}

				if err := p.addRawPacketActionFilter(dropActionFilter); err != nil {
					seclog.Errorf("failed to setup raw packet action programs: %s", err)
				}

				report := &RawPacketActionReport{
					Filter: action.Def.NetworkFilter.BPFFilter,
					Policy: policy.String(),
					rule:   rule,
				}

				ev.ActionReports = append(ev.ActionReports, report)

				p.probe.onRuleActionPerformed(rule, action.Def)
			}
		}
	}
}

// GetAgentContainerContext returns the agent container context
func (p *EBPFProbe) GetAgentContainerContext() *events.AgentContainerContext {
	return p.probe.GetAgentContainerContext()
}

// newPlaceholderProcessCacheEntryPTraceMe returns a new empty process cache entry for PTRACE_TRACEME calls,
// it's similar to model.NewPlaceholderProcessCacheEntry with pid = tid = 0, and isKworker = false
var newPlaceholderProcessCacheEntryPTraceMe = sync.OnceValue(func() *model.ProcessCacheEntry {
	return model.NewPlaceholderProcessCacheEntry(0, 0, false)
})

// newEBPFPooledEventFromPCE returns a new event from a process cache entry
func (p *EBPFProbe) newEBPFPooledEventFromPCE(entry *model.ProcessCacheEntry) *model.Event {
	eventType := model.ExecEventType
	if !entry.IsExec {
		eventType = model.ForkEventType
	}

	event := p.eventPool.Get()

	event.Type = uint32(eventType)
	event.TimestampRaw = uint64(time.Now().UnixNano())
	event.ProcessCacheEntry = entry
	event.ProcessContext = &entry.ProcessContext
	event.Exec.Process = &entry.Process
	event.ProcessContext.Process.ContainerContext.ContainerID = entry.ContainerContext.ContainerID
	event.ProcessContext.Process.CGroup = entry.CGroup

	return event
}

// newBindEventFromSnapshot returns a new bind event with a process context
func (p *EBPFProbe) newBindEventFromSnapshot(entry *model.ProcessCacheEntry, snapshottedBind model.SnapshottedBoundSocket) *model.Event {
	event := p.eventPool.Get()
	event.TimestampRaw = uint64(time.Now().UnixNano())
	event.Type = uint32(model.BindEventType)
	event.ProcessCacheEntry = entry
	event.ProcessContext = &entry.ProcessContext
	event.ProcessContext.Process.ContainerContext.ContainerID = entry.ContainerContext.ContainerID
	event.ProcessContext.Process.CGroup = entry.CGroup

	event.Bind.SyscallEvent.Retval = 0
	event.Bind.AddrFamily = snapshottedBind.Family
	event.Bind.Addr.IPNet.IP = snapshottedBind.IP
	event.Bind.Protocol = snapshottedBind.Protocol
	if snapshottedBind.Family == unix.AF_INET {
		event.Bind.Addr.IPNet.Mask = net.CIDRMask(32, 32)
	} else {
		event.Bind.Addr.IPNet.Mask = net.CIDRMask(128, 128)
	}
	event.Bind.Addr.Port = snapshottedBind.Port

	return event
}

func (p *EBPFProbe) addToDNSResolver(dnsLayer *layers.DNS) {
	for _, answer := range dnsLayer.Answers {
		if answer.Type == layers.DNSTypeCNAME {
			p.Resolvers.DNSResolver.AddNewCname(string(answer.CNAME), string(answer.Name))
		} else if answer.Type == layers.DNSTypeA || answer.Type == layers.DNSTypeAAAA {
			ip, ok := netip.AddrFromSlice(answer.IP)
			if ok {
				p.Resolvers.DNSResolver.AddNew(string(answer.Name), ip)
			} else {
				seclog.Errorf("DNS response with an invalid IP received: %v", ip)
			}
		}
	}
}

func trimRightZeros(b []byte) []byte {
	i := len(b) - 1
	for i >= 0 && b[i] == 0 {
		i--
	}
	return b[:i+1]
}
