package probe

import (
	"fmt"

	"github.com/iovisor/gobpf/bcc"
)

// EventMonitor - Security event monitor interface
type EventMonitor interface {
	Init(options *ProbeManagerOptions) error
	Run() error
	Stop() error
	GetMonitorType() EventMonitorType
	GetMonitorName() EventMonitorName
}

// EventMonitorType - Probe type
type EventMonitorType string

const (
	// EBPF - eBPF probe
	EBPF EventMonitorType = "ebpf"
	// Perf - Perf probe
	Perf EventMonitorType = "perf"
	// Container - container probe
	Container EventMonitorType = "container"
)

// EventMonitorName - Event Monitor names
type EventMonitorName string

const (
	// FimMonitor - eBPF FIM probe
	FimMonitor EventMonitorName = "fim"
	// ProcessMonitor - eBPF Process probe
	ProcessMonitor EventMonitorName = "process"
)

// EventMonitor - Generic eBPF event monitor structure
type GenericEventMonitor struct {
	Name       EventMonitorName
	Type       EventMonitorType
	TableNames []string
	Tables     map[string]*bcc.Table
	Options    *ProbeManagerOptions
	Source     string
	BCCModule  *bcc.Module
	Probes     []*Probe
}

// GetMonitorType - Returns event monitor type
func (em *GenericEventMonitor) GetMonitorType() EventMonitorType {
	return em.Type
}

// GetMonitorName - Returns the name of the event monitor
func (em *GenericEventMonitor) GetMonitorName() EventMonitorName {
	return em.Name
}

// Init - Initialize eBPF event monitor
func (em *GenericEventMonitor) Init(options *ProbeManagerOptions) error {
	em.Options = options
	em.Tables = make(map[string]*bcc.Table)
	if len(em.Source) == 0 {
		return ErrEmptySource
	}
	em.BCCModule = bcc.NewModule(
		em.Source,
		[]string{},
	)
	for _, probe := range em.Probes {
		if err := probe.init(em.BCCModule, options); err != nil {
			em.BCCModule.Close()
			return err
		}
	}
	return nil
}

// Run - Run event monitor
func (em *GenericEventMonitor) Run() error {
	// Prepare tables
	for _, tableName := range em.TableNames {
		_, ok := em.Tables[tableName]
		if ok {
			em.BCCModule.Close()
			return fmt.Errorf("%v has duplicate table name: %v", em.GetMonitorName(), tableName)
		}
		em.Tables[tableName] = bcc.NewTable(em.BCCModule.TableId(tableName), em.BCCModule)
		if em.Tables[tableName] == nil {
			em.BCCModule.Close()
			return fmt.Errorf("%v unknown table: %v", em.GetMonitorName(), tableName)
		}
	}

	// Start probes
	for _, probe := range em.Probes {
		if err := probe.run(em); err != nil {
			em.BCCModule.Close()
			return err
		}
	}
	fmt.Printf("%v event monitor is running ...", em.GetMonitorName())
	return nil
}

/*func (em *EventMonitor) ApplyPidFilter(options *pmodel.ProbeManagerOptions, pidTable string) error {
	options.RLock()
	defer options.RUnlock()
	table := em.Tables[pidTable]
	if table == nil {
		return fmt.Errorf("%v BPF_HASH table doesn't exist", pidTable)
	}
	// Setup filtering
	for _, pid := range options.Filters.InPids {
		key, _ := utils.InterfaceToBytes(pid, 4)
		if err := table.Set(key, []byte{1}); err != nil {
			return fmt.Errorf("couldn't set %v: %v", pidTable, err)
		}
	}
	for _, pid := range options.Filters.ExceptPids {
		key, _ := utils.InterfaceToBytes(pid, 4)
		if err := table.Set(key, []byte{0}); err != nil {
			return fmt.Errorf("couldn't set %v: %v", pidTable, err)
		}
	}
	return nil
}*/
/*
func (em *EventMonitor) ApplyPidnsFilter(options *pmodel.ProbeManagerOptions, pidnsTable string) error {
	options.RLock()
	defer options.RUnlock()
	table := em.Tables[pidnsTable]
	if table == nil {
		return fmt.Errorf("%v BPF_HASH table doesn't exist", pidnsTable)
	}
	// Setup filtering
	for _, pidns := range options.Filters.InPidns {
		key, _ := utils.InterfaceToBytes(pidns, 8)
		if err := table.Set(key, []byte{1}); err != nil {
			return fmt.Errorf("couldn't set %v: %v", pidnsTable, err)
		}
	}
	for _, pidns := range options.Filters.ExceptPidns {
		key, _ := utils.InterfaceToBytes(pidns, 8)
		if err := table.Set(key, []byte{0}); err != nil {
			return fmt.Errorf("couldn't set %v: %v", pidnsTable, err)
		}
	}
	return nil
}*/

/*func (em *EventMonitor) ApplyNewOptions(options *pmodel.ProbeManagerOptions) error {
	em.Options = options
	if em.FilterSetupFunc != nil {
		if err := em.FilterSetupFunc(options, em); err != nil {
			return err
		}
	}
	return nil
}*/

// Stop - Stop event monitor
func (em *GenericEventMonitor) Stop() error {
	for _, probe := range em.Probes {
		if err := probe.stop(); err != nil {
			fmt.Printf("%v probe stopped with error: %v", probe.Name, err)
		}
	}
	// Clean up all tables
	for _, table := range em.Tables {
		if table != nil {
			_ = table.DeleteAll()
		}
	}
	em.BCCModule.Close()
	fmt.Printf("%v event monitor stopped!", em.GetMonitorName())
	return nil
}

