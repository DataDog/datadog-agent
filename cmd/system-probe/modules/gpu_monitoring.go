// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

package modules

import (
	"fmt"
	"net/http"
	"time"

	"go.uber.org/atomic"

	"github.com/DataDog/datadog-agent/cmd/system-probe/api/module"
	"github.com/DataDog/datadog-agent/cmd/system-probe/config"
	sysconfigtypes "github.com/DataDog/datadog-agent/cmd/system-probe/config/types"
	"github.com/DataDog/datadog-agent/cmd/system-probe/utils"
	"github.com/DataDog/datadog-agent/comp/core/telemetry"
	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	"github.com/DataDog/datadog-agent/pkg/gpu"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/datadog-agent/pkg/util/optional"
)

var _ module.Module = &GPUMonitoringModule{}
var gpuMonitoringConfigNamespaces = []string{"gpu_monitoring"}

// GPUMonitoring Factory
var GPUMonitoring = module.Factory{
	Name:             config.GPUMonitoringModule,
	ConfigNamespaces: gpuMonitoringConfigNamespaces,
	Fn: func(cfg *sysconfigtypes.Config, _ optional.Option[workloadmeta.Component], telemetry telemetry.Component) (module.Module, error) {
		t, err := gpu.NewProbe(gpu.NewConfig(), telemetry)
		if err != nil {
			return nil, fmt.Errorf("unable to start GPU monitoring: %w", err)
		}

		return &GPUMonitoringModule{
			Probe:     t,
			lastCheck: atomic.NewInt64(0),
		}, nil
	},
	NeedsEBPF: func() bool {
		return true
	},
}

type GPUMonitoringModule struct {
	*gpu.Probe
	lastCheck *atomic.Int64
}

func (t *GPUMonitoringModule) Register(httpMux *module.Router) error {
	httpMux.HandleFunc("/check", func(w http.ResponseWriter, req *http.Request) {
		t.lastCheck.Store(time.Now().Unix())
		stats, err := t.Probe.GetAndFlush()
		if err != nil {
			log.Errorf("Error getting GPU stats: %v", err)
			w.WriteHeader(500)
			return
		}

		utils.WriteAsJSON(w, stats)
	})

	return nil
}

func (t *GPUMonitoringModule) GetStats() map[string]interface{} {
	return map[string]interface{}{
		"last_check": t.lastCheck.Load(),
	}
}
