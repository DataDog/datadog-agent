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
	"github.com/DataDog/datadog-agent/pkg/ebpf"
	"github.com/DataDog/datadog-agent/pkg/system-probe/api/module"
	"github.com/DataDog/datadog-agent/pkg/system-probe/config"
	sysconfigtypes "github.com/DataDog/datadog-agent/pkg/system-probe/config/types"
	"github.com/DataDog/datadog-agent/pkg/system-probe/utils"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

func init() { registerModule(NoisyNeighbor) }

// NoisyNeighbor is the system-probe module Factory for the noisy neighbor probe.
var NoisyNeighbor = &module.Factory{
	Name: config.NoisyNeighborModule,
	Fn: func(_ *sysconfigtypes.Config, _ module.FactoryDependencies) (module.Module, error) {
		log.Infof("Starting the noisy neighbor module")
		p, err := noisyneighbor.NewProbe(ebpf.NewConfig())
		if err != nil {
			return nil, fmt.Errorf("noisy_neighbor: probe construction failed: %w", err)
		}
		return &noisyNeighborModule{probe: p}, nil
	},
	NeedsEBPF: func() bool {
		return true
	},
}

var _ module.Module = &noisyNeighborModule{}

type noisyNeighborModule struct {
	probe     *noisyneighbor.Probe
	lastCheck atomic.Int64

	// mu serializes (closed flag write, inflight.Add) against (closed flag
	// read in Close, inflight.Wait). Without it, Close could observe
	// inflight==0 while a request is about to call Add.
	mu       sync.Mutex
	closed   bool
	inflight sync.WaitGroup
}

// enter marks the start of an in-flight /check request. Returns false if the
// module is closing, in which case the caller must not proceed.
func (n *noisyNeighborModule) enter() bool {
	n.mu.Lock()
	defer n.mu.Unlock()
	if n.closed {
		return false
	}
	n.inflight.Add(1)
	return true
}

// GetStats implements module.Module.GetStats.
func (n *noisyNeighborModule) GetStats() map[string]any {
	return map[string]any{
		"last_check": n.lastCheck.Load(),
	}
}

// Register implements module.Module.Register.
func (n *noisyNeighborModule) Register(router *module.Router) error {
	// Limit concurrency to one as the probe check is not thread safe (mainly
	// in the entry count buffers).
	router.HandleFunc("/check", utils.WithConcurrencyLimit(1, n.handleCheck))
	return nil
}

// handleCheck serves the /check endpoint: flushes the BPF map and writes the
// aggregated per-cgroup stats as JSON. enter() must be the first call so
// nothing touches n.probe before the close handshake observes us.
func (n *noisyNeighborModule) handleCheck(w http.ResponseWriter, r *http.Request) {
	if !n.enter() {
		http.Error(w, "module closing", http.StatusServiceUnavailable)
		return
	}
	defer n.inflight.Done()

	stats, err := n.probe.GetAndFlush(r.Context())
	if err != nil {
		log.Warnf("noisy_neighbor: GetAndFlush returned error, skipping emission: %v", err)
		http.Error(w, fmt.Sprintf("noisy_neighbor flush error: %v", err), http.StatusInternalServerError)
		return
	}
	n.lastCheck.Store(time.Now().Unix())
	utils.WriteAsJSON(r, w, stats, utils.GetPrettyPrintFromQueryParams(r))
}

// Close marks the module as closing, waits for any in-flight /check handlers
// to complete, and then tears down the underlying eBPF probe. The HTTP
// server that hosts /check is expected to stop accepting new requests
// concurrently with or before Close is invoked. Probe shutdown is idempotent
// via sync.OnceFunc inside Probe.
func (n *noisyNeighborModule) Close() {
	n.mu.Lock()
	n.closed = true
	n.mu.Unlock()
	n.inflight.Wait()
	n.probe.Close()
}
