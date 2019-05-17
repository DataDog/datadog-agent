package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"

	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/mailru/easyjson"

	"github.com/DataDog/datadog-agent/pkg/ebpf"
	"github.com/DataDog/datadog-agent/pkg/process/config"
	"github.com/DataDog/datadog-agent/pkg/process/net"
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
	if nt.supported, err = ebpf.IsTracerSupportedByOS(cfg.ExcludedBPFLinuxVersions); err != nil {
		return nil, fmt.Errorf("%s: %s", ErrTracerUnsupported, err)
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
	httpMux := http.DefaultServeMux

	// If profiling is disabled, then we should overwrite handlers for the pprof endpoints
	// that were registered in init():
	// https://github.com/golang/go/blob/5bd88b0/src/net/http/pprof/pprof.go#L72-L78
	// We can only do this by creating a new HTTP Mux that does not have these endpoints handled
	if !nt.cfg.EnableDebugProfiling {
		httpMux = http.NewServeMux()
	}

	httpMux.HandleFunc("/status", func(w http.ResponseWriter, req *http.Request) {})

	httpMux.HandleFunc("/connections", func(w http.ResponseWriter, req *http.Request) {
		cs, err := nt.tracer.GetActiveConnections(getClientID(req))
		if err != nil {
			log.Errorf("unable to retrieve connections: %s", err)
			w.WriteHeader(500)
			return
		}
		writeConnections(w, cs)
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

	http.Serve(nt.conn.GetListener(), httpMux)
}

func getClientID(req *http.Request) string {
	var clientID = ebpf.DEBUGCLIENT
	if rawCID := req.URL.Query().Get("client_id"); rawCID != "" {
		clientID = rawCID
	}
	return clientID
}

func writeConnections(w http.ResponseWriter, cs *ebpf.Connections) {
	buf, err := easyjson.Marshal(cs)
	if err != nil {
		log.Errorf("unable to marshall connections into JSON: %s", err)
		w.WriteHeader(500)
		return
	}
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
