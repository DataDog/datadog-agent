// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux && linux_bpf

package modules

import (
	"context"
	"io"
	"net/http"

	"github.com/DataDog/datadog-agent/pkg/network/tracer"
	"github.com/DataDog/datadog-agent/pkg/system-probe/api/module"
	"github.com/DataDog/datadog-agent/pkg/system-probe/config"
	"github.com/DataDog/datadog-agent/pkg/system-probe/utils"
	"github.com/DataDog/datadog-agent/pkg/util/ec2"
	"github.com/DataDog/datadog-agent/pkg/util/kernel"
	netnsutil "github.com/DataDog/datadog-agent/pkg/util/kernel/netns"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

func init() { registerModule(NetworkTracer) }

// NetworkTracer is a factory for NPM's tracer
var NetworkTracer = &module.Factory{
	Name:             config.NetworkTracerModule,
	ConfigNamespaces: networkTracerModuleConfigNamespaces,
	Fn:               createNetworkTracerModule,
	NeedsEBPF:        tracer.NeedsEBPF,
}

func (nt *networkTracer) platformRegister(httpMux *module.Router) error {
	httpMux.HandleFunc("/network_id", utils.WithConcurrencyLimit(utils.DefaultMaxConcurrentRequests, func(w http.ResponseWriter, req *http.Request) {
		id, err := getNetworkID(req.Context())
		if err != nil {
			log.Debugf("unable to retrieve network ID: %s", err)
			w.WriteHeader(500)
			return
		}
		_, _ = io.WriteString(w, id)
	}))

	return nil
}

func getNetworkID(ctx context.Context) (string, error) {
	id := ""
	err := netnsutil.WithRootNS(kernel.ProcFSRoot(), func() error {
		var err error
		id, err = ec2.GetNetworkID(ctx)
		return err
	})
	if err != nil {
		return "", err
	}
	return id, nil
}
