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

// ModuleMonitor is responsible for emitting heartbeat metrics for each
// system-probe module. It does so by hitting the stats endpoint from
// system-probe and emitting one metric per enabled module using Datadog Public
// API. If the API can't be reached for some reason, metrics are sent to the
// statsd deaemon.
type ModuleMonitor struct {
	statsFn statsFn
	flusher flusher
	exit    chan struct{}
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
	// hearbeat metric
	TagVersion string

	// TagRevision (optional) contains the agent revision to be sent along with the
	// hearbeat metric
	TagRevision string
}

// statsFn represents the function signature used to fetch stats from system-probe
type statsFn func() (map[string]interface{}, error)

// NewModuleMonitor returns a new ModuleMonitor
func NewModuleMonitor(opts Options) (*ModuleMonitor, error) {
	if opts.SysprobeSocketPath != "" {
		net.SetSystemProbePath(opts.SysprobeSocketPath)
	}
	sysprobe, err := net.GetRemoteSystemProbeUtil()
	if err != nil {
		return nil, err
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
		statsFn: statsFn(sysprobe.GetStats),
		flusher: flusher,
		exit:    make(chan struct{}),
	}, nil
}

// Heartbeat emits one heartbeat metric for each system-probe module that is
// enabled.  The argument `modules` filters which heartbeats should be
// emitted. If no argument is given all enabled modules are reported.
func (m *ModuleMonitor) Heartbeat(modules ...string) {
	enabled, err := m.enabled(modules)
	if err != nil {
		return
	}
	if len(enabled) == 0 {
		return
	}
	m.flusher.Flush(enabled, time.Now())
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
	stats, err := m.statsFn()
	if err != nil {
		return nil, err
	}

	var enabled []string
	for moduleName := range stats {
		if only.Len() == 0 || only.Has(moduleName) {
			enabled = append(enabled, moduleName)
		}
	}
	sort.Strings(enabled)
	return enabled, nil
}
