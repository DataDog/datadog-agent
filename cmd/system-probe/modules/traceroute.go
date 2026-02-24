// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

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

	traceroutecomp "github.com/DataDog/datadog-agent/comp/networkpath/traceroute/def"
	"github.com/DataDog/datadog-agent/pkg/networkpath/payload"
	tracerouteutil "github.com/DataDog/datadog-agent/pkg/networkpath/traceroute/config"
	"github.com/DataDog/datadog-agent/pkg/system-probe/api/module"
	sysconfigtypes "github.com/DataDog/datadog-agent/pkg/system-probe/config/types"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

func init() { registerModule(Traceroute) }

type traceroute struct {
	runner traceroutecomp.Component
}

var (
	_ module.Module = &traceroute{}

	tracerouteConfigNamespaces = []string{"traceroute"}
)

func createTracerouteModule(_ *sysconfigtypes.Config, deps module.FactoryDependencies) (module.Module, error) {
	return &traceroute{
		runner: deps.Traceroute,
	}, nil
}

func (t *traceroute) GetStats() map[string]interface{} {
	return nil
}

func (t *traceroute) Register(httpMux *module.Router) error {
	// Start platform-specific driver (Windows only, no-op on other platforms)
	driverError := startPlatformDriver()

	var runCounter atomic.Uint64

	// TODO: what other config should be passed as part of this request?
	httpMux.HandleFunc("/traceroute/{host}", func(w http.ResponseWriter, req *http.Request) {
		start := time.Now()
		cfg, err := parseParams(req)
		if err != nil {
			handleTracerouteReqError(w, http.StatusBadRequest, fmt.Sprintf("invalid params for host: %s: %s", cfg.DestHostname, err))
			return
		}

		if driverError != nil && !cfg.DisableWindowsDriver {
			handleTracerouteReqError(w, http.StatusInternalServerError, fmt.Sprintf("failed to start platform driver: %s", driverError))
			return
		}

		// Run traceroute
		path, err := t.runner.Run(context.Background(), cfg)
		if err != nil {
			handleTracerouteReqError(w, http.StatusInternalServerError, fmt.Sprintf("unable to run traceroute for host: %s: %s", cfg.DestHostname, err.Error()))
			return
		}

		resp, err := json.Marshal(path)
		if err != nil {
			handleTracerouteReqError(w, http.StatusInternalServerError, fmt.Sprintf("unable to marshall traceroute response: %s", err))
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

func (t *traceroute) Close() {
	err := stopPlatformDriver()
	if err != nil {
		log.Errorf("failed to stop platform driver: %s", err)
	}
}

func handleTracerouteReqError(w http.ResponseWriter, statusCode int, errString string) {
	w.WriteHeader(statusCode)
	log.Error(errString)
	_, err := w.Write([]byte(errString))
	if err != nil {
		log.Errorf("unable to write traceroute error response: %s", err)
	}
}

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
	tcpSynParisTracerouteMode := query.Get("tcp_syn_paris_traceroute_mode")
	disableWindowsDriver := query.Get("disable_windows_driver")
	reverseDNS := query.Get("reverse_dns")
	tracerouteQueries, err := parseUint(query, "traceroute_queries", 32)
	if err != nil {
		return tracerouteutil.Config{}, fmt.Errorf("invalid traceroute_queries: %s", err)
	}
	e2eQueries, err := parseUint(query, "e2e_queries", 32)
	if err != nil {
		return tracerouteutil.Config{}, fmt.Errorf("invalid e2e_queries: %s", err)
	}

	return tracerouteutil.Config{
		DestHostname:              host,
		DestPort:                  uint16(port),
		MaxTTL:                    uint8(maxTTL),
		Timeout:                   time.Duration(timeout),
		Protocol:                  payload.Protocol(protocol),
		TCPMethod:                 payload.TCPMethod(tcpMethod),
		TCPSynParisTracerouteMode: tcpSynParisTracerouteMode == "true",
		DisableWindowsDriver:      disableWindowsDriver == "true",
		ReverseDNS:                reverseDNS == "true",
		TracerouteQueries:         int(tracerouteQueries),
		E2eQueries:                int(e2eQueries),
	}, nil
}

func parseUint(query url.Values, field string, bitSize int) (uint64, error) {
	if query.Has(field) {
		return strconv.ParseUint(query.Get(field), 10, bitSize)
	}

	return 0, nil
}
