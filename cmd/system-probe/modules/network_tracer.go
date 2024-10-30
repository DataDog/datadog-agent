// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux || windows

package modules

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"runtime"
	"strings"
	"time"

	"go.uber.org/atomic"

	"github.com/DataDog/datadog-agent/cmd/system-probe/api/module"
	sysconfigtypes "github.com/DataDog/datadog-agent/cmd/system-probe/config/types"
	"github.com/DataDog/datadog-agent/cmd/system-probe/utils"
	coreconfig "github.com/DataDog/datadog-agent/pkg/config/setup"
	"github.com/DataDog/datadog-agent/pkg/network"
	networkconfig "github.com/DataDog/datadog-agent/pkg/network/config"
	"github.com/DataDog/datadog-agent/pkg/network/encoding/marshal"
	httpdebugging "github.com/DataDog/datadog-agent/pkg/network/protocols/http/debugging"
	kafkadebugging "github.com/DataDog/datadog-agent/pkg/network/protocols/kafka/debugging"
	postgresdebugging "github.com/DataDog/datadog-agent/pkg/network/protocols/postgres/debugging"
	redisdebugging "github.com/DataDog/datadog-agent/pkg/network/protocols/redis/debugging"
	"github.com/DataDog/datadog-agent/pkg/network/protocols/telemetry"
	"github.com/DataDog/datadog-agent/pkg/network/tracer"
	usm "github.com/DataDog/datadog-agent/pkg/network/usm/utils"
	"github.com/DataDog/datadog-agent/pkg/process/statsd"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// ErrSysprobeUnsupported is the unsupported error prefix, for error-class matching from callers
var ErrSysprobeUnsupported = errors.New("system-probe unsupported")

const inactivityLogDuration = 10 * time.Minute
const inactivityRestartDuration = 20 * time.Minute

var networkTracerModuleConfigNamespaces = []string{"network_config", "service_monitoring_config"}

const maxConntrackDumpSize = 3000

func createNetworkTracerModule(cfg *sysconfigtypes.Config, deps module.FactoryDependencies) (module.Module, error) {
	ncfg := networkconfig.New()

	// Checking whether the current OS + kernel version is supported by the tracer
	if supported, err := tracer.IsTracerSupportedByOS(ncfg.ExcludedBPFLinuxVersions); !supported {
		return nil, fmt.Errorf("%w: %s", ErrSysprobeUnsupported, err)
	}

	if ncfg.NPMEnabled {
		log.Info("enabling network performance monitoring (NPM)")
	}
	if ncfg.ServiceMonitoringEnabled {
		log.Info("enabling universal service monitoring (USM)")
	}

	t, err := tracer.NewTracer(ncfg, deps.Telemetry)

	done := make(chan struct{})
	if err == nil {
		startTelemetryReporter(cfg, done)
	}

	return &networkTracer{tracer: t, done: done}, err
}

var _ module.Module = &networkTracer{}

type networkTracer struct {
	tracer       *tracer.Tracer
	done         chan struct{}
	restartTimer *time.Timer
}

func (nt *networkTracer) GetStats() map[string]interface{} {
	stats, _ := nt.tracer.GetStats()
	return stats
}

