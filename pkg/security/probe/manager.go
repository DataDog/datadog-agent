package probe

import (
	"fmt"
	"sync"
	"time"

	"github.com/shirou/gopsutil/host"
)

var Manager *ProbeManager

// NewProbeManager - Creates a new Probe Manager
func NewProbeManager(options ProbeManagerOptions, emList []EventMonitor) *ProbeManager {
	return &ProbeManager{
		Options: &options,
		emList:  emList,
	}
}

// ProbeManager - Security probe manager struct
type ProbeManager struct {
	sync.RWMutex
	// emList - List of available event monitor
	emList []EventMonitor
	// Probes - List of registered event monitors
	EventMonitors map[EventMonitorName]EventMonitor
	// Options - Probe Manager options
	Options *ProbeManagerOptions
	// subscribersList - List of available subscribers
	//sList []pmodel.EventSubscriber
	// MonitorSubscribers - Monitor subscribers
	//MonitorSubscribers map[EventMonitorName][]EventSubscriber
	// EventSubscribers - Probe event subscribers
	//EventSubscribers map[pmodel.ProbeEventType][]pmodel.EventSubscriber
	// useSecurityModule - Internal flag used to route packets through the security
	// module instead of sending them directly to the right subscribers. This flag is set
	// by the configuration.
	//useSecurityModule *bool
	// BootTime - Time of boot (used to calculate timestamps)
	BootTime time.Time

	// Cache - The probe manager cache is used to enrich and route events from the probes to
	// the subscribers. Events arrive in an unpredictable order, and in some cases this can be
	// an issue (example: assessing a syscall event from an unknown process). This cache is used
	// to track the processes and namespaces that are known so that events can be enriched (with
	// their security profile for example) before being routed to the security module and / or
	// other subscribers.
	//Cache *cache.Cache
}

const (
	// ErrInitSubscriber - Subscriber init error msg
	ErrInitSubscriber = "couldn't init subscriber %v: %v"
	// ErrInitEventMonitor - Probe init error msg
	ErrInitEventMonitor = "couldn't init event monitor %v (%v): %v"
	// ErrBootTime - Boot time error
	ErrBootTime = "couldn't get boot time: %v"
)

// init - Initializes the probe manager
func (pm *ProbeManager) init() error {
	// Prepare probe manager
	bt, err := host.BootTime()
	if err != nil {
		return fmt.Errorf(ErrBootTime, err)
	}
	pm.BootTime = time.Unix(int64(bt), 0)
	//pm.useSecurityModule = &pm.Options.UseSecurityModule
	//pm.Cache = cache.NewCache(pm.Options, utils.GetPidnsFromPid(1))
	// Load subscribers
	//pm.MonitorSubscribers = make(map[pmodel.EventMonitorName][]pmodel.EventSubscriber)
	//pm.EventSubscribers = make(map[pmodel.ProbeEventType][]pmodel.EventSubscriber)
	/*for _, sub := range pm.sList {
		if err := sub.Init(pm.Options); err != nil {
			utils.Error.Printf(ErrInitSubscriber, sub.GetSubscriberName(), err)
		}
	}*/
	// Load event monitors
	pm.EventMonitors = make(map[EventMonitorName]EventMonitor)
	for _, em := range pm.emList {
		if err := pm.registerEventMonitor(em); err != nil {
			fmt.Printf(ErrInitEventMonitor, em.GetMonitorName(), em.GetMonitorType(), err)
		}
	}
	if len(pm.EventMonitors) == 0 {
		return fmt.Errorf("no event monitor to start")
	}
	return nil
}

// registerEventMonitor - Run an EventMonitor Init function and add it to the list of registered event monitors
func (pm *ProbeManager) registerEventMonitor(em EventMonitor) error {
	if err := em.Init(pm.Options); err != nil {
		return err
	}
	pm.EventMonitors[em.GetMonitorName()] = em
	return nil
}

