package probe

import (
	"bytes"
	"fmt"
	"math"
	"strings"

	"github.com/pkg/errors"

	"github.com/iovisor/gobpf/elf"

	"github.com/DataDog/datadog-agent/pkg/ebpf/gobpf"
	eprobe "github.com/DataDog/datadog-agent/pkg/ebpf/probe"
	"github.com/DataDog/datadog-agent/pkg/ebpf/probe/types"
	"github.com/DataDog/datadog-agent/pkg/security/config"
	"github.com/DataDog/datadog-agent/pkg/security/secl/eval"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/datadog-go/statsd"
)

const MetricPrefix = "datadog.agent.runtime_security"

type EventHandler interface {
	HandleEvent(event *Event)
}

type Stats struct {
	Events   EventsStats
	Syscalls *SyscallStats
}

type KTable struct {
	Name string
}

type Discarder struct {
	Field string
	Value interface{}
}

type onApproversFnc func(probe *Probe, approvers eval.Approvers) error
type onDiscarderFnc func(probe *Probe, discarder Discarder) error

type Probe struct {
	*eprobe.Probe
	config           *config.Config
	handler          EventHandler
	resolvers        *Resolvers
	onDiscardersFncs map[string][]onDiscarderFnc
	enableFilters    bool
	tables           map[string]eprobe.Table
	stats            EventsStats
	syscallMonitor   *SyscallMonitor
}

type Capability struct {
	PolicyFlags     PolicyFlag
	FieldValueTypes eval.FieldValueType
}

type Capabilities map[string]Capability

