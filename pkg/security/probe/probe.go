package probe

import (
	"bytes"
	"fmt"
	"math"
	"strings"

	"github.com/pkg/errors"

	"github.com/iovisor/gobpf/elf"

	"github.com/DataDog/datadog-agent/pkg/security/config"
	"github.com/DataDog/datadog-agent/pkg/security/ebpf"
	"github.com/DataDog/datadog-agent/pkg/security/rules"
	"github.com/DataDog/datadog-agent/pkg/security/secl/eval"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/datadog-go/statsd"
)

const MetricPrefix = "datadog.agent.runtime_security"

type EventHandler interface {
	HandleEvent(event *Event)
}

type KTable struct {
	Name string
}

type Discarder struct {
	Field eval.Field
	Value interface{}
}

type onApproversFnc func(probe *Probe, approvers rules.Approvers) error
type onDiscarderFnc func(probe *Probe, discarder Discarder) error

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

type Capability struct {
	PolicyFlags     PolicyFlag
	FieldValueTypes eval.FieldValueType
}

type Capabilities map[eval.Field]Capability

type HookPoint struct {
	Name            string
	KProbes         []*ebpf.KProbe
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

var AllHookPoints = []*HookPoint{
	{
		Name: "security_inode_setattr",
		KProbes: []*ebpf.KProbe{{
			EntryFunc: "kprobe/security_inode_setattr",
		}},
		EventTypes: map[eval.EventType]Capabilities{
			"chmod":  Capabilities{},
			"chown":  Capabilities{},
			"utimes": Capabilities{},
		},
	},
	{
		Name:    "sys_chmod",
		KProbes: syscallKprobe("chmod"),
		EventTypes: map[eval.EventType]Capabilities{
			"chmod": Capabilities{},
		},
	},
	{
		Name:    "sys_fchmod",
		KProbes: syscallKprobe("fchmod"),
		EventTypes: map[eval.EventType]Capabilities{
			"chmod": Capabilities{},
		},
	},
	{
		Name:    "sys_fchmodat",
		KProbes: syscallKprobe("fchmodat"),
		EventTypes: map[eval.EventType]Capabilities{
			"chmod": Capabilities{},
		},
	},
	{
		Name:    "sys_chown",
		KProbes: syscallKprobe("chown"),
		EventTypes: map[eval.EventType]Capabilities{
			"chown": Capabilities{},
		},
	},
	{
		Name:    "sys_fchown",
		KProbes: syscallKprobe("fchown"),
		EventTypes: map[eval.EventType]Capabilities{
			"chown": Capabilities{},
		},
	},
	{
		Name:    "sys_fchownat",
		KProbes: syscallKprobe("fchownat"),
		EventTypes: map[eval.EventType]Capabilities{
			"chown": Capabilities{},
		},
	},
	{
		Name:    "sys_lchown",
		KProbes: syscallKprobe("lchown"),
		EventTypes: map[eval.EventType]Capabilities{
			"chown": Capabilities{},
		},
	},
	{
		Name: "mnt_want_write",
		KProbes: []*ebpf.KProbe{{
			EntryFunc: "kprobe/mnt_want_write",
		}},
		EventTypes: map[eval.EventType]Capabilities{
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
		KProbes: []*ebpf.KProbe{{
			EntryFunc: "kprobe/mnt_want_write_file",
		}},
		EventTypes: map[eval.EventType]Capabilities{
			"chown": Capabilities{},
		},
	},
	{
		Name:    "sys_utime",
		KProbes: syscallKprobe("utime"),
		EventTypes: map[eval.EventType]Capabilities{
			"utimes": Capabilities{},
		},
	},
	{
		Name:    "sys_utimes",
		KProbes: syscallKprobe("utimes"),
		EventTypes: map[eval.EventType]Capabilities{
			"utimes": Capabilities{},
		},
	},
	{
		Name:    "sys_utimensat",
		KProbes: syscallKprobe("utimensat"),
		EventTypes: map[eval.EventType]Capabilities{
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
		KProbes: []*ebpf.KProbe{{
			EntryFunc: "kprobe/vfs_mkdir",
		}},
		EventTypes: map[eval.EventType]Capabilities{
			"mkdir": Capabilities{},
		},
	},
	{
		Name: "filename_create",
		KProbes: []*ebpf.KProbe{{
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
		EventTypes: map[eval.EventType]Capabilities{
			"mkdir": Capabilities{},
		},
	},
	{
		Name:    "sys_mkdirat",
		KProbes: syscallKprobe("mkdirat"),
		EventTypes: map[eval.EventType]Capabilities{
			"mkdir": Capabilities{},
		},
	},
	{
		Name: "vfs_rmdir",
		KProbes: []*ebpf.KProbe{{
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
		EventTypes: map[eval.EventType]Capabilities{
			"rmdir": Capabilities{},
		},
	},
	{
		Name: "vfs_unlink",
		KProbes: []*ebpf.KProbe{{
			EntryFunc: "kprobe/vfs_unlink",
		}},
		EventTypes: map[eval.EventType]Capabilities{
			"unlink": Capabilities{},
		},
	},
	{
		Name:    "sys_unlink",
		KProbes: syscallKprobe("unlink"),
		EventTypes: map[eval.EventType]Capabilities{
			"unlink": Capabilities{},
		},
	},
	{
		Name:    "sys_unlinkat",
		KProbes: syscallKprobe("unlinkat"),
		EventTypes: map[eval.EventType]Capabilities{
			"unlink": Capabilities{},
		},
	},
	{
		Name: "vfs_rename",
		KProbes: []*ebpf.KProbe{{
			EntryFunc: "kprobe/vfs_rename",
		}},
		EventTypes: map[eval.EventType]Capabilities{
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
		EventTypes: map[eval.EventType]Capabilities{
			"rename": Capabilities{},
		},
	},
	{
		Name:    "sys_renameat2",
		KProbes: syscallKprobe("renameat2"),
		EventTypes: map[eval.EventType]Capabilities{
			"rename": Capabilities{},
		},
	},
	{
		Name: "vfs_link",
		KProbes: []*ebpf.KProbe{{
			EntryFunc: "kprobe/vfs_link",
		}},
		EventTypes: map[string]Capabilities{
			"link": Capabilities{},
		},
	},
	{
		Name:    "sys_link",
		KProbes: syscallKprobe("link"),
		EventTypes: map[eval.EventType]Capabilities{
			"link": Capabilities{},
		},
	},
	{
		Name:    "sys_linkat",
		KProbes: syscallKprobe("linkat"),
		EventTypes: map[eval.EventType]Capabilities{
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

func (caps Capabilities) GetField() []eval.Field {
	var fields []eval.Field

	for field := range caps {
		fields = append(fields, field)
	}

	return fields
}

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

func (p *Probe) NewRuleSet(opts *eval.Opts) *rules.RuleSet {
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

	return append(tables, OpenTables...)
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

func (p *Probe) Start() error {
	asset := "pkg/security/ebpf/c/probe"
	openSyscall := getSyscallFnName("open")
	if !strings.HasPrefix(openSyscall, "SyS_") && !strings.HasPrefix(openSyscall, "sys_") {
		asset += "-syscall-wrapper"
	}

	bytecode, err := Asset(asset + ".o") // ioutil.ReadFile("pkg/security/ebpf/c/probe.o")
	if err != nil {
		return err
	}

	p.Module, err = ebpf.NewModuleFromReader(bytes.NewReader(bytecode))
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

	return p.Probe.Start()
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

	if err := statsdClient.Count(MetricPrefix+".events.lost", p.eventsStats.GetAndResetLost(), nil, 1.0); err != nil {
		return err
	}

	if err := statsdClient.Count(MetricPrefix+".events.received", p.eventsStats.GetAndResetReceived(), nil, 1.0); err != nil {
		return err
	}

	for i := range p.eventsStats.PerEventType {
		if i == 0 {
			continue
		}

		eventType := ProbeEventType(i)
		key := MetricPrefix + ".events." + eventType.String()
		if err := statsdClient.Count(key, p.eventsStats.GetAndResetEventCount(eventType), nil, 1.0); err != nil {
			return err
		}
	}

	return nil
}

// GetStats - return Stats according to the system-probe module format
func (p *Probe) GetStats() (map[string]interface{}, error) {
	stats := make(map[string]interface{})

	syscalls, err := p.syscallMonitor.GetStats()

	stats["events"] = map[string]interface{}{
		"received": p.eventsStats.GetReceived(),
		"lost":     p.eventsStats.GetLost(),
		"syscalls": syscalls,
	}

	perEventType := make(map[string]int64)
	stats["per_event_type"] = perEventType
	for i := range p.eventsStats.PerEventType {
		if i == 0 {
			continue
		}

		eventType := ProbeEventType(i)
		perEventType[eventType.String()] = p.eventsStats.GetEventCount(eventType)
	}

	return stats, err
}

func (p *Probe) GetEventsStats() EventsStats {
	return p.eventsStats
}

func (p *Probe) handleLostEvents(count uint64) {
	log.Warnf("Lost %d events\n", count)
	p.eventsStats.CountLost(int64(count))
}

func (p *Probe) handleEvent(data []byte) {
	log.Debugf("Handling dentry event (len %d)", len(data))
	p.eventsStats.CountReceived(1)

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
		log.Errorf("Unsupported event type %d\n", eventType)
		return
	}

	p.eventsStats.CountEventType(eventType, 1)

	log.Debugf("Dispatching event %+v\n", event)
	p.DispatchEvent(event)
}

func (p *Probe) OnNewDiscarder(event *Event, field eval.Field) error {
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
	Approvers rules.Approvers
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
	ApplyFilterPolicy(eventType eval.EventType, tableName string, mode PolicyMode, flags PolicyFlag) error
	ApplyApprovers(eventType eval.EventType, hook *HookPoint, approvers rules.Approvers) error
	GetReport() *Report
}

type Reporter struct {
	report *Report
}

func (r *Reporter) getPolicyReport(eventType eval.EventType) *PolicyReport {
	if r.report.Policies[eventType] == nil {
		r.report.Policies[eventType] = &PolicyReport{Approvers: rules.Approvers{}}
	}
	return r.report.Policies[eventType]
}

func (r *Reporter) ApplyFilterPolicy(eventType eval.EventType, tableName string, mode PolicyMode, flags PolicyFlag) error {
	policyReport := r.getPolicyReport(eventType)
	policyReport.Mode = mode
	policyReport.Flags = flags
	return nil
}

func (r *Reporter) ApplyApprovers(eventType eval.EventType, hookPoint *HookPoint, approvers rules.Approvers) error {
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

func (k *KProbeApplier) ApplyFilterPolicy(eventType eval.EventType, tableName string, mode PolicyMode, flags PolicyFlag) error {
	log.Infof("Setting in-kernel filter policy to `%s` for `%s`", mode, eventType)

	k.reporter.ApplyFilterPolicy(eventType, tableName, mode, flags)
	return k.probe.SetFilterPolicy(tableName, mode, flags)
}

func (k *KProbeApplier) ApplyApprovers(eventType eval.EventType, hookPoint *HookPoint, approvers rules.Approvers) error {
	k.reporter.ApplyApprovers(eventType, hookPoint, approvers)
	return hookPoint.OnNewApprovers(k.probe, approvers)
}

func (k *KProbeApplier) GetReport() *Report {
	return k.reporter.GetReport()
}

func (p *Probe) setKProbePolicy(hookPoint *HookPoint, rs *rules.RuleSet, eventType eval.EventType, capabilities Capabilities, applier Applier) error {
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

func (p *Probe) ApplyRuleSet(rs *rules.RuleSet, dryRun bool) (*Report, error) {
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

// Snapshot - Snapshot runs the different snapshot functions of the resolvers that require to sync with the current
// state of the system
func (p *Probe) Snapshot() error {
	// Sync with the current mount points of the system
	if err := p.resolvers.MountResolver.SyncCache(0); err != nil {
		return errors.Wrap(err, "couldn't sync mount points of the host")
	}
	return nil
}

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

	dentryResolver, err := NewDentryResolver(p.Probe)
	if err != nil {
		return nil, err
	}

	p.resolvers = &Resolvers{
		DentryResolver: dentryResolver,
		MountResolver:  NewMountResolver(),
	}

	return p, nil
}

func init() {
	AllHookPoints = append(AllHookPoints, OpenHookPoints...)
	AllHookPoints = append(AllHookPoints, MountHookPoints...)
	AllHookPoints = append(AllHookPoints, ExecHookPoints...)
}