// Register all networkTracer endpoints
func (nt *networkTracer) Register(httpMux *module.Router) error {
	var runCounter = atomic.NewUint64(0)

	httpMux.HandleFunc("/connections", utils.WithConcurrencyLimit(utils.DefaultMaxConcurrentRequests, func(w http.ResponseWriter, req *http.Request) {
		start := time.Now()
		id := getClientID(req)
		cs, err := nt.tracer.GetActiveConnections(id)
		if err != nil {
			log.Errorf("unable to retrieve connections: %s", err)
			w.WriteHeader(500)
			return
		}
		contentType := req.Header.Get("Accept")
		marshaler := marshal.GetMarshaler(contentType)
		writeConnections(w, marshaler, cs)

		if nt.restartTimer != nil {
			nt.restartTimer.Reset(inactivityRestartDuration)
		}
		count := runCounter.Inc()
		logRequests(id, count, len(cs.Conns), start)
	}))

	httpMux.HandleFunc("/network_id", utils.WithConcurrencyLimit(utils.DefaultMaxConcurrentRequests, func(w http.ResponseWriter, req *http.Request) {
		id, err := nt.tracer.GetNetworkID(req.Context())
		if err != nil {
			log.Errorf("unable to retrieve network_id: %s", err)
			w.WriteHeader(500)
			return
		}
		io.WriteString(w, id)
	}))

	httpMux.HandleFunc("/register", utils.WithConcurrencyLimit(utils.DefaultMaxConcurrentRequests, func(w http.ResponseWriter, req *http.Request) {
		id := getClientID(req)
		err := nt.tracer.RegisterClient(id)
		log.Debugf("Got request on /network_tracer/register?client_id=%s", id)
		if err != nil {
			log.Errorf("unable to register client: %s", err)
			w.WriteHeader(http.StatusInternalServerError)
		} else {
			w.WriteHeader(http.StatusOK)
		}
	}))

	httpMux.HandleFunc("/debug/net_maps", func(w http.ResponseWriter, req *http.Request) {
		cs, err := nt.tracer.DebugNetworkMaps()
		if err != nil {
			log.Errorf("unable to retrieve connections: %s", err)
			w.WriteHeader(500)
			return
		}

		contentType := req.Header.Get("Accept")
		marshaler := marshal.GetMarshaler(contentType)
		writeConnections(w, marshaler, cs)
	})

	httpMux.HandleFunc("/debug/net_state", func(w http.ResponseWriter, req *http.Request) {
		stats, err := nt.tracer.DebugNetworkState(getClientID(req))
		if err != nil {
			log.Errorf("unable to retrieve tracer stats: %s", err)
			w.WriteHeader(500)
			return
		}

		utils.WriteAsJSON(w, stats)
	})

	httpMux.HandleFunc("/debug/http_monitoring", func(w http.ResponseWriter, req *http.Request) {
		if !coreconfig.SystemProbe().GetBool("service_monitoring_config.enable_http_monitoring") {
			writeDisabledProtocolMessage("http", w)
			return
		}
		id := getClientID(req)
		cs, err := nt.tracer.GetActiveConnections(id)
		if err != nil {
			log.Errorf("unable to retrieve connections: %s", err)
			w.WriteHeader(500)
			return
		}

		utils.WriteAsJSON(w, httpdebugging.HTTP(cs.HTTP, cs.DNS))
	})

	httpMux.HandleFunc("/debug/kafka_monitoring", func(w http.ResponseWriter, req *http.Request) {
		if !coreconfig.SystemProbe().GetBool("service_monitoring_config.enable_kafka_monitoring") {
			writeDisabledProtocolMessage("kafka", w)
			return
		}
		id := getClientID(req)
		cs, err := nt.tracer.GetActiveConnections(id)
		if err != nil {
			log.Errorf("unable to retrieve connections: %s", err)
			w.WriteHeader(500)
			return
		}

		utils.WriteAsJSON(w, kafkadebugging.Kafka(cs.Kafka))
	})

	httpMux.HandleFunc("/debug/postgres_monitoring", func(w http.ResponseWriter, req *http.Request) {
		if !coreconfig.SystemProbe().GetBool("service_monitoring_config.enable_postgres_monitoring") {
			writeDisabledProtocolMessage("postgres", w)
			return
		}
		id := getClientID(req)
		cs, err := nt.tracer.GetActiveConnections(id)
		if err != nil {
			log.Errorf("unable to retrieve connections: %s", err)
			w.WriteHeader(500)
			return
		}

		utils.WriteAsJSON(w, postgresdebugging.Postgres(cs.Postgres))
	})

	httpMux.HandleFunc("/debug/redis_monitoring", func(w http.ResponseWriter, req *http.Request) {
		if !coreconfig.SystemProbe().GetBool("service_monitoring_config.enable_redis_monitoring") {
			writeDisabledProtocolMessage("redis", w)
			return
		}
		id := getClientID(req)
		cs, err := nt.tracer.GetActiveConnections(id)
		if err != nil {
			log.Errorf("unable to retrieve connections: %s", err)
			w.WriteHeader(500)
			return
		}

		utils.WriteAsJSON(w, redisdebugging.Redis(cs.Redis))
	})

	httpMux.HandleFunc("/debug/http2_monitoring", func(w http.ResponseWriter, req *http.Request) {
		if !coreconfig.SystemProbe().GetBool("service_monitoring_config.enable_http2_monitoring") {
			writeDisabledProtocolMessage("http2", w)
			return
		}
		id := getClientID(req)
		cs, err := nt.tracer.GetActiveConnections(id)
		if err != nil {
			log.Errorf("unable to retrieve connections: %s", err)
			w.WriteHeader(500)
			return
		}

		utils.WriteAsJSON(w, httpdebugging.HTTP(cs.HTTP2, cs.DNS))
	})

	// /debug/ebpf_maps as default will dump all registered maps/perfmaps
	// an optional ?maps= argument could be pass with a list of map name : ?maps=map1,map2,map3
	httpMux.HandleFunc("/debug/ebpf_maps", func(w http.ResponseWriter, req *http.Request) {
		maps := []string{}
		if listMaps := req.URL.Query().Get("maps"); listMaps != "" {
			maps = strings.Split(listMaps, ",")
		}

		err := nt.tracer.DebugEBPFMaps(w, maps...)
		if err != nil {
			log.Errorf("unable to retrieve eBPF maps: %s", err)
			w.WriteHeader(500)
			return
		}
	})

	httpMux.HandleFunc("/debug/conntrack/cached", func(w http.ResponseWriter, req *http.Request) {
		ctx, cancelFunc := context.WithTimeout(req.Context(), 30*time.Second)
		defer cancelFunc()
		table, err := nt.tracer.DebugCachedConntrack(ctx)
		if err != nil {
			log.Errorf("unable to retrieve cached conntrack table: %s", err)
			w.WriteHeader(500)
			return
		}

		writeConntrackTable(table, w)
	})

	httpMux.HandleFunc("/debug/conntrack/host", func(w http.ResponseWriter, req *http.Request) {
		ctx, cancelFunc := context.WithTimeout(req.Context(), 10*time.Second)
		defer cancelFunc()
		table, err := nt.tracer.DebugHostConntrack(ctx)
		if err != nil {
			log.Errorf("unable to retrieve host conntrack table: %s", err)
			w.WriteHeader(500)
			return
		}

		writeConntrackTable(table, w)
	})

	httpMux.HandleFunc("/debug/process_cache", func(w http.ResponseWriter, r *http.Request) {
		ctx, cancelFunc := context.WithTimeout(r.Context(), 10*time.Second)
		defer cancelFunc()
		cache, err := nt.tracer.DebugDumpProcessCache(ctx)
		if err != nil {
			log.Errorf("unable to dump tracer process cache: %s", err)
			w.WriteHeader(500)
			return
		}

		utils.WriteAsJSON(w, cache)
	})

	httpMux.HandleFunc("/debug/usm_telemetry", telemetry.Handler)
	httpMux.HandleFunc("/debug/usm/traced_programs", usm.TracedProgramsEndpoint)
	httpMux.HandleFunc("/debug/usm/blocked_processes", usm.BlockedPathIDEndpoint)
	httpMux.HandleFunc("/debug/usm/clear_blocked", usm.ClearBlockedEndpoint)
	httpMux.HandleFunc("/debug/usm/attach-pid", usm.AttachPIDEndpoint)
	httpMux.HandleFunc("/debug/usm/detach-pid", usm.DetachPIDEndpoint)

	// Convenience logging if nothing has made any requests to the system-probe in some time, let's log something.
	// This should be helpful for customers + support to debug the underlying issue.
	time.AfterFunc(inactivityLogDuration, func() {
		if runCounter.Load() == 0 {
			log.Warnf("%v since the agent started without activity, the process-agent may not be configured correctly and/or running", inactivityLogDuration)
		}
	})

	if runtime.GOOS == "windows" {
		nt.restartTimer = time.AfterFunc(inactivityRestartDuration, func() {
			log.Criticalf("%v since the process-agent last queried for data. It may not be configured correctly and/or running. Exiting system-probe to save system resources.", inactivityRestartDuration)
			inactivityEventLog(inactivityRestartDuration)
			nt.Close()
			os.Exit(1)
		})
	}

	return nil
}

