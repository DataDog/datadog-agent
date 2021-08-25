// +build linux windows

package modules

import (
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"sync/atomic"
	"time"

	"github.com/DataDog/datadog-agent/cmd/system-probe/api/module"
	"github.com/DataDog/datadog-agent/cmd/system-probe/config"
	"github.com/DataDog/datadog-agent/cmd/system-probe/utils"
	"github.com/DataDog/datadog-agent/pkg/network"
	networkconfig "github.com/DataDog/datadog-agent/pkg/network/config"
	"github.com/DataDog/datadog-agent/pkg/network/encoding"
	"github.com/DataDog/datadog-agent/pkg/network/http/debugging"
	"github.com/DataDog/datadog-agent/pkg/network/tracer"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// ErrSysprobeUnsupported is the unsupported error prefix, for error-class matching from callers
var ErrSysprobeUnsupported = errors.New("system-probe unsupported")

const inactivityLogDuration = 10 * time.Minute
const inactivityRestartDuration = 20 * time.Minute

// NetworkTracer is a factory for NPM's tracer
var NetworkTracer = module.Factory{
	Name: config.NetworkTracerModule,
	Fn: func(cfg *config.Config) (module.Module, error) {
		ncfg := networkconfig.New()

		// Checking whether the current OS + kernel version is supported by the tracer
		if supported, msg := tracer.IsTracerSupportedByOS(ncfg.ExcludedBPFLinuxVersions); !supported {
			return nil, fmt.Errorf("%w: %s", ErrSysprobeUnsupported, msg)
		}

		log.Infof("Creating tracer for: %s", filepath.Base(os.Args[0]))

		t, err := tracer.NewTracer(ncfg)
		return &networkTracer{tracer: t}, err
	},
}

var _ module.Module = &networkTracer{}

type networkTracer struct {
	tracer       *tracer.Tracer
	restartTimer *time.Timer
}

func (nt *networkTracer) GetStats() map[string]interface{} {
	stats, _ := nt.tracer.GetStats()
	return stats
}

// Register all networkTracer endpoints
func (nt *networkTracer) Register(httpMux *module.Router) error {
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
		contentType := req.Header.Get("Accept")
		marshaler := encoding.GetMarshaler(contentType)
		writeConnections(w, marshaler, cs)

		if nt.restartTimer != nil {
			nt.restartTimer.Reset(inactivityRestartDuration)
		}
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

	httpMux.HandleFunc("/debug/http_monitoring", func(w http.ResponseWriter, req *http.Request) {
		id := getClientID(req)
		cs, err := nt.tracer.GetActiveConnections(id)
		if err != nil {
			log.Errorf("unable to retrieve connections: %s", err)
			w.WriteHeader(500)
			return
		}

		utils.WriteAsJSON(w, debugging.HTTP(cs.HTTP, cs.DNS))
	})

	// Convenience logging if nothing has made any requests to the system-probe in some time, let's log something.
	// This should be helpful for customers + support to debug the underlying issue.
	time.AfterFunc(inactivityLogDuration, func() {
		if run := atomic.LoadUint64(&runCounter); run == 0 {
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
