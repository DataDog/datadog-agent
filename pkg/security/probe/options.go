package probe

import (
	"sync"
)

// ProbeManagerOptions - Probe Manager options
type ProbeManagerOptions struct {
	sync.RWMutex
	// Probes - List of probes to activate
	Probes []string `default:""`
	// Trace - List of probes to trace
	Trace []string `default:""`
	// DDTrace - List of events to send to Datadog
	DDTrace []string `default:""`
	// DDTraceMonitor - List of monitor which events will be sent to Datadog
	DDTraceMonitor []string `default:""`
	// Filters - Custom event filters
	//Filters *Filters
	// ChannelBufferLength - The default buffer length for Go channels used internally
	ChannelBufferLength int `default:"4096"`
	// ProcessInfoCacheSize - Process cache size
	ProcessCacheSize uint `default:"65536"`
	// UseSecurityModule - Use the security dispatcher to route events to subscribers,
	// thus allowing the security module to enrich the event with a threat assessment.
	UseSecurityModule bool `default:"true"`
	// DDLogServer - Events will be sent to Datadog through this log server (exposed by the agent)
	DDLogServer string `default:"127.0.0.1:10518"`
	// SecurityProfiles - Default set of profiles loaded during initialization
	SecurityProfiles struct {
		Enabled bool   `default:"false"`
		Path    string `default:"src/config/profiles"`
	}
	// SecurityPolicies - Sets of security policies to monitor
	SecurityPolicies struct {
		Enabled bool   `default:"false"`
		Path    string `default:"src/config/policies/policies.conf"`
	}
}

