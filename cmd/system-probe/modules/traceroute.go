// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux || windows

package modules

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/gorilla/mux"
	"go.uber.org/atomic"
	"google.golang.org/grpc"

	"github.com/DataDog/datadog-agent/cmd/system-probe/api/module"
	sysconfigtypes "github.com/DataDog/datadog-agent/cmd/system-probe/config/types"
	"github.com/DataDog/datadog-agent/pkg/networkpath/payload"
	tracerouteutil "github.com/DataDog/datadog-agent/pkg/networkpath/traceroute"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

type traceroute struct {
	runner *tracerouteutil.Runner
}

var (
	_ module.Module = &traceroute{}

	tracerouteConfigNamespaces = []string{"traceroute"}
)

func createTracerouteModule(_ *sysconfigtypes.Config, deps module.FactoryDependencies) (module.Module, error) {
	runner, err := tracerouteutil.NewRunner(deps.Telemetry)
	if err != nil {
		return &traceroute{}, err
	}

	return &traceroute{
		runner: runner,
	}, nil
}

func (t *traceroute) GetStats() map[string]interface{} {
	return nil
}

func (t *traceroute) Register(httpMux *module.Router) error {
	var runCounter = atomic.NewUint64(0)

	// TODO: what other config should be passed as part of this request?
	httpMux.HandleFunc("/traceroute/{host}", func(w http.ResponseWriter, req *http.Request) {
		start := time.Now()
		id := getClientID(req)
		cfg, err := parseParams(req)
		if err != nil {
			log.Errorf("invalid params for host: %s: %s", cfg.DestHostname, err)
			w.WriteHeader(http.StatusBadRequest)
			return
		}

		// Run traceroute
		path, err := t.runner.RunTraceroute(context.Background(), cfg)
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
		logTracerouteRequests(cfg, id, runCount, start)
	})

	return nil
}

func (t *traceroute) RegisterGRPC(_ grpc.ServiceRegistrar) error {
	return nil
}

func (t *traceroute) Close() {}

func logTracerouteRequests(cfg tracerouteutil.Config, client string, runCount uint64, start time.Time) {
	args := []interface{}{cfg.DestHostname, client, cfg.DestPort, cfg.MaxTTL, cfg.Timeout, cfg.Protocol, runCount, time.Since(start)}
	msg := "Got request on /traceroute/%s?client_id=%s&port=%d&maxTTL=%d&timeout=%d&protocol=%s (count: %d): retrieved traceroute in %s"
	switch {
	case runCount <= 5, runCount%200 == 0:
		log.Infof(msg, args...)
	default:
		log.Debugf(msg, args...)
	}
}

func parseParams(req *http.Request) (tracerouteutil.Config, error) {
	vars := mux.Vars(req)
	host := vars["host"]
	port, err := parseUint(req, "port", 16)
	if err != nil {
		return tracerouteutil.Config{}, fmt.Errorf("invalid port: %s", err)
	}
	maxTTL, err := parseUint(req, "max_ttl", 8)
	if err != nil {
		return tracerouteutil.Config{}, fmt.Errorf("invalid max_ttl: %s", err)
	}
	timeout, err := parseUint(req, "timeout", 64)
	if err != nil {
		return tracerouteutil.Config{}, fmt.Errorf("invalid timeout: %s", err)
	}
	protocol := req.URL.Query().Get("protocol")

	return tracerouteutil.Config{
		DestHostname: host,
		DestPort:     uint16(port),
		MaxTTL:       uint8(maxTTL),
		Timeout:      time.Duration(timeout),
		Protocol:     payload.Protocol(protocol),
	}, nil
}

func parseUint(req *http.Request, field string, bitSize int) (uint64, error) {
	if req.URL.Query().Has(field) {
		return strconv.ParseUint(req.URL.Query().Get(field), 10, bitSize)
	}

	return 0, nil
}
