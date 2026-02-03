// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build linux && linux_bpf

package tcpqueuelengthimpl

import (
	"net/http"
	"sync/atomic"
	"time"

	"github.com/DataDog/datadog-agent/comp/system-probe/types"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/ebpf/probe/tcpqueuelength"
	"github.com/DataDog/datadog-agent/pkg/system-probe/utils"
)

type tcpQueueLengthModule struct {
	*tcpqueuelength.Tracer
	lastCheck atomic.Int64
}

func (t *tcpQueueLengthModule) Register(httpMux types.SystemProbeRouter) error {
	httpMux.HandleFunc("/check", func(w http.ResponseWriter, _ *http.Request) {
		t.lastCheck.Store(time.Now().Unix())
		stats := t.Tracer.GetAndFlush()
		utils.WriteAsJSON(w, stats, utils.CompactOutput)
	})

	return nil
}

func (t *tcpQueueLengthModule) GetStats() map[string]interface{} {
	return map[string]interface{}{
		"last_check": t.lastCheck.Load(),
	}
}
