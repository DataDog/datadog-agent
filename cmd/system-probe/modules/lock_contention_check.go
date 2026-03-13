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

	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/ebpf/probe/lockcontentioncheck"
	"github.com/DataDog/datadog-agent/pkg/ebpf"
	"github.com/DataDog/datadog-agent/pkg/system-probe/api/module"
	"github.com/DataDog/datadog-agent/pkg/system-probe/config"
	sysconfigtypes "github.com/DataDog/datadog-agent/pkg/system-probe/config/types"
	"github.com/DataDog/datadog-agent/pkg/system-probe/utils"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

func init() { registerModule(LockContentionCheck) }

// LockContentionCheck Factory
var LockContentionCheck = &module.Factory{
	Name:             config.LockContentionCheckModule,
	ConfigNamespaces: []string{"lock_contention_check"},
	Fn: func(_ *sysconfigtypes.Config, _ module.FactoryDependencies) (module.Module, error) {
		log.Infof("Starting the lock contention check module")
		p, err := lockcontentioncheck.NewProbe(ebpf.NewConfig())
		if err != nil {
			return nil, fmt.Errorf("unable to start the lock contention check probe: %w", err)
		}
		return &lockContentionCheckModule{
			Probe:     p,
			lastCheck: &atomic.Int64{},
		}, nil
	},
	NeedsEBPF: func() bool {
		return true
	},
}

var _ module.Module = &lockContentionCheckModule{}

type lockContentionCheckModule struct {
	*lockcontentioncheck.Probe
	lastCheck *atomic.Int64
}

// GetStats implements module.Module.GetStats
func (m lockContentionCheckModule) GetStats() map[string]interface{} {
	return map[string]interface{}{
		"last_check": m.lastCheck.Load(),
	}
}

// Register implements module.Module.Register
func (m lockContentionCheckModule) Register(httpMux *module.Router) error {
	httpMux.HandleFunc("/check", utils.WithConcurrencyLimit(1, func(w http.ResponseWriter, req *http.Request) {
		m.lastCheck.Store(time.Now().Unix())
		stats := m.Probe.GetAndFlush()
		utils.WriteAsJSON(req, w, stats, utils.GetPrettyPrintFromQueryParams(req))
	}))

	return nil
}
