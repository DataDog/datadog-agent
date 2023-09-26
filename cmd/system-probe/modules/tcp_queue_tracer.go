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
	"google.golang.org/grpc"

	"github.com/DataDog/datadog-agent/cmd/system-probe/api/module"
	"github.com/DataDog/datadog-agent/cmd/system-probe/config"
	"github.com/DataDog/datadog-agent/cmd/system-probe/utils"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/ebpf/probe/tcpqueuelength"
	"github.com/DataDog/datadog-agent/pkg/ebpf"
)

// TCPQueueLength Factory
var TCPQueueLength = module.Factory{
	Name:             config.TCPQueueLengthTracerModule,
	ConfigNamespaces: []string{},
	Fn: func(cfg *config.Config) (module.Module, error) {
		t, err := tcpqueuelength.NewTracer(ebpf.NewConfig())
		if err != nil {
			return nil, fmt.Errorf("unable to start the TCP queue length tracer: %w", err)
		}

		return &tcpQueueLengthModule{
			Tracer:    t,
			lastCheck: atomic.NewInt64(0),
		}, nil
	},
}

var _ module.Module = &tcpQueueLengthModule{}

type tcpQueueLengthModule struct {
	*tcpqueuelength.Tracer
	lastCheck *atomic.Int64
}

func (t *tcpQueueLengthModule) Register(httpMux *module.Router) error {
	httpMux.HandleFunc("/check", func(w http.ResponseWriter, req *http.Request) {
		t.lastCheck.Store(time.Now().Unix())
		stats := t.Tracer.GetAndFlush()
		utils.WriteAsJSON(w, stats)
	})

	return nil
}

// RegisterGRPC register to system probe gRPC server
func (t *tcpQueueLengthModule) RegisterGRPC(_ grpc.ServiceRegistrar) error {
	return nil
}

func (t *tcpQueueLengthModule) GetStats() map[string]interface{} {
	return map[string]interface{}{
		"last_check": t.lastCheck.Load(),
	}
}
