package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"sync/atomic"
	"time"

	"github.com/DataDog/datadog-agent/pkg/process/statsd"
	"github.com/DataDog/datadog-agent/pkg/util/log"

	"github.com/DataDog/datadog-agent/pkg/network"
	"github.com/DataDog/datadog-agent/pkg/network/encoding"
	"github.com/DataDog/datadog-agent/pkg/process/config"
	"github.com/DataDog/datadog-agent/pkg/process/net"
)

// ErrSysprobeUnsupported is the unsupported error prefix, for error-class matching from callers
var ErrSysprobeUnsupported = errors.New("system-probe unsupported")

var inactivityLogDuration = 10 * time.Minute

// SystemProbe maintains and starts the underlying network connection collection process as well as
// exposes these connections over HTTP (via UDS)
type SystemProbe struct {
	cfg *config.AgentConfig

	tracer *network.Tracer
	conn   net.Conn
}

// CreateSystemProbe creates a SystemProbe as well as it's UDS socket after confirming that the OS supports BPF-based
// system probe
func CreateSystemProbe(cfg *config.AgentConfig) (*SystemProbe, error) {
	// Checking whether the current OS + kernel version is supported by the tracer
	if supported, msg := network.IsTracerSupportedByOS(cfg.ExcludedBPFLinuxVersions); !supported {
		return nil, fmt.Errorf("%s: %s", ErrSysprobeUnsupported, msg)
	}

	log.Infof("Creating tracer for: %s", filepath.Base(os.Args[0]))

	t, err := network.NewTracer(config.SysProbeConfigFromConfig(cfg))
	if err != nil {
		return nil, err
	}

	conn, err := net.NewListener(cfg)
	if err != nil {
		return nil, err
	}

	return &SystemProbe{
		tracer: t,
		cfg:    cfg,
		conn:   conn,
	}, nil
}

// Run makes available the HTTP endpoint for network collection
func (nt *SystemProbe) Run() {
	// if a debug port is specified, we expose the default handler to that port
	if nt.cfg.SystemProbeDebugPort > 0 {
		go http.ListenAndServe(fmt.Sprintf("localhost:%d", nt.cfg.SystemProbeDebugPort), http.DefaultServeMux)
	}

	var runCounter uint64

	// We don't want the endpoint for the system-probe output to be mixed with pprof and expvar
	// We can only do this by creating a new HTTP Mux that does not have these endpoints handled
	httpMux := http.NewServeMux()

	httpMux.HandleFunc("/status", func(w http.ResponseWriter, req *http.Request) {})

	httpMux.HandleFunc("/connections", func(w http.ResponseWriter, req *http.Request) {
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

		count := atomic.AddUint64(&runCounter, 1)
		logRequests(id, count, len(cs.Conns), start)
	})

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
		stats, err := nt.tracer.DebugState(getClientID(req))
		if err != nil {
			log.Errorf("unable to retrieve tracer stats: %s", err)
			w.WriteHeader(500)
			return
		}

		writeAsJSON(w, stats)
	})

	httpMux.HandleFunc("/debug/stats", func(w http.ResponseWriter, req *http.Request) {
		stats, err := nt.tracer.GetStats()
		if err != nil {
			log.Errorf("unable to retrieve tracer stats: %s", err)
			w.WriteHeader(500)
			return
		}

		writeAsJSON(w, stats)
	})

	go func() {
		tags := []string{
			fmt.Sprintf("version:%s", Version),
			fmt.Sprintf("revision:%s", GitCommit),
		}
		heartbeat := time.NewTicker(15 * time.Second)
		for range heartbeat.C {
			statsd.Client.Gauge("datadog.system_probe.agent", 1, tags, 1)
		}
	}()

	// Convenience logging if nothing has made any requests to the system-probe in some time, let's log something.
	// This should be helpful for customers + support to debug the underlying issue.
	time.AfterFunc(inactivityLogDuration, func() {
		if run := atomic.LoadUint64(&runCounter); run == 0 {
			log.Warnf("%v since the agent started without activity, the process-agent may not be configured correctly and/or running", inactivityLogDuration)
		}
	})

	http.Serve(nt.conn.GetListener(), httpMux)
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
	buf, err := marshaler.Marshal(cs)
	if err != nil {
		log.Errorf("unable to marshall connections with type %s: %s", marshaler.ContentType(), err)
		w.WriteHeader(500)
		return
	}

	w.Header().Set("Content-type", marshaler.ContentType())
	w.Write(buf)
	log.Tracef("/connections: %d connections, %d bytes", len(cs.Conns), len(buf))
}

func writeAsJSON(w http.ResponseWriter, data interface{}) {
	buf, err := json.Marshal(data)
	if err != nil {
		log.Errorf("unable to marshall connections into JSON: %s", err)
		w.WriteHeader(500)
		return
	}
	w.Write(buf)
}

// Close will stop all system probe activities
func (nt *SystemProbe) Close() {
	nt.conn.Stop()
	nt.tracer.Stop()
}
