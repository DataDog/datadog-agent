package probe

import (
	"bytes"
	"fmt"
	"math"
	"strings"
	"sync/atomic"

	"github.com/iovisor/gobpf/elf"

	"github.com/DataDog/datadog-agent/pkg/ebpf/gobpf"
	eprobe "github.com/DataDog/datadog-agent/pkg/ebpf/probe"
	"github.com/DataDog/datadog-agent/pkg/ebpf/probe/types"
	"github.com/DataDog/datadog-agent/pkg/security/config"
	"github.com/DataDog/datadog-agent/pkg/security/secl/eval"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

type EventHandler interface {
	HandleEvent(event *Event)
}

type Stats struct {
	Events struct {
		Lost     uint64
		Received uint64
	}
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
	handler          EventHandler
	resolvers        *Resolvers
	stats            Stats
	onDiscardersFncs map[string][]onDiscarderFnc
	enableFilters    bool
	tables           map[string]eprobe.Table
}

type Capability struct {
	PolicyFlags     PolicyFlag
	FieldValueTypes eval.FieldValueType
}

type Capabilities map[string]Capability

type KProbe struct {
	KProbe          *eprobe.KProbe
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

func syscallKprobe(name string) *eprobe.KProbe {
	return &eprobe.KProbe{
		Name:      "sys_" + name,
		EntryFunc: "kprobe/" + getSyscallFnName(name),
		ExitFunc:  "kretprobe/" + getSyscallFnName(name),
	}
}

var AllKProbes = []*KProbe{
	{
		KProbe: &eprobe.KProbe{
			Name:      "security_path_chmod",
			EntryFunc: "kprobe/security_path_chmod",
		},
		EventTypes: map[string]Capabilities{
			"chmod": Capabilities{},
		},
	},
	{
		KProbe: syscallKprobe("chmod"),
		EventTypes: map[string]Capabilities{
			"chmod": Capabilities{},
		},
	},
	{
		KProbe: syscallKprobe("fchmod"),
		EventTypes: map[string]Capabilities{
			"chmod": Capabilities{},
		},
	},
	{
		KProbe: syscallKprobe("fchmodat"),
		EventTypes: map[string]Capabilities{
			"chmod": Capabilities{},
		},
	},
	{
		KProbe: &eprobe.KProbe{
			Name:      "security_path_chown",
			EntryFunc: "kprobe/security_path_chown",
		},
		EventTypes: map[string]Capabilities{
			"chown": Capabilities{},
		},
	},
	{
		KProbe: syscallKprobe("chown"),
		EventTypes: map[string]Capabilities{
			"chown": Capabilities{},
		},
	},
	{
		KProbe: syscallKprobe("fchown"),
		EventTypes: map[string]Capabilities{
			"chown": Capabilities{},
		},
	},
	{
		KProbe: syscallKprobe("fchownat"),
		EventTypes: map[string]Capabilities{
			"chown": Capabilities{},
		},
	},
	{
		KProbe: syscallKprobe("lchown"),
		EventTypes: map[string]Capabilities{
			"chown": Capabilities{},
		},
	},
	{
		KProbe: &eprobe.KProbe{
			Name: "utimes_common",
			// TODO: switch to the new eBPF library. `isra.0` might be needed on your kernel for this probe to work.
			// adding `isra.0` now will fail anyway because the current eBPF lib doesn't properly sanitize the kprobe
			// events names.
			EntryFunc: "kprobe/utimes_common",
		},
		EventTypes: map[string]Capabilities{
			"utimes": Capabilities{},
		},
	},
	{
		KProbe: syscallKprobe("utime"),
		EventTypes: map[string]Capabilities{
			"utimes": Capabilities{},
		},
	},
	{
		KProbe: syscallKprobe("utimes"),
		EventTypes: map[string]Capabilities{
			"utimes": Capabilities{},
		},
	},
	{
		KProbe: syscallKprobe("utimensat"),
		EventTypes: map[string]Capabilities{
			"utimes": Capabilities{},
		},
	},
	{
		KProbe: syscallKprobe("futimesat"),
		EventTypes: map[string]Capabilities{
			"utimes": Capabilities{},
		},
	},
	{
		KProbe: syscallKprobe("mkdir"),
		EventTypes: map[string]Capabilities{
			"mkdir": Capabilities{},
		},
	},
	{
		KProbe: syscallKprobe("mkdirat"),
		EventTypes: map[string]Capabilities{
			"mkdir": Capabilities{},
		},
	},
	{
		KProbe: &eprobe.KProbe{
			Name:      "security_path_mkdir",
			EntryFunc: "kprobe/security_path_mkdir",
		},
		EventTypes: map[string]Capabilities{
			"mkdir": Capabilities{},
		},
	},
	{
		KProbe: syscallKprobe("rmdir"),
		EventTypes: map[string]Capabilities{
			"rmdir": Capabilities{},
		},
	},
	{
		KProbe: &eprobe.KProbe{
			Name:      "security_path_rmdir",
			EntryFunc: "kprobe/security_path_rmdir",
		},
		EventTypes: map[string]Capabilities{
			"rmdir": Capabilities{},
		},
	},
	{
		KProbe: syscallKprobe("unlink"),
		EventTypes: map[string]Capabilities{
			"unlink": Capabilities{},
		},
	},
	{
		KProbe: syscallKprobe("unlinkat"),
		EventTypes: map[string]Capabilities{
			"unlink": Capabilities{},
		},
	},
	{
		KProbe: &eprobe.KProbe{
			Name:      "security_path_unlink",
			EntryFunc: "kprobe/security_path_unlink",
		},
		EventTypes: map[string]Capabilities{
			"unlink": Capabilities{},
		},
	},
	{
		KProbe: syscallKprobe("rename"),
		EventTypes: map[string]Capabilities{
			"rename": Capabilities{},
		},
	},
	{
		KProbe: syscallKprobe("renameat"),
		EventTypes: map[string]Capabilities{
			"rename": Capabilities{},
		},
	},
	{
		KProbe: syscallKprobe("renameat2"),
		EventTypes: map[string]Capabilities{
			"rename": Capabilities{},
		},
	},
	{
		KProbe: &eprobe.KProbe{
			Name:      "security_path_rename",
			EntryFunc: "kprobe/security_path_rename",
		},
		EventTypes: map[string]Capabilities{
			"rename": Capabilities{},
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
	if err := p.Load(); err != nil {
		return err
	}

	if err := p.resolvers.Start(); err != nil {
		return err
	}

	for _, kprobe := range AllKProbes {
		if kprobe.EventTypes == nil {
			continue
		}

		for eventType := range kprobe.EventTypes {
			if kprobe.OnNewDiscarders != nil {
				fncs := p.onDiscardersFncs[eventType]
				fncs = append(fncs, kprobe.OnNewDiscarders)
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

func (p *Probe) GetStats() Stats {
	return p.stats
}

func (p *Probe) ResetStats() {
	p.stats = Stats{}
}

func (p *Probe) handleLostEvents(count uint64) {
	log.Warnf("Lost %d events\n", count)
	atomic.AddUint64(&p.stats.Events.Lost, count)
}

func (p *Probe) handleEvent(data []byte) {
	log.Debugf("Handling dentry event (len %d)", len(data))
	atomic.AddUint64(&p.stats.Events.Received, 1)

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

	switch ProbeEventType(event.Event.Type) {
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
		log.Errorf("Unsupported event type %d\n", event.Event.Type)
	}

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

	key := Int32ToKey(0)

	policy := FilterPolicy{
		Mode:  mode,
		Flags: flags,
	}
	return table.Set(key, policy.Bytes())
}

func (p *Probe) setKProbePolicy(kprobe *KProbe, rs *eval.RuleSet, eventType string, capabilities Capabilities) error {
	if !p.enableFilters {
		if err := p.SetFilterPolicy(kprobe.PolicyTable, POLICY_MODE_ACCEPT, math.MaxUint8); err != nil {
			return err
		}
		return nil
	}

	approvers, err := rs.GetApprovers(eventType, capabilities.GetFieldCapabilities())
	if err != nil {
		log.Infof("Setting in-kernel filter policy to `PASS` for `%s`: no approver", eventType)
		if err := p.SetFilterPolicy(kprobe.PolicyTable, POLICY_MODE_ACCEPT, math.MaxUint8); err != nil {
			return err
		}
		return nil
	}

	log.Debugf("Approver discovered: %+v\n", approvers)

	if err := kprobe.OnNewApprovers(p, approvers); err != nil {
		log.Errorf("Error while adding approvers set in-kernel policy to `PASS` for `%s`: %s", eventType, err)
		if err := p.SetFilterPolicy(kprobe.PolicyTable, POLICY_MODE_ACCEPT, math.MaxUint8); err != nil {
			return err
		}
		return nil
	}

	log.Infof("Setting in-kernel filter policy to `DENY` for `%s`", eventType)
	if err := p.SetFilterPolicy(kprobe.PolicyTable, POLICY_MODE_DENY, capabilities.GetFlags()); err != nil {
		return err
	}

	return nil
}

func (p *Probe) ApplyRuleSet(rs *eval.RuleSet) error {
	already := make(map[*KProbe]bool)

	if !p.enableFilters {
		log.Warn("Forcing in-kernel filter policy to `pass`: filtering not enabled")
	}

	for _, kprobe := range AllKProbes {
		for eventType, capabilities := range kprobe.EventTypes {
			if rs.HasRulesForEventType(eventType) || eventType == "*" {
				if _, ok := already[kprobe]; !ok {
					if err := p.Module.RegisterKprobe(kprobe.KProbe); err != nil {
						return err
					}
					already[kprobe] = true
				}

		// first set policies
		for eventType, capabilities := range kprobe.EventTypes {
			if eventType == "*" || rs.HasRulesForEventType(eventType) {
				if kprobe.PolicyTable == "" {
					continue
				}

				if err := p.setKProbePolicy(kprobe, rs, eventType, capabilities); err != nil {
					return err
				}
			}
		}

		// then register kprobes
		for eventType := range kprobe.EventTypes {
			if eventType == "*" || rs.HasRulesForEventType(eventType) {
				if _, ok := already[kprobe]; !ok {
					log.Infof("Register kProbe `%s`", kprobe.KProbe.Name)
					if err := p.Module.RegisterKprobe(kprobe.KProbe); err != nil {
						return err
					}
					already[kprobe] = true
				}
			}
		}
	}

	return nil
}

func NewProbe(config *config.Config) (*Probe, error) {
	p := &Probe{
		onDiscardersFncs: make(map[string][]onDiscarderFnc),
		enableFilters:    config.EnableKernelFilters,
		tables:           make(map[string]eprobe.Table),
	}

	asset := "pkg/security/ebpf/probe"
	openSyscall := getSyscallFnName("open")
	if !strings.HasPrefix(openSyscall, "SyS_") && !strings.HasPrefix(openSyscall, "sys_") {
		asset += "-syscall-wrapper"
	}

	bytecode, err := Asset(asset + ".o") // ioutil.ReadFile("pkg/security/ebpf/probe.o")
	if err != nil {
		return nil, err
	}

	module, err := gobpf.NewModuleFromReader(bytes.NewReader(bytecode))
	if err != nil {
		return nil, err
	}

	p.Probe = &eprobe.Probe{
		Module:   module,
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
	AllKProbes = append(AllKProbes, OpenKProbes...)
	AllKProbes = append(AllKProbes, MountProbes...)
}
