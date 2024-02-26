// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

package modules

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/DataDog/datadog-agent/cmd/system-probe/api/module"
	"github.com/DataDog/datadog-agent/cmd/system-probe/config"
	sysconfigtypes "github.com/DataDog/datadog-agent/cmd/system-probe/config/types"
	tracerouteutil "github.com/DataDog/datadog-agent/pkg/networkpath/traceroute"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/gorilla/mux"
	"go.uber.org/atomic"
	"google.golang.org/grpc"
)

type traceroute struct{}

// Traceroute is a factory for NDMs Traceroute module
var Traceroute = module.Factory{
	Name:             config.TracerouteModule,
	ConfigNamespaces: []string{"traceroute"},
	Fn: func(cfg *sysconfigtypes.Config) (module.Module, error) {
		return &traceroute{}, nil
	},
	NeedsEBPF: func() bool {
		return false
	},
}

var _ module.Module = &traceroute{}

func (t *traceroute) GetStats() map[string]interface{} {
	return nil
}

func (t *traceroute) Register(httpMux *module.Router) error {
	var runCounter = atomic.NewUint64(0)

	httpMux.HandleFunc("/traceroute/{host}", func(w http.ResponseWriter, req *http.Request) {
		start := time.Now()
		vars := mux.Vars(req)
		id := getClientID(req)
		host := vars["host"]

		cfg := tracerouteutil.Config{
			DestHostname: host,
		}

		// Run traceroute
		path, err := tracerouteutil.RunTraceroute(cfg)
		if err != nil {
			log.Errorf("unable to run traceroute for host: %s: %w", cfg.DestHostname, err)
			w.WriteHeader(http.StatusInternalServerError)
			return
		}

		resp, err := json.Marshal(path)
		if err != nil {
			log.Errorf("unable to marshall traceroute response: %s", err)
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		_, err = w.Write(resp)
		if err != nil {
			log.Errorf("unable to write traceroute response: %s", err)
		}

		runCount := runCounter.Inc()
		logTracerouteRequests(host, id, runCount, start)
	})

	return nil
}

func (t *traceroute) RegisterGRPC(_ grpc.ServiceRegistrar) error {
	return nil
}

func (t *traceroute) Close() {}

func logTracerouteRequests(host string, client string, runCount uint64, start time.Time) {
	args := []interface{}{host, client, runCount, time.Since(start)}
	msg := "Got request on /traceroute/%s?client_id=%s (count: %d): retrieved traceroute in %s"
	switch {
	case runCount <= 5, runCount%20 == 0:
		log.Infof(msg, args...)
	default:
		log.Debugf(msg, args...)
	}
}
