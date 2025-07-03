// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

package modules

import (
	"io"
	"net/http"
	"os"
	"path/filepath"
	"sync/atomic"
	"time"

	"github.com/DataDog/datadog-agent/pkg/process/encoding"
	reqEncoding "github.com/DataDog/datadog-agent/pkg/process/encoding/request"
	"github.com/DataDog/datadog-agent/pkg/process/procutil"
	"github.com/DataDog/datadog-agent/pkg/system-probe/api/module"
	"github.com/DataDog/datadog-agent/pkg/system-probe/config"
	sysconfigtypes "github.com/DataDog/datadog-agent/pkg/system-probe/config/types"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

func init() { registerModule(Process) }

// Process is a module that fetches process level data
var Process = &module.Factory{
	Name:             config.ProcessModule,
	ConfigNamespaces: []string{},
	Fn: func(_ *sysconfigtypes.Config, _ module.FactoryDependencies) (module.Module, error) {
		log.Infof("Creating process module for: %s", filepath.Base(os.Args[0]))

		// we disable returning zero values for stats to reduce parsing work on process-agent side
		p := procutil.NewProcessProbe(procutil.WithReturnZeroPermStats(false))
		return &process{
			probe: p,
		}, nil
	},
	NeedsEBPF: func() bool {
		return false
	},
}

var _ module.Module = &process{}

type process struct {
	probe           procutil.Probe
	lastCheck       atomic.Int64
	statsRunCounter atomic.Uint64
}

// GetStats returns stats for the module
func (t *process) GetStats() map[string]interface{} {
	return map[string]interface{}{
		"last_check": t.lastCheck.Load(),
	}
}

// Register registers endpoints for the module to expose data
func (t *process) Register(httpMux *module.Router) error {
	httpMux.HandleFunc("/stats", t.statsHandler).Methods("POST")
	httpMux.HandleFunc("/service", t.serviceHandler).Methods("POST")
	httpMux.HandleFunc("/network", t.networkHandler).Methods("POST")
	return nil
}

// statsHandler handles requests for process IO stats
func (t *process) statsHandler(w http.ResponseWriter, req *http.Request) {
	start := time.Now()
	t.lastCheck.Store(start.Unix())
	pids, err := getPids(req)
	if err != nil {
		log.Errorf("Unable to get PIDs from request: %s", err)
		w.WriteHeader(http.StatusBadRequest)
	}

	stats, err := t.probe.StatsWithPermByPID(pids)
	if err != nil {
		log.Errorf("unable to retrieve process stats: %s", err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	contentType := req.Header.Get("Accept")
	marshaler := encoding.GetMarshaler(contentType)
	writeStats(w, marshaler, stats)

	count := t.statsRunCounter.Add(1)
	logProcTracerRequests(count, len(stats), start)
}

// serviceHandler handles requests for service information for given processes
func (t *process) serviceHandler(_ http.ResponseWriter, _ *http.Request) {
	// TODO: Add implementation for this handler
}

// networkHandler handles requests for network stats for given processes
func (t *process) networkHandler(_ http.ResponseWriter, _ *http.Request) {
	// TODO: Add implementation for this handler
}

// Close cleans up the underlying probe object
func (t *process) Close() {
	if t.probe != nil {
		t.probe.Close()
	}
}

func logProcTracerRequests(count uint64, statsCount int, start time.Time) {
	args := []interface{}{string(config.ProcessModule), count, statsCount, time.Since(start)}
	msg := "Got request on /%s/stats (count: %d): retrieved %d stats in %s"
	switch {
	case count <= 5, count%20 == 0:
		log.Infof(msg, args...)
	default:
		log.Debugf(msg, args...)
	}
}

func writeStats(w http.ResponseWriter, marshaler encoding.Marshaler, stats map[int32]*procutil.StatsWithPerm) {
	buf, err := marshaler.Marshal(stats)
	if err != nil {
		log.Errorf("unable to marshall stats with type %s: %s", marshaler.ContentType(), err)
		w.WriteHeader(500)
		return
	}

	w.Header().Set("Content-type", marshaler.ContentType())
	w.Write(buf)
	log.Tracef("/%s/stats: %d stats, %d bytes", string(config.ProcessModule), len(stats), len(buf))
}

func getPids(r *http.Request) ([]int32, error) {
	contentType := r.Header.Get("Content-Type")
	body, err := io.ReadAll(r.Body)
	if err != nil {
		return nil, err
	}

	procReq, err := reqEncoding.GetUnmarshaler(contentType).Unmarshal(body)
	if err != nil {
		return nil, err
	}

	return procReq.Pids, nil
}
