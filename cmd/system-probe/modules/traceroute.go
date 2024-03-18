// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

////go:build linux || windows

package modules

import (
	"encoding/json"
	"net/http"
	"strconv"
	"time"

	"github.com/DataDog/datadog-agent/cmd/system-probe/api/module"
	sysconfigtypes "github.com/DataDog/datadog-agent/cmd/system-probe/config/types"
	"github.com/DataDog/datadog-agent/comp/core/workloadmeta"
	tracerouteutil "github.com/DataDog/datadog-agent/pkg/networkpath/traceroute"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/datadog-agent/pkg/util/optional"
	"github.com/gorilla/mux"
	"go.uber.org/atomic"
	"google.golang.org/grpc"
)

type traceroute struct{}

var (
	_ module.Module = &traceroute{}

	tracerouteConfigNamespaces = []string{"traceroute"}
)

func createTracerouteModule(_ *sysconfigtypes.Config, _ optional.Option[workloadmeta.Component]) (module.Module, error) {
	return &traceroute{}, nil
}

func (t *traceroute) GetStats() map[string]interface{} {
	return nil
}

func (t *traceroute) Register(httpMux *module.Router) error {
	var runCounter = atomic.NewUint64(0)

	// TODO: what other config should be passed as part of this request?
	httpMux.HandleFunc("/traceroute/{host}", func(w http.ResponseWriter, req *http.Request) {
		start := time.Now()
		vars := mux.Vars(req)
		id := getClientID(req)
		host := vars["host"]
		port, err := strconv.ParseUint(vars["port"], 10, 16)
		if err != nil {
			log.Errorf("invalid port: %s", err)
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		maxTTL, err := strconv.ParseUint(vars["max_ttl"], 10, 8)
		if err != nil {
			log.Errorf("invalid max_ttl: %s", err)
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		timeout, err := strconv.ParseUint(vars["timeout"], 10, 32)
		if err != nil {
			log.Errorf("invalid max_ttl: %s", err)
			w.WriteHeader(http.StatusBadRequest)
			return
		}

		cfg := tracerouteutil.Config{
			DestHostname: host,
			DestPort:     uint16(port),
			MaxTTL:       uint8(maxTTL),
			TimeoutMs:    uint(timeout),
		}

		// Run traceroute
		path, err := tracerouteutil.RunTraceroute(cfg)
		if err != nil {
			log.Errorf("unable to run traceroute for host: %s: %s", cfg.DestHostname, err.Error())
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
