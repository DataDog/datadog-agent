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
	"net/url"
	"strconv"
	"sync/atomic"
	"time"

	"github.com/gorilla/mux"
	"google.golang.org/grpc"

	"github.com/DataDog/datadog-agent/pkg/networkpath/payload"
	tracerouteutil "github.com/DataDog/datadog-agent/pkg/networkpath/traceroute/config"
	"github.com/DataDog/datadog-agent/pkg/networkpath/traceroute/runner"
	"github.com/DataDog/datadog-agent/pkg/system-probe/api/module"
	sysconfigtypes "github.com/DataDog/datadog-agent/pkg/system-probe/config/types"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

func init() { registerModule(Traceroute) }

type traceroute struct {
	runner *runner.Runner
}

var (
	_ module.Module = &traceroute{}

	tracerouteConfigNamespaces = []string{"traceroute"}
)

func createTracerouteModule(_ *sysconfigtypes.Config, deps module.FactoryDependencies) (module.Module, error) {
	runner, err := runner.New(deps.Telemetry, deps.Hostname)
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
	var runCounter atomic.Uint64

	// TODO: what other config should be passed as part of this request?
	httpMux.HandleFunc("/traceroute/{host}", func(w http.ResponseWriter, req *http.Request) {
		start := time.Now()
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

		runCount := runCounter.Add(1)

		logTracerouteRequests(req.URL, runCount, start)
	})

	return nil
}

func (t *traceroute) RegisterGRPC(_ grpc.ServiceRegistrar) error {
	return nil
}

func (t *traceroute) Close() {}

func logTracerouteRequests(url *url.URL, runCount uint64, start time.Time) {
	msg := fmt.Sprintf("Got request on %s?%s (count: %d): retrieved traceroute in %s", url.RawPath, url.RawQuery, runCount, time.Since(start))
	switch {
	case runCount <= 5, runCount%200 == 0:
		log.Info(msg)
	default:
		log.Debug(msg)
	}
}

func parseParams(req *http.Request) (tracerouteutil.Config, error) {
	vars := mux.Vars(req)
	host := vars["host"]

	query := req.URL.Query()

	port, err := parseUint(query, "port", 16)
	if err != nil {
		return tracerouteutil.Config{}, fmt.Errorf("invalid port: %s", err)
	}
	maxTTL, err := parseUint(query, "max_ttl", 8)
	if err != nil {
		return tracerouteutil.Config{}, fmt.Errorf("invalid max_ttl: %s", err)
	}
	timeout, err := parseUint(query, "timeout", 64)
	if err != nil {
		return tracerouteutil.Config{}, fmt.Errorf("invalid timeout: %s", err)
	}
	protocol := query.Get("protocol")
	tcpMethod := query.Get("tcp_method")

	return tracerouteutil.Config{
		DestHostname: host,
		DestPort:     uint16(port),
		MaxTTL:       uint8(maxTTL),
		Timeout:      time.Duration(timeout),
		Protocol:     payload.Protocol(protocol),
		TCPMethod:    payload.TCPMethod(tcpMethod),
	}, nil
}

func parseUint(query url.Values, field string, bitSize int) (uint64, error) {
	if query.Has(field) {
		return strconv.ParseUint(query.Get(field), 10, bitSize)
	}

	return 0, nil
}