// EBPFProbeType - Probe type enum
type EBPFProbeType int

const (
	// TracepointType - Tracepoint probe
	TracepointType EBPFProbeType = iota
	// KProbeType - Kernel probe
	KProbeType
	// UProbeType - User probe
	UProbeType
)

// Probe - Genneric eBPF probe
type Probe struct {
	Name       string
	Type       EBPFProbeType
	EntryFunc  string
	EntryEvent string
	entryFd    int
	ExitFunc   string
	ExitEvent  string
	exitFd     int
	// Kprobe specific parameters
	KProbeMaxActive int
	// Perf maps
	PerfMaps []*PerfMap
}

// PerfMap - Definition of a perf map, used to bring data back to user space
type PerfMap struct {
	UserSpaceBufferLen  int
	PerfOutputTableName string
	eventTable          *bcc.Table
	perfMap             *bcc.PerfMap
	dataChan            chan []byte
	stopChan            chan bool
	stoppedChan         chan bool
	eventCache          *SafeCache
	DataHandler         func(data []byte, cache *SafeCache, em *GenericEventMonitor)
}

// init - Initializes perfmap members
func (m *PerfMap) init(options *ProbeManagerOptions) error {
	if m.DataHandler == nil {
		return ErrNoDataHandler
	}
	// Default userspace buffer length
	if m.UserSpaceBufferLen == 0 {
		m.UserSpaceBufferLen = options.ChannelBufferLength
	}
	m.dataChan = make(chan []byte, m.UserSpaceBufferLen)
	m.stopChan = make(chan bool, 1)
	m.stoppedChan = make(chan bool, 1)
	m.eventCache = NewSafeCache()
	return nil
}

func (m *PerfMap) run(em *GenericEventMonitor) error {
	module := em.BCCModule
	var err error
	m.eventTable = bcc.NewTable(module.TableId(m.PerfOutputTableName), module)
	m.perfMap, err = bcc.InitPerfMap(m.eventTable, m.dataChan)
	if err != nil {
		return fmt.Errorf("failed to start perf map: %s", err)
	}
	go m.listen(em)
	m.perfMap.Start()
	return nil
}

// listen - Listen for new events from the kernel
func (m *PerfMap) listen(em *GenericEventMonitor) {
	var data []byte
	for {
		select {
		case <-m.stopChan:
			m.stoppedChan <- true
			return
		case data = <-m.dataChan:
			m.DataHandler(data, m.eventCache, em)
		}
	}
}

// stop - Stop a perf map listener
func (m *PerfMap) stop() {
	m.stopChan <- true
	<-m.stoppedChan
	close(m.stopChan)
	close(m.stoppedChan)
	m.perfMap.Stop()
}

// init - Initializes a Probe
func (p *Probe) init(module *bcc.Module, options *ProbeManagerOptions) error {
	for _, m := range p.PerfMaps {
		if err := m.init(options); err != nil {
			return err
		}
	}

	// Load probe depending on its type
	var err error
	switch p.Type {
	case TracepointType:
		if p.EntryFunc != "" {
			if p.entryFd, err = module.LoadTracepoint(p.EntryFunc); err != nil {
				return fmt.Errorf("failed to load Tracepoint %v: %s", p.EntryFunc, err)
			}
		}
		if p.ExitFunc != "" {
			if p.exitFd, err = module.LoadTracepoint(p.ExitFunc); err != nil {
				return fmt.Errorf("failed to load Tracepoint %v: %s", p.ExitFunc, err)
			}
		}
	case KProbeType:
		if p.EntryFunc != "" {
			if p.entryFd, err = module.LoadKprobe(p.EntryFunc); err != nil {
				return fmt.Errorf("failed to load Kprobe %v: %s", p.EntryFunc, err)
			}
		}
		if p.ExitFunc != "" {
			if p.exitFd, err = module.LoadKprobe(p.ExitFunc); err != nil {
				return fmt.Errorf("failed to load Kprobe %v: %s", p.ExitFunc, err)
			}
		}
	default:
		return ErrUnknownProbeType
	}
	return nil
}

func (p *Probe) run(em *GenericEventMonitor) error {
	module := em.BCCModule
	var err error
	switch p.Type {
	case TracepointType:
		if p.EntryEvent != "" {
			if err = module.AttachTracepoint(p.EntryEvent, p.entryFd); err != nil {
				return fmt.Errorf("failed to attach Tracepoint %v: %s", p.EntryEvent, err)
			}
		}
		if p.ExitEvent != "" {
			if err = module.AttachTracepoint(p.ExitEvent, p.exitFd); err != nil {
				return fmt.Errorf("failed to attach Tracepoint %v: %s", p.ExitEvent, err)
			}
		}
	case KProbeType:
		if p.EntryEvent != "" {
			if err = module.AttachKprobe(p.EntryEvent, p.entryFd, p.KProbeMaxActive); err != nil {
				return fmt.Errorf("failed to attach Kprobe %v: %s", p.EntryEvent, err)
			}
		}
		if p.ExitEvent != "" {
			if err = module.AttachKretprobe(p.ExitEvent, p.exitFd, p.KProbeMaxActive); err != nil {
				return fmt.Errorf("failed to attach KRetprobe %v: %s", p.ExitEvent, err)
			}
		}
	}
	for _, m := range p.PerfMaps {
		if err := m.run(em); err != nil {
			return err
		}
	}
	return nil
}

// stop - Stop all running listeners and perf map
func (p *Probe) stop() error {
	// Stop eBPF event listeners
	for _, m := range p.PerfMaps {
		m.stop()
	}
	return nil
}
