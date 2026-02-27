// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build linux && linux_bpf

package modules

import (
	"fmt"
	"net/http"
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

// NoisyNeighbor Factory
var NoisyNeighbor = &module.Factory{
	Name:             config.NoisyNeighborModule,
	ConfigNamespaces: []string{"noisy_neighbor"},
	Fn: func(_ *sysconfigtypes.Config, _ module.FactoryDependencies) (module.Module, error) {
		log.Infof("Starting the noisy neighbor module")
		p, err := noisyneighbor.NewProbe(ebpf.NewConfig())
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
func (n noisyNeighborModule) GetStats() map[string]interface{} {
	return map[string]interface{}{
		"last_check": n.lastCheck.Load(),
	}
}

// Register implements module.Module.Register
func (n noisyNeighborModule) Register(httpMux *module.Router) error {
	// Limit concurrency to one as the probe check is not thread safe (mainly in the entry count buffers)
	httpMux.HandleFunc("/check", utils.WithConcurrencyLimit(1, func(w http.ResponseWriter, req *http.Request) {
		n.lastCheck.Store(time.Now().Unix())
		stats := n.Probe.GetAndFlush()
		utils.WriteAsJSON(req, w, stats, utils.GetPrettyPrintFromQueryParams(req))
	}))

	return nil
}
