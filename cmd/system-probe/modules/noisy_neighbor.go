// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build linux && linux_bpf

package modules

import (
	"encoding/json"
	"fmt"
	"net/http"
	"sync/atomic"
	"time"

	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/ebpf/probe/noisyneighbor"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/ebpf/probe/noisyneighbor/model"
	pkgconfigsetup "github.com/DataDog/datadog-agent/pkg/config/setup"
	"github.com/DataDog/datadog-agent/pkg/ebpf"
	"github.com/DataDog/datadog-agent/pkg/system-probe/api/module"
	"github.com/DataDog/datadog-agent/pkg/system-probe/config"
	sysconfigtypes "github.com/DataDog/datadog-agent/pkg/system-probe/config/types"
	"github.com/DataDog/datadog-agent/pkg/system-probe/utils"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

func init() { registerModule(NoisyNeighbor) }

// NoisyNeighbor Factory
var NoisyNeighbor = &module.Factory{
	Name: config.NoisyNeighborModule,
	Fn: func(_ *sysconfigtypes.Config, _ module.FactoryDependencies) (module.Module, error) {
		log.Infof("Starting the noisy neighbor module")
		cfg := pkgconfigsetup.SystemProbe()
		p, err := noisyneighbor.NewProbe(ebpf.NewConfig(), noisyneighbor.Config{
			EventMask:         noisyNeighborEventMask(),
			MaxTrackedCgroups: cfg.GetInt("noisy_neighbor.max_tracked_cgroups"),
		})
		if err != nil {
			return nil, fmt.Errorf("unable to start the noisy neighbor probe: %w", err)
		}
		return &noisyNeighborModule{
			Probe:     p,
			lastCheck: &atomic.Int64{},
		}, nil
	},
	NeedsEBPF: func() bool {
		return true
	},
}

var _ module.Module = &noisyNeighborModule{}

type noisyNeighborModule struct {
	*noisyneighbor.Probe
	lastCheck *atomic.Int64
}

// GetStats implements module.Module.GetStats
func (n *noisyNeighborModule) GetStats() map[string]interface{} {
	probeStats := n.Probe.GetStats()
	return map[string]interface{}{
		"last_check":            n.lastCheck.Load(),
		"configured_event_mask": probeStats.ConfiguredEventMask,
		"effective_event_mask":  probeStats.EffectiveEventMask,
		"perf_fd_count":         probeStats.PerfFDCount,
		"online_cpus":           probeStats.OnlineCPUs,
		"watchlist_size":        probeStats.WatchlistSize,
		"eligible_cgroups":      probeStats.EligibleCgroups,
		"attach_errors":         probeStats.AttachErrors,
		"read_errors":           probeStats.ReadErrors,
		"scaling_errors":        probeStats.ScalingErrors,
		"last_rotation":         probeStats.LastRotation,
	}
}

// Register implements module.Module.Register
func (n *noisyNeighborModule) Register(httpMux *module.Router) error {
	// Limit concurrency to one as the probe check is not thread safe (mainly in the entry count buffers)
	httpMux.HandleFunc("/check", utils.WithConcurrencyLimit(1, func(w http.ResponseWriter, req *http.Request) {
		n.lastCheck.Store(time.Now().Unix())
		stats := n.Probe.GetAndFlush()
		utils.WriteAsJSON(req, w, stats, utils.GetPrettyPrintFromQueryParams(req))
	}))
	httpMux.HandleFunc("/watchlist", utils.WithConcurrencyLimit(1, func(w http.ResponseWriter, req *http.Request) {
		if req.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		defer req.Body.Close()
		var watchlist model.WatchlistRequest
		decoder := json.NewDecoder(http.MaxBytesReader(w, req.Body, 16*1024))
		decoder.DisallowUnknownFields()
		if err := decoder.Decode(&watchlist); err != nil {
			http.Error(w, fmt.Sprintf("invalid watchlist: %v", err), http.StatusBadRequest)
			return
		}
		response, err := n.Probe.ReplaceWatchlist(watchlist)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		utils.WriteAsJSON(req, w, response, utils.GetPrettyPrintFromQueryParams(req))
	}))

	return nil
}

func noisyNeighborEventMask() uint64 {
	cfg := pkgconfigsetup.SystemProbe()
	var mask uint64
	settings := []struct {
		key  string
		mask uint64
	}{
		{"cycles", model.EventCycles},
		{"instructions", model.EventInstructions},
		{"cache_misses", model.EventCacheMisses},
		{"cache_references", model.EventCacheReferences},
		{"itlb_misses", model.EventITLBMisses},
		{"branch_misses", model.EventBranchMisses},
		{"cpu_migrations", model.EventCPUMigrations},
	}
	for _, setting := range settings {
		if cfg.GetBool("noisy_neighbor.pmu_metrics." + setting.key) {
			mask |= setting.mask
		}
	}
	return mask
}
