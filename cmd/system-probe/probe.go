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
	"github.com/DataDog/datadog-agent/pkg/process/util"
	"github.com/DataDog/datadog-agent/pkg/util/log"

	"github.com/DataDog/datadog-agent/pkg/ebpf"
	"github.com/DataDog/datadog-agent/pkg/process/config"
	"github.com/DataDog/datadog-agent/pkg/process/net"
	"github.com/mailru/easyjson/jwriter"
)

// ErrTracerUnsupported is the unsupported error prefix, for error-class matching from callers
var ErrTracerUnsupported = errors.New("tracer unsupported")

// SystemProbe maintains and starts the underlying network connection collection process as well as
// exposes these connections over HTTP (via UDS)
type SystemProbe struct {
	cfg *config.AgentConfig

	supported bool
	tracer    *ebpf.Tracer
	conn      net.Conn
}

// CreateSystemProbe creates a SystemProbe as well as it's UDS socket after confirming that the OS supports BPF-based
// system probe
func CreateSystemProbe(cfg *config.AgentConfig) (*SystemProbe, error) {
	var err error
	nt := &SystemProbe{}

	// Checking whether the current OS + kernel version is supported by the tracer
	if nt.supported, err = ebpf.IsTracerSupportedByOS(cfg.SkipLinuxVersionCheck, cfg.ExcludedBPFLinuxVersions); err != nil {
		return nil, fmt.Errorf("%s: %s", ErrTracerUnsupported, err)
	}

	// make sure debugfs is mounted
	mounted := util.IsDebugfsMounted()
	if !mounted {
		return nil, fmt.Errorf("%s: debugfs is not mounted and is needed for eBPF-based checks, run \"sudo mount -t debugfs none /sys/kernel/debug\" to mount debugfs", ErrTracerUnsupported)
	}

	log.Infof("Creating tracer for: %s", filepath.Base(os.Args[0]))

	t, err := ebpf.NewTracer(config.SysProbeConfigFromConfig(cfg))
	if err != nil {
		return nil, err
	}

	// Setting up the unix socket
	uds, err := net.NewUDSListener(cfg)
	if err != nil {
		return nil, err
	}

	nt.tracer = t
	nt.cfg = cfg
	nt.conn = uds
	return nt, nil
}

// Run makes available the HTTP endpoint for network collection
func (nt *SystemProbe) Run() {
	// if a debug port is specified, we expose the default handler to that port
	if nt.cfg.SystemProbeDebugPort > 0 {
		go http.ListenAndServe(fmt.Sprintf("localhost:%d", nt.cfg.SystemProbeDebugPort), http.DefaultServeMux)
	}

	// We don't want the endpoint for systemprobe output to be mixed with pprof and expvar
	// We can only do this by creating a new HTTP Mux that does not have these endpoints handled
	httpMux := http.NewServeMux()

	httpMux.HandleFunc("/status", func(w http.ResponseWriter, req *http.Request) {})

	var runCounter uint64
	httpMux.HandleFunc("/connections", func(w http.ResponseWriter, req *http.Request) {
		start := time.Now()
		id := getClientID(req)
		cs, err := nt.tracer.GetActiveConnections(id)
		if err != nil {
			log.Errorf("unable to retrieve connections: %s", err)
			w.WriteHeader(500)
			return
		}
		writeConnections(w, cs)

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

		writeConnections(w, cs)
	})

	httpMux.HandleFunc("/debug/net_state", func(w http.ResponseWriter, req *http.Request) {
		stats, err := nt.tracer.DebugNetworkState(getClientID(req))
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
		heartbeat := time.NewTicker(15 * time.Second)
		for range heartbeat.C {
			statsd.Client.Gauge("datadog.system_probe.agent", 1, []string{"version:" + Version}, 1)
		}
	}()

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
	var clientID = ebpf.DEBUGCLIENT
	if rawCID := req.URL.Query().Get("client_id"); rawCID != "" {
		clientID = rawCID
	}
	return clientID
}

func writeConnections(w http.ResponseWriter, cs *ebpf.Connections) {
	jw := &jwriter.Writer{}
	cs.MarshalEasyJSON(jw)
	if err := jw.Error; err != nil {
		log.Errorf("unable to marshall connections into JSON: %s", err)
		w.WriteHeader(500)
		return
	}
	bytesWritten, err := jw.DumpTo(w)
	if err != nil {
		log.Errorf("unable to dump JSON to response: %s", err)
		w.WriteHeader(500)
		return
	}
	log.Tracef("/connections: %d connections, %d bytes", len(cs.Conns), bytesWritten)
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