type HookPoint struct {
	Name            string
	KProbes         []*eprobe.KProbe
	Optional        bool
	EventTypes      map[string]Capabilities
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

func syscallKprobe(name string) []*eprobe.KProbe {
	return []*eprobe.KProbe{{
		EntryFunc: "kprobe/" + getSyscallFnName(name),
		ExitFunc:  "kretprobe/" + getSyscallFnName(name),
	}}
}

var AllHookPoints = []*HookPoint{
	{
		Name: "security_inode_setattr",
		KProbes: []*eprobe.KProbe{{
			EntryFunc: "kprobe/security_inode_setattr",
		}},
		EventTypes: map[string]Capabilities{
			"chmod":  Capabilities{},
			"chown":  Capabilities{},
			"utimes": Capabilities{},
		},
	},
	{
		Name:    "sys_chmod",
		KProbes: syscallKprobe("chmod"),
		EventTypes: map[string]Capabilities{
			"chmod": Capabilities{},
		},
	},
	{
		Name:    "sys_fchmod",
		KProbes: syscallKprobe("fchmod"),
		EventTypes: map[string]Capabilities{
			"chmod": Capabilities{},
		},
	},
	{
		Name:    "sys_fchmodat",
		KProbes: syscallKprobe("fchmodat"),
		EventTypes: map[string]Capabilities{
			"chmod": Capabilities{},
		},
	},
	{
		Name:    "sys_chown",
		KProbes: syscallKprobe("chown"),
		EventTypes: map[string]Capabilities{
			"chown": Capabilities{},
		},
	},
	{
		Name:    "sys_fchown",
		KProbes: syscallKprobe("fchown"),
		EventTypes: map[string]Capabilities{
			"chown": Capabilities{},
		},
	},
	{
		Name:    "sys_fchownat",
		KProbes: syscallKprobe("fchownat"),
		EventTypes: map[string]Capabilities{
			"chown": Capabilities{},
		},
	},
	{
		Name:    "sys_lchown",
		KProbes: syscallKprobe("lchown"),
		EventTypes: map[string]Capabilities{
			"chown": Capabilities{},
		},
	},
	{
		Name: "mnt_want_write",
		KProbes: []*eprobe.KProbe{{
			EntryFunc: "kprobe/mnt_want_write",
		}},
		EventTypes: map[string]Capabilities{
			"utimes": Capabilities{},
			"chmod":  Capabilities{},
			"chown":  Capabilities{},
			"rmdir":  Capabilities{},
			"unlink": Capabilities{},
			"rename": Capabilities{},
		},
	},
	{
		Name: "mnt_want_write_file",
		KProbes: []*eprobe.KProbe{{
			EntryFunc: "kprobe/mnt_want_write_file",
		}},
		EventTypes: map[string]Capabilities{
			"chown": Capabilities{},
		},
	},
	{
		Name:    "sys_utime",
		KProbes: syscallKprobe("utime"),
		EventTypes: map[string]Capabilities{
			"utimes": Capabilities{},
		},
	},
	{
		Name:    "sys_utimes",
		KProbes: syscallKprobe("utimes"),
		EventTypes: map[string]Capabilities{
			"utimes": Capabilities{},
		},
	},
	{
		Name:    "sys_utimensat",
		KProbes: syscallKprobe("utimensat"),
		EventTypes: map[string]Capabilities{
			"utimes": Capabilities{},
		},
	},
	{
		Name:    "sys_futimesat",
		KProbes: syscallKprobe("futimesat"),
		EventTypes: map[string]Capabilities{
			"utimes": Capabilities{},
		},
	},
	{
		Name: "vfs_mkdir",
		KProbes: []*eprobe.KProbe{{
			EntryFunc: "kprobe/vfs_mkdir",
		}},
		EventTypes: map[string]Capabilities{
			"mkdir": Capabilities{},
		},
	},
	{
		Name: "filename_create",
		KProbes: []*eprobe.KProbe{{
			EntryFunc: "kprobe/filename_create",
		}},
		EventTypes: map[string]Capabilities{
			"mkdir": Capabilities{},
			"link":  Capabilities{},
		},
	},
	{
		Name:    "sys_mkdir",
		KProbes: syscallKprobe("mkdir"),
		EventTypes: map[string]Capabilities{
			"mkdir": Capabilities{},
		},
	},
	{
		Name:    "sys_mkdirat",
		KProbes: syscallKprobe("mkdirat"),
		EventTypes: map[string]Capabilities{
			"mkdir": Capabilities{},
		},
	},
	{
		Name: "vfs_rmdir",
		KProbes: []*eprobe.KProbe{{
			EntryFunc: "kprobe/vfs_rmdir",
		}},
		EventTypes: map[string]Capabilities{
			"rmdir":  Capabilities{},
			"unlink": Capabilities{},
		},
	},
	{
		Name:    "sys_rmdir",
		KProbes: syscallKprobe("rmdir"),
		EventTypes: map[string]Capabilities{
			"rmdir": Capabilities{},
		},
	},
	{
		Name: "vfs_unlink",
		KProbes: []*eprobe.KProbe{{
			EntryFunc: "kprobe/vfs_unlink",
		}},
		EventTypes: map[string]Capabilities{
			"unlink": Capabilities{},
		},
	},
	{
		Name:    "sys_unlink",
		KProbes: syscallKprobe("unlink"),
		EventTypes: map[string]Capabilities{
			"unlink": Capabilities{},
		},
	},
	{
		Name:    "sys_unlinkat",
		KProbes: syscallKprobe("unlinkat"),
		EventTypes: map[string]Capabilities{
			"unlink": Capabilities{},
		},
	},
	{
		Name: "vfs_rename",
		KProbes: []*eprobe.KProbe{{
			EntryFunc: "kprobe/vfs_rename",
		}},
		EventTypes: map[string]Capabilities{
			"rename": Capabilities{},
		},
	},
	{
		Name:    "sys_rename",
		KProbes: syscallKprobe("rename"),
		EventTypes: map[string]Capabilities{
			"rename": Capabilities{},
		},
	},
	{
		Name:    "sys_renameat",
		KProbes: syscallKprobe("renameat"),
		EventTypes: map[string]Capabilities{
			"rename": Capabilities{},
		},
	},
	{
		Name:    "sys_renameat2",
		KProbes: syscallKprobe("renameat2"),
		EventTypes: map[string]Capabilities{
			"rename": Capabilities{},
		},
	},
	{
		Name: "vfs_link",
		KProbes: []*eprobe.KProbe{{
			EntryFunc: "kprobe/vfs_link",
		}},
		EventTypes: map[string]Capabilities{
			"link": Capabilities{},
		},
	},
	{
		Name:    "sys_link",
		KProbes: syscallKprobe("link"),
		EventTypes: map[string]Capabilities{
			"link": Capabilities{},
		},
	},
	{
		Name:    "sys_linkat",
		KProbes: syscallKprobe("linkat"),
		EventTypes: map[string]Capabilities{
			"link": Capabilities{},
		},
	},
}

func (caps Capabilities) GetFlags() PolicyFlag {
	var flags PolicyFlag
	for _, cap := range caps {
		flags |= cap.PolicyFlags
	}
	return flags
}

func (caps Capabilities) GetField() []string {
	var fields []string

	for field := range caps {
		fields = append(fields, field)
	}

	return fields
}

func (caps Capabilities) GetFieldCapabilities() eval.FieldCapabilities {
	var fcs eval.FieldCapabilities

	for field, cap := range caps {
		fcs = append(fcs, eval.FieldCapability{
			Field: field,
			Types: cap.FieldValueTypes,
		})
	}

	return fcs
}

func (p *Probe) NewRuleSet(opts eval.Opts) *eval.RuleSet {
	eventCtor := func() eval.Event {
		return NewEvent(p.resolvers)
	}

	return eval.NewRuleSet(&Model{}, eventCtor, opts)
}

func (p *Probe) getTableNames() []*types.Table {
	tables := []*types.Table{
		{
			Name: "pathnames",
		},
		{
			Name: "noisy_processes_buffer",
		},
		{
			Name: "noisy_processes_fb",
		},
		{
			Name: "noisy_processes_bb",
		},
	}

	kTables := OpenTables
	for _, ktable := range kTables {
		tables = append(tables, &types.Table{Name: ktable.Name})
	}

	return tables
}

// Table returns either an eprobe Table or a LRU based eprobe Table
func (p *Probe) Table(name string) eprobe.Table {
	if table, exists := p.tables[name]; exists {
		return table
	}

	return p.Probe.Table(name)
}

func (p *Probe) getPerfMaps() []*types.PerfMap {
	return []*types.PerfMap{
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

func (p *Probe) Start() error {
	asset := "pkg/security/ebpf/probe"
	openSyscall := getSyscallFnName("open")
	if !strings.HasPrefix(openSyscall, "SyS_") && !strings.HasPrefix(openSyscall, "sys_") {
		asset += "-syscall-wrapper"
	}

	bytecode, err := Asset(asset + ".o") // ioutil.ReadFile("pkg/security/ebpf/probe.o")
	if err != nil {
		return err
	}

	p.Module, err = gobpf.NewModuleFromReader(bytes.NewReader(bytecode))
	if err != nil {
		return err
	}

	if err := p.Load(); err != nil {
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

	for _, hookpoint := range AllHookPoints {
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

	if err := ebpfProbe.Load(); err != nil {
		return nil, err
	}
	p.Probe = ebpfProbe

	if err := p.initLRUTables(); err != nil {
		return nil, err
	}

	dentryResolver, err := NewDentryResolver(ebpfProbe)
	if err != nil {
		return nil, err
	}

	p.resolvers = &Resolvers{
		DentryResolver: dentryResolver,
		MountResolver:  NewMountResolver(),
	}

	return p, nil
}

func (p *Probe) SetEventHandler(handler EventHandler) {
	p.handler = handler
}

func (p *Probe) DispatchEvent(event *Event) {
	if p.handler != nil {
		p.handler.HandleEvent(event)
	}
}

func (p *Probe) SendStats(statsdClient *statsd.Client) error {
	if p.syscallMonitor != nil {
		if err := p.syscallMonitor.SendStats(statsdClient); err != nil {
			return err
		}
	}

	if err := statsdClient.Count(MetricPrefix+".events.lost", p.stats.GetAndResetLost(), nil, 1.0); err != nil {
		return err
	}

	if err := statsdClient.Count(MetricPrefix+".events.received", p.stats.GetAndResetReceived(), nil, 1.0); err != nil {
		return err
	}

	for i := range p.stats.PerEventType {
		if i == 0 {
			continue
		}

		eventType := ProbeEventType(i)
		key := MetricPrefix + ".events." + eventType.String()
		if err := statsdClient.Count(key, p.stats.GetAndResetEventCount(eventType), nil, 1.0); err != nil {
			return err
		}
	}

	return nil
}

func (p *Probe) GetStats() (stats Stats, err error) {
	stats.Events = p.stats
	if p.syscallMonitor != nil {
		stats.Syscalls, err = p.syscallMonitor.GetStats()
	}

	return stats, err
}

func (p *Probe) handleLostEvents(count uint64) {
	log.Warnf("Lost %d events\n", count)
	p.stats.CountLost(int64(count))
}

func (p *Probe) handleEvent(data []byte) {
	log.Debugf("Handling dentry event (len %d)", len(data))
	p.stats.CountReceived(1)

	offset := 0
	event := NewEvent(p.resolvers)

	read, err := event.Event.UnmarshalBinary(data)
	if err != nil {
		log.Errorf("failed to decode event")
		return
	}
	offset += read

	read, err = event.Process.UnmarshalBinary(data[offset:])
	if err != nil {
		log.Errorf("failed to decode process event")
		return
	}
	offset += read

	eventType := ProbeEventType(event.Event.Type)
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
		// Resolve event inode
		event.Mount.ResolveInode(p.resolvers)
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
		log.Errorf("Unsupported event type %d\n", eventType)
		return
	}

	p.stats.CountEventType(eventType, 1)

	log.Debugf("Dispatching event %+v\n", event)
	p.DispatchEvent(event)
}

func (p *Probe) OnNewDiscarder(event *Event, field string) error {
	log.Debugf("New discarder event %+v for field %s\n", event, field)

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

		if err = fnc(p, discarder); err != nil {
			return err
		}
	}

	return nil
}

func (p *Probe) SetFilterPolicy(tableName string, mode PolicyMode, flags PolicyFlag) error {
	table := p.Table(tableName)
	if table == nil {
		return fmt.Errorf("unable to find policy table `%s`", tableName)
	}

	policy := FilterPolicy{
		Mode:  mode,
		Flags: flags,
	}
	return table.Set(zeroInt32, policy.Bytes())
}

type PolicyReport struct {
	Mode      PolicyMode
	Flags     PolicyFlag
	Approvers eval.Approvers
}

type Report struct {
	Policies map[string]*PolicyReport
}

func NewReport() *Report {
	return &Report{
		Policies: make(map[string]*PolicyReport),
	}
}

type Applier interface {
	ApplyFilterPolicy(eventType string, tableName string, mode PolicyMode, flags PolicyFlag) error
	ApplyApprovers(eventType string, hook *HookPoint, approvers eval.Approvers) error
	GetReport() *Report
}

type Reporter struct {
	report *Report
}

func (r *Reporter) getPolicyReport(eventType string) *PolicyReport {
	if r.report.Policies[eventType] == nil {
		r.report.Policies[eventType] = &PolicyReport{Approvers: eval.Approvers{}}
	}
	return r.report.Policies[eventType]
}

func (r *Reporter) ApplyFilterPolicy(eventType string, tableName string, mode PolicyMode, flags PolicyFlag) error {
	policyReport := r.getPolicyReport(eventType)
	policyReport.Mode = mode
	policyReport.Flags = flags
	return nil
}

func (r *Reporter) ApplyApprovers(eventType string, hookPoint *HookPoint, approvers eval.Approvers) error {
	policyReport := r.getPolicyReport(eventType)
	policyReport.Approvers = approvers
	return nil
}

func (r *Reporter) GetReport() *Report {
	return r.report
}

func NewReporter() *Reporter {
	return &Reporter{report: NewReport()}
}

type KProbeApplier struct {
	reporter Applier
	probe    *Probe
}

func (k *KProbeApplier) ApplyFilterPolicy(eventType string, tableName string, mode PolicyMode, flags PolicyFlag) error {
	log.Infof("Setting in-kernel filter policy to `%s` for `%s`", mode, eventType)

	k.reporter.ApplyFilterPolicy(eventType, tableName, mode, flags)
	return k.probe.SetFilterPolicy(tableName, mode, flags)
}

func (k *KProbeApplier) ApplyApprovers(eventType string, hookPoint *HookPoint, approvers eval.Approvers) error {
	k.reporter.ApplyApprovers(eventType, hookPoint, approvers)
	return hookPoint.OnNewApprovers(k.probe, approvers)
}

func (k *KProbeApplier) GetReport() *Report {
	return k.reporter.GetReport()
}

func (p *Probe) setKProbePolicy(hookPoint *HookPoint, rs *eval.RuleSet, eventType string, capabilities Capabilities, applier Applier) error {
	if !p.enableFilters {
		if err := applier.ApplyFilterPolicy(eventType, hookPoint.PolicyTable, POLICY_MODE_ACCEPT, math.MaxUint8); err != nil {
			return err
		}
		return nil
	}

	approvers, err := rs.GetApprovers(eventType, capabilities.GetFieldCapabilities())
	if err != nil {
		if err := applier.ApplyFilterPolicy(eventType, hookPoint.PolicyTable, POLICY_MODE_ACCEPT, math.MaxUint8); err != nil {
			return err
		}
		return nil
	}

	if err := applier.ApplyApprovers(eventType, hookPoint, approvers); err != nil {
		log.Errorf("Error while adding approvers fallback in-kernel policy to `%s` for `%s`: %s", POLICY_MODE_ACCEPT, eventType, err)
		if err := applier.ApplyFilterPolicy(eventType, hookPoint.PolicyTable, POLICY_MODE_ACCEPT, math.MaxUint8); err != nil {
			return err
		}
		return nil
	}

	if err := applier.ApplyFilterPolicy(eventType, hookPoint.PolicyTable, POLICY_MODE_DENY, capabilities.GetFlags()); err != nil {
		return err
	}

	return nil
}

func (p *Probe) ApplyRuleSet(rs *eval.RuleSet, dryRun bool) (*Report, error) {
	var applier Applier = NewReporter()
	if !dryRun {
		applier = &KProbeApplier{probe: p, reporter: applier}
	}

	already := make(map[*HookPoint]bool)

	if !p.enableFilters {
		log.Warn("Forcing in-kernel filter policy to `pass`: filtering not enabled")
	}

	for _, hookPoint := range AllHookPoints {
		if hookPoint.EventTypes == nil {
			continue
		}

		// first set policies
		for eventType, capabilities := range hookPoint.EventTypes {
			if eventType == "*" || rs.HasRulesForEventType(eventType) {
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
						active++

						log.Infof("kProbe `%s` registered", kprobe.Name)
						break
					}
					log.Debugf("failed to register kProbe `%s`", kprobe.Name)
				}

				if err != nil {
					if !hookPoint.Optional {
						return nil, err
					}
				}

				log.Infof("Hook Point `%s` registered with %d active kProbes", hookPoint.Name, active)
				already[hookPoint] = true
			}
		}
	}

	return applier.GetReport(), nil
}

func NewProbe(config *config.Config) (*Probe, error) {
	p := &Probe{
		config:           config,
		onDiscardersFncs: make(map[string][]onDiscarderFnc),
		enableFilters:    config.EnableKernelFilters,
		tables:           make(map[string]eprobe.Table),
	}

	p.Probe = &eprobe.Probe{
		Tables:   p.getTableNames(),
		PerfMaps: p.getPerfMaps(),
	}

	dentryResolver, err := NewDentryResolver(p.Probe)
	if err != nil {
		return nil, err
	}

	p.resolvers = &Resolvers{
		DentryResolver: dentryResolver,
	}

	return p, nil
}

// Snapshot - Snapshot runs the different snapshot functions of the resolvers that require to sync with the current
// state of the system
func (p *Probe) Snapshot() error {
	// Sync with the current mount points of the system
	if err := p.resolvers.MountResolver.SyncCache(0); err != nil {
		return errors.Wrap(err, "couldn't sync mount points of the host")
	}
	return nil
}

func init() {
	AllHookPoints = append(AllHookPoints, OpenHookPoints...)
	AllHookPoints = append(AllHookPoints, MountHookPoints...)
	AllHookPoints = append(AllHookPoints, ExecHookPoints...)
}
