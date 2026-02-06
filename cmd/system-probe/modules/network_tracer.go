// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build (linux && linux_bpf) || (windows && npm) || darwin

package modules

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"sync/atomic"
	"time"

	"github.com/DataDog/datadog-agent/pkg/network"
	networkconfig "github.com/DataDog/datadog-agent/pkg/network/config"
	"github.com/DataDog/datadog-agent/pkg/network/encoding/marshal"
	"github.com/DataDog/datadog-agent/pkg/network/sender"
	"github.com/DataDog/datadog-agent/pkg/network/tracer"
	"github.com/DataDog/datadog-agent/pkg/system-probe/api/module"
	sysconfigtypes "github.com/DataDog/datadog-agent/pkg/system-probe/config/types"
	"github.com/DataDog/datadog-agent/pkg/system-probe/utils"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// ErrSysprobeUnsupported is the unsupported error prefix, for error-class matching from callers
var ErrSysprobeUnsupported = errors.New("system-probe unsupported")

const inactivityLogDuration = 10 * time.Minute
const inactivityRestartDuration = 20 * time.Minute

var networkTracerModuleConfigNamespaces = []string{"network_config", "service_monitoring_config"}

const maxConntrackDumpSize = 3000

func createNetworkTracerModule(_ *sysconfigtypes.Config, deps module.FactoryDependencies) (module.Module, error) {
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

	t, err := tracer.NewTracer(ncfg, deps.Telemetry, deps.Statsd)
	if err != nil {
		return nil, err
	}

	ctx, cancel := context.WithCancel(context.Background())
	var connsSender sender.Sender
	if ncfg.DirectSend {
		connsSender, err = sender.New(ctx, t, sender.Dependencies{
			Config:         deps.CoreConfig,
			Logger:         deps.Log,
			Sysprobeconfig: deps.SysprobeConfig,
			Tagger:         deps.Tagger,
			Wmeta:          deps.WMeta,
			Hostname:       deps.Hostname,
			Forwarder:      deps.ConnectionsForwarder,
			NPCollector:    deps.NPCollector,
		})
		if err != nil {
			t.Stop()
			cancel()
			return nil, fmt.Errorf("create direct sender: %s", err)
		}
	}

	return &networkTracer{
		tracer:      t,
		cfg:         ncfg,
		connsSender: connsSender,
		ctx:         ctx,
		cancelFunc:  cancel,
	}, nil
}

var _ module.Module = &networkTracer{}

type networkTracer struct {
	tracer       *tracer.Tracer
	cfg          *networkconfig.Config
	restartTimer *time.Timer
	connsSender  sender.Sender
	ctx          context.Context
	cancelFunc   context.CancelFunc
}

func (nt *networkTracer) GetStats() map[string]interface{} {
	stats, _ := nt.tracer.GetStats()
	return stats
}

// Register all networkTracer endpoints
func (nt *networkTracer) Register(httpMux *module.Router) error {
	if !nt.cfg.DirectSend {
		var runCounter atomic.Uint64

		httpMux.HandleFunc("/connections", utils.WithConcurrencyLimit(utils.DefaultMaxConcurrentRequests, func(w http.ResponseWriter, req *http.Request) {
			start := time.Now()
			id := utils.GetClientID(req)
			cs, cleanup, err := nt.tracer.GetActiveConnections(id)
			if err != nil {
				log.Errorf("unable to retrieve connections: %s", err)
				w.WriteHeader(500)
				return
			}
			defer cleanup()
			contentType := req.Header.Get("Accept")
			marshaler := marshal.GetMarshaler(contentType)
			writeConnections(w, marshaler, cs)

			if nt.restartTimer != nil {
				nt.restartTimer.Reset(inactivityRestartDuration)
			}
			count := runCounter.Add(1)
			logRequests(id, count, len(cs.Conns), start)
		}))

		httpMux.HandleFunc("/register", utils.WithConcurrencyLimit(utils.DefaultMaxConcurrentRequests, func(w http.ResponseWriter, req *http.Request) {
			id := utils.GetClientID(req)
			err := nt.tracer.RegisterClient(id)
			log.Debugf("Got request on /network_tracer/register?client_id=%s", id)
			if err != nil {
				log.Errorf("unable to register client: %s", err)
				w.WriteHeader(http.StatusInternalServerError)
			} else {
				w.WriteHeader(http.StatusOK)
			}
		}))

		// Convenience logging if nothing has made any requests to the system-probe in some time, let's log something.
		// This should be helpful for customers + support to debug the underlying issue.
		time.AfterFunc(inactivityLogDuration, func() {
			if runCounter.Load() == 0 {
				log.Warnf("%v since the agent started without activity, the process-agent may not be configured correctly and/or running", inactivityLogDuration)
			}
		})
	}

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
		stats, err := nt.tracer.DebugNetworkState(utils.GetClientID(req))
		if err != nil {
			log.Errorf("unable to retrieve tracer stats: %s", err)
			w.WriteHeader(500)
			return
		}

		utils.WriteAsJSON(w, stats, utils.CompactOutput)
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

		utils.WriteAsJSON(w, cache, utils.CompactOutput)
	})

	registerUSMEndpoints(nt, httpMux)

	return nt.platformRegister(httpMux)
}

// Close will stop all system probe activities
func (nt *networkTracer) Close() {
	if nt.connsSender != nil {
		nt.connsSender.Stop()
	}
	nt.tracer.Stop()
	nt.cancelFunc()
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

func writeConnections(w http.ResponseWriter, marshaler marshal.Marshaler, cs *network.Connections) {
	defer network.Reclaim(cs)

	w.Header().Set("Content-type", marshaler.ContentType())

	connectionsModeler, err := marshal.NewConnectionsModeler(cs)
	if err != nil {
		log.Errorf("unable to create new connections modeler: %s", err)
		w.WriteHeader(500)
		return
	}
	defer connectionsModeler.Close()

	err = marshaler.Marshal(cs, w, connectionsModeler)
	if err != nil {
		log.Errorf("unable to marshall connections with type %s: %s", marshaler.ContentType(), err)
		w.WriteHeader(500)
		return
	}

	log.Tracef("/connections: %d connections", len(cs.Conns))
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
