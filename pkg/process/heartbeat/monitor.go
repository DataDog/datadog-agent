package heartbeat

import (
	"fmt"
	"sort"
	"time"

	"github.com/DataDog/datadog-agent/pkg/process/net"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/datadog-go/statsd"
	"k8s.io/apimachinery/pkg/util/sets"
)

// MetricName is used to determine the metric name for a given module
func MetricName(moduleName string) string {
	return fmt.Sprintf("datadog.system_probe.agent.%s", moduleName)
}

// ModuleMonitor is responsible for emitting heartbeat metrics for each
// system-probe module. It does so by hitting the stats endpoint from
// system-probe and emitting one metric per enabled module using Datadog Public
// API.
type ModuleMonitor struct {
	enabledModulesFn func() ([]string, error)
	metricNameFn     func(string) string
	flusher          flusher
	exit             chan struct{}
}

// Options encapsulates all configuration params used by ModuleMonitor
type Options struct {
	// KeysPerDomain (required) contains API key entries per Datadog domain
	KeysPerDomain map[string][]string

	// HostName (required)
	HostName string

	// SysprobeSocketPath (optional) sets the location of the Unix socket used
	// to reach system-probe
	SysprobeSocketPath string

	// StatsdClient (optional) points to a statsd client to be used as a
	// fallback mechanism
	StatsdClient statsd.ClientInterface

	// TagVersion (optional) contains the agent version to be sent along with the
	// heartbeat metric
	TagVersion string

	// TagRevision (optional) contains the agent revision to be sent along with the
	// heartbeat metric
	TagRevision string

	// MetricNameFn (optional) allows metric names to be specified
	MetricNameFn func(string) string
}

// NewModuleMonitor returns a new ModuleMonitor
func NewModuleMonitor(opts Options) (*ModuleMonitor, error) {
	if opts.SysprobeSocketPath != "" {
		net.SetSystemProbePath(opts.SysprobeSocketPath)
	}

	metricNameFn := MetricName
	if opts.MetricNameFn != nil {
		metricNameFn = opts.MetricNameFn
	}

	flusher, err := newStatsdFlusher(opts)
	if err != nil {
		log.Warnf("could not create statsd flusher: %s", err)
	}

	apiFlusher, err := newAPIFlusher(opts, flusher)
	if err != nil {
		log.Warnf("could not create api flusher: %s", err)
	} else {
		flusher = apiFlusher
	}

	if flusher == nil {
		return nil, fmt.Errorf("can't emit system-probe heartbeat metrics")
	}

	return &ModuleMonitor{
		enabledModulesFn: SystemProbeEnabledModules,
		metricNameFn:     metricNameFn,
		flusher:          flusher,
		exit:             make(chan struct{}),
	}, nil
}

// Heartbeat emits one heartbeat metric for each system-probe module that is
// enabled.  The argument `modules` filters which heartbeats should be
// emitted. If no argument is given all enabled modules are reported.
func (m *ModuleMonitor) Heartbeat(modules ...string) {
	enabled, err := m.enabled(modules)
	if err != nil || len(enabled) == 0 {
		return
	}

	metricNames := make([]string, 0, len(enabled))
	for _, moduleName := range enabled {
		metricNames = append(metricNames, m.metricNameFn(moduleName))
	}

	m.flusher.Flush(metricNames, time.Now())
}

// Every can be used to automatically send heartbeats based on the given time
// interval.
func (m *ModuleMonitor) Every(interval time.Duration, modules ...string) {
	ticker := time.NewTicker(interval)
	go func() {
		for {
			select {
			case <-ticker.C:
				m.Heartbeat(modules...)
			case <-m.exit:
				ticker.Stop()
				return
			}
		}
	}()
}

// Stop flushing the heartbeats
func (m *ModuleMonitor) Stop() {
	if m == nil {
		return
	}

	close(m.exit)
	m.flusher.Stop()
}

func (m *ModuleMonitor) enabled(modules []string) ([]string, error) {
	only := sets.NewString(modules...)
	all, err := m.enabledModulesFn()
	if err != nil {
		return nil, err
	}

	var enabled []string
	for _, moduleName := range all {
		if only.Len() == 0 || only.Has(moduleName) {
			enabled = append(enabled, moduleName)
		}
	}
	sort.Strings(enabled)
	return enabled, nil
}

// SystemProbeEnabledModules returns a list of all system-probe modules that are running
func SystemProbeEnabledModules() ([]string, error) {
	sysprobe, err := net.GetRemoteSystemProbeUtil()
	if err != nil {
		return nil, fmt.Errorf("system-probe not initialized: %s", err)
	}

	stats, err := sysprobe.GetStats()
	if err != nil {
		return nil, err
	}

	modules := make([]string, 0, len(stats))
	for moduleName := range stats {
		modules = append(modules, moduleName)
	}

	return modules, nil
}
