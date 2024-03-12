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
	"github.com/DataDog/datadog-agent/comp/core/workloadmeta"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/ebpf/probe/ebpfcheck"
	"github.com/DataDog/datadog-agent/pkg/ebpf"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/datadog-agent/pkg/util/optional"
)

// EBPFProbe Factory
var EBPFProbe = module.Factory{
	Name:             config.EBPFModule,
	ConfigNamespaces: []string{},
	Fn: func(cfg *sysconfigtypes.Config, _ optional.Option[workloadmeta.Component]) (module.Module, error) {
		log.Infof("Starting the ebpf probe")
		okp, err := ebpfcheck.NewProbe(ebpf.NewConfig())
		if err != nil {
			return nil, fmt.Errorf("unable to start the ebpf probe: %w", err)
		}
		return &ebpfModule{
			Probe:     okp,
			lastCheck: atomic.NewInt64(0),
		}, nil
	},
	NeedsEBPF: func() bool {
		return true
	},
}

var _ module.Module = &ebpfModule{}

type ebpfModule struct {
	*ebpfcheck.Probe
	lastCheck *atomic.Int64
}

func (o *ebpfModule) Register(httpMux *module.Router) error {
	// Limit concurrency to one as the probe check is not thread safe (mainly in the entry count buffers)
	httpMux.HandleFunc("/check", utils.WithConcurrencyLimit(1, func(w http.ResponseWriter, req *http.Request) {
		o.lastCheck.Store(time.Now().Unix())
		stats := o.Probe.GetAndFlush()
		utils.WriteAsJSON(w, stats)
	}))

	return nil
}

func (o *ebpfModule) GetStats() map[string]interface{} {
	return map[string]interface{}{
		"last_check": o.lastCheck.Load(),
	}
}