// Close will stop all system probe activities
func (nt *networkTracer) Close() {
	close(nt.done)
	nt.tracer.Stop()
}

func logRequests(client string, count uint64, connectionsCount int, start time.Time) {
	args := []interface{}{client, count, connectionsCount, time.Since(start)}
	msg := "Got request on /connections?client_id=%s (count: %d): retrieved %d connections in %s"
	switch {
	case count <= 5, count%20 == 0:
		log.Infof(msg, args...)
	default:
		log.Debugf(msg, args...)
	}
}

func getClientID(req *http.Request) string {
	var clientID = network.DEBUGCLIENT
	if rawCID := req.URL.Query().Get("client_id"); rawCID != "" {
		clientID = rawCID
	}
	return clientID
}

func writeConnections(w http.ResponseWriter, marshaler marshal.Marshaler, cs *network.Connections) {
	defer network.Reclaim(cs)

	w.Header().Set("Content-type", marshaler.ContentType())

	connectionsModeler := marshal.NewConnectionsModeler(cs)
	defer connectionsModeler.Close()

	err := marshaler.Marshal(cs, w, connectionsModeler)
	if err != nil {
		log.Errorf("unable to marshall connections with type %s: %s", marshaler.ContentType(), err)
		w.WriteHeader(500)
		return
	}

	log.Tracef("/connections: %d connections", len(cs.Conns))
}

func startTelemetryReporter(_ *sysconfigtypes.Config, done <-chan struct{}) {
	telemetry.SetStatsdClient(statsd.Client)
	ticker := time.NewTicker(30 * time.Second)
	go func() {
		defer ticker.Stop()

		for {
			select {
			case <-ticker.C:
				telemetry.ReportStatsd()
			case <-done:
				return
			}
		}
	}()
}

func writeDisabledProtocolMessage(protocolName string, w http.ResponseWriter) {
	log.Warnf("%s monitoring is disabled", protocolName)
	w.WriteHeader(404)
	// Writing JSON to ensure compatibility when using the jq bash utility for output
	outputString := map[string]string{"error": fmt.Sprintf("%s monitoring is disabled", protocolName)}
	// We are marshaling a static string, so we can ignore the error
	buf, _ := json.Marshal(outputString)
	w.Write(buf)
}

func writeConntrackTable(table *tracer.DebugConntrackTable, w http.ResponseWriter) {
	err := table.WriteTo(w, maxConntrackDumpSize)
	if err != nil {
		log.Errorf("unable to dump conntrack: %s", err)
		w.WriteHeader(500)
	}
}