const (
	// ErrStartSubscriber - Subscriber start error msg
	ErrStartSubscriber = "couldn't start subscriber %v: %v"
	// ErrStartEventMonitor - Probe start error msg
	ErrStartEventMonitor = "couldn't start event monitor %v (%v): %v"
	// ErrListRunningProcesses - List running processes error msg
	ErrListRunningProcesses = "couldn't list running processes: %v"
)

// Start - Start all registered event monitors & subscribers
func (pm *ProbeManager) Start() error {
	// Initialize the probe manager
	if err := pm.init(); err != nil {
		return err
	}

	// Start the event monitors
	for _, em := range pm.EventMonitors {
		go func(p EventMonitor) {
			if err := p.Run(); err != nil {
				fmt.Printf(ErrStartEventMonitor, p.GetMonitorName(), p.GetMonitorType(), err)
			}
		}(em)
	}

	// List running processes
	/*if err := ListRunningProcesses(pm); err != nil {
		fmt.Printf(ErrListRunningProcesses, err)
	}*/
	return nil
}

const (
	// ErrStopSubscriber - Subscriber stop error msg
	ErrStopSubscriber = "error stopping subscriber %v: %v"
	// ErrStopEventMonitor - Event monitor stop error msg
	ErrStopEventMonitor = "error stopping event monitor %v (%v): %v"
)

// Stop - Stop all registered event monitors & subscribers
func (pm *ProbeManager) Stop() error {
	// Stop event monitors
	for _, p := range pm.EventMonitors {
		if err := p.Stop(); err != nil {
			fmt.Printf(ErrStopEventMonitor, p.GetMonitorName(), p.GetMonitorType(), err)
		}
	}
	return nil
}

// DispatchEvent - Processes the event. Depending on how the probe manager was configured,
// the event might be delayed, routed to the security module or dispatched directly to the
// event subscribers. This should be the default endpoint used by a monitor.
func (pm *ProbeManager) DispatchEvent(event ProbeEvent) error {
	fmt.Printf("################: %+v\n", event)

	/*// Some events require preprocessing to update the probe manager filter
	switch event.GetEventType() {
	case pmodel.ContainerRunningEventType:
		pm.UpdateContainerFilter(event.(*pmodel.ContainerEvent))
	case pmodel.ProcessExecEventType:
		pm.UpdateProcessFilter(event.(*pmodel.ExecveEvent))
	case pmodel.ProcessForkEventType:
		pm.UpdateProcessFilterFork(event.(*pmodel.ForkEvent))
	}

	fmt.Printf("###################: %+v\n", pm.Cache.EnrichEvent(event))
	// Check if the kernel filters are applied && if this event isn't being delayed by the
	// security module.
	if pm.Cache.EnrichEvent(event) && !event.HasRoutingFlag(pmodel.SecurityDelayedEventFlag) {
		// This event shall be routed to its next processor. This might be the security
		// module or the subscribers straight away.
		fmt.Printf("ZZZZZZZZZZZZZZZZ: %+v\n", pm.useSecurityModule)
		if *pm.useSecurityModule {
			// This event shall be sent to the security module. The security module will
			// enrich the event with a security assessment before routing it to the relevant
			// subscribers.
			// NOTE: Routing flag events should go to the security module and be filtered out
			// there!
			// Add Context data
			pm.Cache.AddContextData(event)
			return pm.securityDispatch(event)
		}
		// If this event has the CacheDataFlag, then only route it to the delayed events
		// subscribers. See the flag definition for more information.
		if event.HasRoutingFlag(pmodel.CacheDataFlag) {
			return pm.DelayerDispatch(event)
		}
		// Add Context data
		pm.Cache.AddContextData(event)
		// Route this event directly to its subscribers.
		return pm.SubscriberDispatch(event)
	}
	// This event needs to be delayed until the filters are resolved. Set the correct routing
	// flag and send it to the delayer.
	if !event.HasRoutingFlag(pmodel.SecurityDelayedEventFlag) {
		event.AddRoutingFlag(pmodel.UnfilteredEventFlag)
	}
	return pm.DelayerDispatch(event)*/

	return nil
}
