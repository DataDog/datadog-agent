// +build linux windows

package modules

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"sync/atomic"
	"time"

	"github.com/pkg/errors"

	"github.com/DataDog/datadog-agent/cmd/system-probe/module"
	"github.com/DataDog/datadog-agent/pkg/ebpf"
	"github.com/DataDog/datadog-agent/pkg/network"
	"github.com/DataDog/datadog-agent/pkg/network/encoding"
	"github.com/DataDog/datadog-agent/pkg/process/config"
	processnet "github.com/DataDog/datadog-agent/pkg/process/net"
	"github.com/DataDog/datadog-agent/pkg/process/statsd"
	secmodule "github.com/DataDog/datadog-agent/pkg/security/module"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// ErrSysprobeUnsupported is the unsupported error prefix, for error-class matching from callers
var ErrSysprobeUnsupported = errors.New("system-probe unsupported")

var inactivityLogDuration = 10 * time.Minute

// SystemProbe maintains and starts the underlying network connection collection process as well as
// exposes these connections over HTTP (via UDS)
type SystemProbe struct {
	cfg    *config.AgentConfig
	loader *module.Loader

	tracer *ebpf.Tracer
	conn   processnet.Conn

		t, err := ebpf.NewTracer(config.SysProbeConfigFromConfig(cfg))
		return &networkTracer{tracer: t}, err
	},
}

// CreateSystemProbe creates a SystemProbe as well as it's UDS socket after confirming that the OS supports BPF-based
// system probe
func CreateSystemProbe(cfg *config.AgentConfig) (*SystemProbe, error) {
	// Checking whether the current OS + kernel version is supported by the tracer
	if supported, msg := ebpf.IsTracerSupportedByOS(cfg.ExcludedBPFLinuxVersions); !supported {
		return nil, fmt.Errorf("%s: %s", ErrSysprobeUnsupported, msg)
	}

	log.Infof("Creating tracer for: %s", filepath.Base(os.Args[0]))

	t, err := ebpf.NewTracer(config.SysProbeConfigFromConfig(cfg))
	if err != nil {
		return nil, err
	}

	var tqlt *ebpf.TCPQueueLengthTracer
	if cfg.CheckIsEnabled("TCP queue length") {
		log.Infof("Starting the TCP queue length tracer")
		tqlt, err = ebpf.NewTCPQueueLengthTracer()
		if err != nil {
			log.Errorf("unable to start the TCP queue length tracer: %v", err)
		}
	} else {
		log.Infof("TCP queue length tracer disabled")
	}

	// Setting up the unix socket
	conn, err := processnet.NewListener(cfg)
	if err != nil {
		return nil, err
	}

	factories := []module.Factory{
		{
			Name: "runtime-security-module",
			Fn:   secmodule.NewModule,
		},
	}

	loader := module.NewLoader()
	if err := loader.Register(cfg, nil, factories); err != nil {
		return nil, errors.Wrap(err, "failed to register modules")
	}

	return &SystemProbe{
		tracer:               t,
		tcpQueueLengthTracer: tqlt,
		cfg:                  cfg,
		conn:                 conn,
		loader:               loader,
	}, nil
}

func (nt *networkTracer) GetStats() map[string]interface{} {
	stats, _ := nt.tracer.GetStats()
	return stats
}

// Register all networkTracer endpoints
func (nt *networkTracer) Register(httpMux *http.ServeMux) error {
	var runCounter uint64

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
		stats, err := nt.tracer.DebugNetworkState(getClientID(req))
		if err != nil {
			log.Errorf("unable to retrieve tracer stats: %s", err)
			w.WriteHeader(500)
			return
		}

		utils.WriteAsJSON(w, stats)
	})

	httpMux.HandleFunc("/debug/stats", func(w http.ResponseWriter, req *http.Request) {
		stats, err := nt.tracer.GetStats()
		if err != nil {
			log.Errorf("unable to retrieve tracer stats: %s", err)
			w.WriteHeader(500)
			return
		}

		for k, v := range nt.loader.GetStats() {
			stats[k] = v
		}

		writeAsJSON(w, stats)
	})

	httpMux.HandleFunc("/check/tcp_queue_length", func(w http.ResponseWriter, req *http.Request) {
		if nt.tcpQueueLengthTracer == nil {
			log.Errorf("TCP queue length tracer was not properly initialized")
			w.WriteHeader(500)
			return
		}
		stats := nt.tcpQueueLengthTracer.GetAndFlush()

		writeAsJSON(w, stats)
	})

	go func() {
		tags := []string{
			fmt.Sprintf("version:%s", Version),
			fmt.Sprintf("revision:%s", GitCommit),
		}
		heartbeat := time.NewTicker(15 * time.Second)
		for range heartbeat.C {
			statsd.Client.Gauge("datadog.system_probe.agent", 1, tags, 1) //nolint:errcheck
		}
	}()

	// Convenience logging if nothing has made any requests to the system-probe in some time, let's log something.
	// This should be helpful for customers + support to debug the underlying issue.
	time.AfterFunc(inactivityLogDuration, func() {
		if run := atomic.LoadUint64(&runCounter); run == 0 {
			log.Warnf("%v since the agent started without activity, the process-agent may not be configured correctly and/or running", inactivityLogDuration)
		}
	})

	return nil
}

// Close will stop all system probe activities
func (nt *networkTracer) Close() {
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

func writeAsJSON(w http.ResponseWriter, data interface{}) {
	buf, err := json.Marshal(data)
	if err != nil {
		log.Errorf("unable to marshall connections into JSON: %s", err)
		w.WriteHeader(500)
		return
	}
	w.Write(buf) //nolint:errcheck
}

// Close will stop all system probe activities
func (nt *SystemProbe) Close() {
	nt.conn.Stop()
	nt.tracer.Stop()
	nt.loader.Close()
}