/*
func (pmo ProbeManagerOptions) String() string {
	c, err := json.MarshalIndent(pmo, "", "    ")
	if err != nil {
		return ""
	}
	return string(c)
}

// SafeAppendInPidFilter - Append a in pid filter safely
func (pmo *ProbeManagerOptions) SafeAppendInPidFilter(pid uint32) {
	pmo.Lock()
	pmo.Filters.InPids = append(pmo.Filters.InPids, pid)
	pmo.Unlock()
}

// SafeAppendInPidnsFilter - Append a in pidns filter safely
func (pmo *ProbeManagerOptions) SafeAppendInPidnsFilter(pidns uint64) {
	pmo.Lock()
	pmo.Filters.InPidns = append(pmo.Filters.InPidns, pidns)
	pmo.Unlock()
}

// SafeAppendExceptPidFilter - Append a except pid filter safely
func (pmo *ProbeManagerOptions) SafeAppendExceptPidFilter(pid uint32) {
	pmo.Lock()
	pmo.Filters.ExceptPids = append(pmo.Filters.ExceptPids, pid)
	pmo.Unlock()
}

// SafeAppendExceptPidnsFilter - Append a except pidns filter safely
func (pmo *ProbeManagerOptions) SafeAppendExceptPidnsFilter(pidns uint64) {
	pmo.Lock()
	pmo.Filters.ExceptPidns = append(pmo.Filters.ExceptPidns, pidns)
	pmo.Unlock()
}

// SafeRemoveInPidFilter - Remove a in pid filter safely
func (pmo *ProbeManagerOptions) SafeRemoveInPidFilter(id int) {
	pmo.Lock()
	pmo.Filters.InPids = append(pmo.Filters.InPids[:id], pmo.Filters.InPids[id+1:]...)
	pmo.Unlock()
}

// HasProcessAwareFilters - Check if a process aware filter is set
func (pmo *ProbeManagerOptions) HasProcessAwareFilters() bool {
	return len(pmo.Filters.ExceptBinaries) > 0 || len(pmo.Filters.InBinaries) > 0
}

// HasContainerAwareFilters - Check if a container aware filter is set
func (pmo *ProbeManagerOptions) HasContainerAwareFilters() bool {
	return len(pmo.Filters.ExceptContainers) > 0 || len(pmo.Filters.InContainers) > 0 ||
		len(pmo.Filters.ExceptImages) > 0 || len(pmo.Filters.InImages) > 0
}

// HasContextAwareFilters - Some filters are context aware. This means that they require a context
// before being resolved to a kernel filter. This method will tell if at least one of those filters
// are set.
func (pmo *ProbeManagerOptions) HasContextAwareFilters() bool {
	return pmo.HasProcessAwareFilters() || pmo.HasContainerAwareFilters()
}

// FilterByPidAndPidns - Filters an event by Pid and Pidns. The function will answer true if the
// event should be kept or false if it should be dropped.
func (pmo *ProbeManagerOptions) FilterByPidAndPidns(event ProbeEvent) bool {
	// Check pidns
	if len(pmo.Filters.InPidns) > 0 {
		// the event has to be in the list to be kept
		if utils.Uint64ListContains(pmo.Filters.InPidns, event.GetPidns()) < 0 {
			return false
		}
	}
	if len(pmo.Filters.ExceptPidns) > 0 {
		// the event has to not be in the list to be kept
		if utils.Uint64ListContains(pmo.Filters.ExceptPidns, event.GetPidns()) >= 0 {
			return false
		}
	}
	// Check pid
	if len(pmo.Filters.InPids) > 0 {
		// the event has to be in the list to be kept
		if utils.Uint32ListContains(pmo.Filters.InPids, event.GetPid()) < 0 {
			return false
		}
	}
	if len(pmo.Filters.ExceptPids) > 0 {
		// the event has to not be in the list to be kept
		if utils.Uint32ListContains(pmo.Filters.ExceptPids, event.GetPid()) >= 0 {
			return false
		}
	}
	return true
}

// Filter - Filters an event based on its type and the filters relevant to this type.
// The function will answer true if the event should be kept and flase if it should be
// dropped.
func (pmo *ProbeManagerOptions) Filter(event ProbeEvent) bool {
	if !pmo.FilterByPidAndPidns(event) {
		return false
	}
	switch event.GetEventMonitorName() {
	case DockerMonitor:
		return pmo.FilterContainerEvent(event)
	}
	return true
}

// FilterContainerEvent - Filter a container event based on its container name and image name
func (pmo *ProbeManagerOptions) FilterContainerEvent(event ProbeEvent) bool {
	evt := event.(*ContainerEvent)
	// Check container name
	if len(pmo.Filters.InContainers) > 0 {
		// the event has to be in the list to be kept
		if utils.StringListContains(pmo.Filters.InContainers, evt.ContainerName) < 0 {
			return false
		}
	}
	if len(pmo.Filters.ExceptContainers) > 0 {
		// the event has to not be in the list to be kept
		if utils.StringListContains(pmo.Filters.ExceptContainers, evt.ContainerName) >= 0 {
			return false
		}
	}
	// Check image name
	if len(pmo.Filters.InImages) > 0 {
		// the event has to be in the list to be kept
		if utils.StringListContains(pmo.Filters.InImages, evt.Image) < 0 {
			return false
		}
	}
	if len(pmo.Filters.ExceptImages) > 0 {
		// the event has to not be in the list to be kept
		if utils.StringListContains(pmo.Filters.ExceptImages, evt.Image) >= 0 {
			return false
		}
	}
	return true
}

// Filters - Filter options
type Filters struct {
	// Process filters
	InPids         []uint32 `default:""`
	ExceptPids     []uint32 `default:""`
	InBinaries     []string `default:""`
	ExceptBinaries []string `default:""`
	// TTY filter
	TTYOnly bool `default:"false"`
	// Pidns (& container) filters
	InPidns          []uint64 `default:""`
	ExceptPidns      []uint64 `default:""`
	InContainers     []string `default:""`
	ExceptContainers []string `default:""`
	InImages         []string `default:""`
	ExceptImages     []string `default:""`
	// Network filters
	InNet     []NetFilter `default:""`
	ExceptNet []NetFilter `default:""`
	// Syscalls - Syscalls filter
	Syscalls []SyscallFilter `default:""`
}

// NetFilter - Net filter
type NetFilter struct {
	NetworkProtocol   uint64 `default:"0"`
	TransportProtocol int64  `default:"0"`
}

// SyscallFilter - Syscall filter
type SyscallFilter struct {
	SyscallID uint32
	Ret       *ParamFilter
	Arg0      *ParamFilter
	Arg1      *ParamFilter
	Arg2      *ParamFilter
	Arg3      *ParamFilter
	Arg4      *ParamFilter
	Arg5      *ParamFilter
}

// ParamFilter - Parameter filter
type ParamFilter struct {
	Action ParamAction `default:"equal"`
	Value  interface{} `default:"0"`
}
*/
