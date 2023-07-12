// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux || windows

package modules

import (
	"context"
	"errors"
	"fmt"
	"google.golang.org/grpc"
	"net"
	"net/http"
	"os"
	"os/signal"
	"runtime"
	"strings"
	"syscall"
	"time"

	"go.uber.org/atomic"

	"github.com/DataDog/datadog-agent/cmd/system-probe/api/module"
	"github.com/DataDog/datadog-agent/cmd/system-probe/config"
	"github.com/DataDog/datadog-agent/cmd/system-probe/utils"
	"github.com/DataDog/datadog-agent/pkg/network"
	networkconfig "github.com/DataDog/datadog-agent/pkg/network/config"
	"github.com/DataDog/datadog-agent/pkg/network/encoding"
	httpdebugging "github.com/DataDog/datadog-agent/pkg/network/protocols/http/debugging"
	kafkadebugging "github.com/DataDog/datadog-agent/pkg/network/protocols/kafka/debugging"
	"github.com/DataDog/datadog-agent/pkg/network/protocols/telemetry"
	"github.com/DataDog/datadog-agent/pkg/network/tracer"
	"github.com/DataDog/datadog-agent/pkg/process/statsd"
	"github.com/DataDog/datadog-agent/pkg/proto/connectionserver"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// ErrSysprobeUnsupported is the unsupported error prefix, for error-class matching from callers
var ErrSysprobeUnsupported = errors.New("system-probe unsupported")

const inactivityLogDuration = 10 * time.Minute
const inactivityRestartDuration = 20 * time.Minute

// NetworkTracer is a factory for NPM's tracer
var NetworkTracer = module.Factory{
	Name:             config.NetworkTracerModule,
	ConfigNamespaces: []string{"network_config", "service_monitoring_config", "data_streams_config"},
	Fn: func(cfg *config.Config) (module.Module, error) {
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
		if ncfg.DataStreamsEnabled {
			log.Info("enabling data streams monitoring (DSM)")
		}

		t, err := tracer.NewTracer(ncfg)

		done := make(chan struct{})
		if err == nil {
			startTelemetryReporter(cfg, done)
		}

		nt := networkTracer{
			tracer: t,
		}
		nt.done = done

		if ncfg.UseGRPC {
			if ncfg.GRPCSocketFilePath == "" {
				log.Errorf("unable to create a grpc server without a path for the socket file")
			} else {
				go func() {
					err := startRPCServer(nt, ncfg.GRPCSocketFilePath)
					if err != nil {
						log.Errorf("Failed to start gRPC server: %v", err)
					}
				}()
			}
		}

		return &nt, err
	},
}

var _ module.Module = &networkTracer{}

type networkTracer struct {
	tracer       *tracer.Tracer
	done         chan struct{}
	restartTimer *time.Timer
}

// startRPCServer is used to start a gRPC server using Unix sockets.
func startRPCServer(tracer networkTracer, socketFilePath string) error {
	// Remove existing socket file if it exists
	os.Remove(socketFilePath)

	listener, err := net.Listen("unix", socketFilePath)
	if err != nil {
		return err
	}

	server := grpc.NewServer()
	connectionserver.RegisterSystemProbeServer(server, &tracer)

	go func() {
		log.Info("gRPC server listening on Unix domain socket...")

		err = server.Serve(listener)
		if err != nil {
			log.Errorf("unable to serve gRPC server %v", err)
		}
	}()

	// Wait for interrupt signal to gracefully stop the server
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)
	<-sigCh

	// Gracefully stop the server
	server.GracefulStop()
	return nil
}

func (nt *networkTracer) GetStats() map[string]interface{} {
	stats, _ := nt.tracer.GetStats()
	return stats
}

func getConnectionsFromMarshal(marshaler encoding.Marshaler, cs *network.Connections) ([]byte, error) {
	defer network.Reclaim(cs)

	buf, err := marshaler.Marshal(cs)
	if err != nil {
		log.Errorf("unable to marshall connections with type %s: %s", marshaler.ContentType(), err)
		return nil, err
	}

	log.Tracef("/GetConnections: %d connections, %d bytes", len(cs.Conns), len(buf))
	return buf, nil
}

