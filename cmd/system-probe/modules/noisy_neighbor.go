// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build linux && linux_bpf

package modules

import (
	"fmt"
	"net/http"
	"sync"
	"sync/atomic"
	"time"

	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/ebpf/probe/noisyneighbor"
	pkgconfigsetup "github.com/DataDog/datadog-agent/pkg/config/setup"
	"github.com/DataDog/datadog-agent/pkg/ebpf"
	"github.com/DataDog/datadog-agent/pkg/system-probe/api/module"
	"github.com/DataDog/datadog-agent/pkg/system-probe/config"
	sysconfigtypes "github.com/DataDog/datadog-agent/pkg/system-probe/config/types"
	"github.com/DataDog/datadog-agent/pkg/system-probe/utils"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// pmuMetricConfigKeys maps each BPF perf-event-array map name to its
// noisy_neighbor.pmu_metrics.* config key. Kept in one place so adding a
// new PMU event is a single edit.
var pmuMetricConfigKeys = map[string]string{
	"cycles_pmu":           "noisy_neighbor.pmu_metrics.cycles",
	"instructions_pmu":     "noisy_neighbor.pmu_metrics.instructions",
	"cache_misses_pmu":     "noisy_neighbor.pmu_metrics.cache_misses",
	"cache_references_pmu": "noisy_neighbor.pmu_metrics.cache_references",
	"itlb_misses_pmu":      "noisy_neighbor.pmu_metrics.itlb_misses",
	"branch_misses_pmu":    "noisy_neighbor.pmu_metrics.branch_misses",
	"cpu_migrations_pmu":   "noisy_neighbor.pmu_metrics.cpu_migrations",
}

func readPMUMetricsConfig() map[string]bool {
	cfg := pkgconfigsetup.SystemProbe()
	out := make(map[string]bool, len(pmuMetricConfigKeys))
	for mapName, configKey := range pmuMetricConfigKeys {
		out[mapName] = cfg.GetBool(configKey)
	}
	return out
}

func init() { registerModule(NoisyNeighbor) }

// NoisyNeighbor Factory
var NoisyNeighbor = &module.Factory{
	Name: config.NoisyNeighborModule,
	Fn: func(_ *sysconfigtypes.Config, _ module.FactoryDependencies) (module.Module, error) {
		log.Infof("Starting the noisy neighbor module")
		pmuMetrics := readPMUMetricsConfig()
		p, err := noisyneighbor.NewProbe(ebpf.NewConfig(), noisyneighbor.Config{PMUMetrics: pmuMetrics})
		if err != nil {
			return nil, fmt.Errorf("unable to start the noisy neighbor probe: %w", err)
		}
		return &noisyNeighborModule{
			Probe:      p,
			lastCheck:  &atomic.Int64{},
			pmuMetrics: pmuMetrics,
		}, nil
	},
	NeedsEBPF: func() bool {
		return true
	},
}

var _ module.Module = &noisyNeighborModule{}

type noisyNeighborModule struct {
	*noisyneighbor.Probe
	lastCheck  *atomic.Int64
	pmuMetrics map[string]bool
	inflight   sync.WaitGroup
	closed     atomic.Bool
}

// GetStats implements module.Module.GetStats
func (n *noisyNeighborModule) GetStats() map[string]interface{} {
	return map[string]interface{}{
		"last_check":  n.lastCheck.Load(),
		"pmu_metrics": n.pmuMetrics,
	}
}

// Register implements module.Module.Register
func (n *noisyNeighborModule) Register(httpMux *module.Router) error {
	// Limit concurrency to one as the probe check is not thread safe (mainly in the entry count buffers)
	httpMux.HandleFunc("/check", utils.WithConcurrencyLimit(1, func(w http.ResponseWriter, req *http.Request) {
		if n.closed.Load() {
			http.Error(w, "module closing", http.StatusServiceUnavailable)
			return
		}
		n.inflight.Add(1)
		defer n.inflight.Done()
		// Re-check after Add to close the race where Close() observed inflight==0
		// before our Add but set closed afterwards.
		if n.closed.Load() {
			http.Error(w, "module closing", http.StatusServiceUnavailable)
			return
		}
		n.lastCheck.Store(time.Now().Unix())
		stats := n.Probe.GetAndFlush()
		utils.WriteAsJSON(req, w, stats, utils.GetPrettyPrintFromQueryParams(req))
	}))

	return nil
}

// Close marks the module as closing, waits for any in-flight /check
// handlers to complete, and then tears down the underlying eBPF probe.
// Without this, an in-flight GetAndFlush iterating the BPF map could race
// with Probe.Close() unloading the manager.
func (n *noisyNeighborModule) Close() {
	n.closed.Store(true)
	n.inflight.Wait()
	n.Probe.Close()
}
