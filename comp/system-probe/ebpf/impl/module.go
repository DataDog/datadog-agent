// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build linux && linux_bpf

package ebpfimpl

import (
	"net/http"
	"sync/atomic"
	"time"

	"github.com/DataDog/datadog-agent/comp/system-probe/types"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/ebpf/probe/ebpfcheck"
	"github.com/DataDog/datadog-agent/pkg/system-probe/utils"
)

type ebpfModule struct {
	*ebpfcheck.Probe
	lastCheck atomic.Int64
}

func (o *ebpfModule) Register(httpMux types.SystemProbeRouter) error {
	// Limit concurrency to one as the probe check is not thread safe (mainly in the entry count buffers)
	httpMux.HandleFunc("/check", utils.WithConcurrencyLimit(1, func(w http.ResponseWriter, _ *http.Request) {
		o.lastCheck.Store(time.Now().Unix())
		stats := o.Probe.GetAndFlush()
		utils.WriteAsJSON(w, stats, utils.CompactOutput)
	}))

	return nil
}

func (o *ebpfModule) GetStats() map[string]interface{} {
	return map[string]interface{}{
		"last_check": o.lastCheck.Load(),
	}
}