func (nt *networkTracer) GetConnections(req *connectionserver.GetConnectionsRequest, s2 connectionserver.SystemProbe_GetConnectionsServer) error {
	start := time.Now()
	runCounter := atomic.NewUint64(0)
	id := req.GetClientID()
	cs, err := nt.tracer.GetActiveConnections(id)
	if err != nil {
		return err
	}

	marshaler := encoding.GetMarshaler(encoding.ContentTypeProtobuf)
	conns, err := getConnectionsFromMarshal(marshaler, cs)
	if err != nil {
		return err
	}

	if nt.restartTimer != nil {
		nt.restartTimer.Reset(inactivityRestartDuration)
	}
	count := runCounter.Inc()
	logRequests(id, count, len(cs.Conns), start)

	// iterate over all the connections
	s2.Send(&connectionserver.Connection{Data: conns})

	return nil
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
		marshaler := encoding.GetMarshaler(contentType)
		writeConnections(w, marshaler, cs)

		if nt.restartTimer != nil {
			nt.restartTimer.Reset(inactivityRestartDuration)
		}

		count := runCounter.Inc()
		logRequests(id, count, len(cs.Conns), start)
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
		marshaler := encoding.GetMarshaler(contentType)
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
		id := getClientID(req)
		cs, err := nt.tracer.GetActiveConnections(id)
		if err != nil {
			log.Errorf("unable to retrieve connections: %s", err)
			w.WriteHeader(500)
			return
		}

		utils.WriteAsJSON(w, kafkadebugging.Kafka(cs.Kafka))
	})

	httpMux.HandleFunc("/debug/http2_monitoring", func(w http.ResponseWriter, req *http.Request) {
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

		ebpfMaps, err := nt.tracer.DebugEBPFMaps(maps...)
		if err != nil {
			log.Errorf("unable to retrieve eBPF maps: %s", err)
			w.WriteHeader(500)
			return
		}

		utils.WriteAsJSON(w, ebpfMaps)
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

		utils.WriteAsJSON(w, table)
	})

	httpMux.HandleFunc("/debug/conntrack/host", func(w http.ResponseWriter, req *http.Request) {
		ctx, cancelFunc := context.WithTimeout(req.Context(), 30*time.Second)
		defer cancelFunc()
		table, err := nt.tracer.DebugHostConntrack(ctx)
		if err != nil {
			log.Errorf("unable to retrieve host conntrack table: %s", err)
			w.WriteHeader(500)
			return
		}

		utils.WriteAsJSON(w, table)
	})

	httpMux.HandleFunc("/debug/process_cache", func(w http.ResponseWriter, r *http.Request) {
		ctx, cancelFunc := context.WithTimeout(r.Context(), 30*time.Second)
		defer cancelFunc()
		cache, err := nt.tracer.DebugDumpProcessCache(ctx)
		if err != nil {
			log.Errorf("unable to dump tracer process cache: %s", err)
			w.WriteHeader(500)
			return
		}

		utils.WriteAsJSON(w, cache)
	})

	httpMux.HandleFunc("/debug/telemetry", func(w http.ResponseWriter, req *http.Request) {
		metrics := telemetry.GetMetrics()
		utils.WriteAsJSON(w, metrics)
	})

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
	args := []interface{}{client, count, connectionsCount, time.Now().Sub(start)}
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

func writeConnections(w http.ResponseWriter, marshaler encoding.Marshaler, cs *network.Connections) {
	defer network.Reclaim(cs)

	buf, err := marshaler.Marshal(cs)
	if err != nil {
		log.Errorf("unable to marshall connections with type %s: %s", marshaler.ContentType(), err)
		w.WriteHeader(500)
		return
	}

	w.Header().Set("Content-type", marshaler.ContentType())
	w.Write(buf) //nolint:errcheck
	log.Tracef("/connections: %d connections, %d bytes", len(cs.Conns), len(buf))
}

func startTelemetryReporter(cfg *config.Config, done <-chan struct{}) {
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
