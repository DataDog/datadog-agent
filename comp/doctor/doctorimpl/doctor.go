// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package doctorimpl provides the implementation of the doctor component
package doctorimpl

import (
	"context"
	"encoding/json"
	"net/http"
	"sync"
	"time"

	"go.uber.org/fx"

	api "github.com/DataDog/datadog-agent/comp/api/api/def"
	"github.com/DataDog/datadog-agent/comp/collector/collector"
	"github.com/DataDog/datadog-agent/comp/core/config"
	"github.com/DataDog/datadog-agent/comp/core/hostname/hostnameinterface"
	ipc "github.com/DataDog/datadog-agent/comp/core/ipc/def"
	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	"github.com/DataDog/datadog-agent/comp/core/status"
	"github.com/DataDog/datadog-agent/comp/doctor/def"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
	httputils "github.com/DataDog/datadog-agent/pkg/util/http"
	"github.com/DataDog/datadog-agent/pkg/util/option"
	"github.com/DataDog/datadog-agent/pkg/version"
)

// team: agent-runtimes

type dependencies struct {
	fx.In

	Lc         fx.Lifecycle
	Config     config.Component
	Log        log.Component
	Hostname   hostnameinterface.Component
	Collector  option.Option[collector.Component]
	HTTPClient ipc.HTTPClient
}

type doctorImpl struct {
	log        log.Component
	config     config.Component
	hostname   hostnameinterface.Component
	httpclient ipc.HTTPClient
	collector  option.Option[collector.Component]
	startTime  time.Time

	// Cached status computed by background ticker
	statusMu     sync.RWMutex
	cachedStatus *def.DoctorStatus

	// Background ticker for periodic stat collection
	ticker     *time.Ticker
	stopChan   chan struct{}
	tickerDone chan struct{}

	// Delta tracking for logs rate calculation
	logsDeltaMu            sync.Mutex
	previousLogsBytesRead  map[string]int64 // Map of service name to bytes read
	lastLogsCollectionTime time.Time

	// Delta tracking for DogStatsD rate calculation
	dogstatsdDeltaMu            sync.Mutex
	previousDogstatsdSamples    map[string]int64 // Map of service name to metric sample count
	lastDogstatsdCollectionTime time.Time

	// Delta tracking for traces rate calculation
	tracesDeltaMu            sync.Mutex
	previousTracesReceived   map[string]int64 // Map of service name to traces received
	lastTracesCollectionTime time.Time
}

type provides struct {
	fx.Out

	Comp         def.Component
	APIEndpoint  api.AgentEndpointProvider
	StatusHeader status.HeaderInformationProvider
}

// Module defines the fx options for this component
func Module() fxutil.Module {
	return fxutil.Component(
		fx.Provide(newProvides),
	)
}

func newProvides(deps dependencies) provides {
	d := newDoctor(deps)

	return provides{
		Comp:         d,
		APIEndpoint:  api.NewAgentEndpointProvider(d.handleDoctor, "/doctor", "GET"),
		StatusHeader: status.NewHeaderInformationProvider(&StatusProvider{}),
	}
}

func newDoctor(deps dependencies) *doctorImpl {
	d := &doctorImpl{
		log:                         deps.Log,
		config:                      deps.Config,
		hostname:                    deps.Hostname,
		collector:                   deps.Collector,
		httpclient:                  deps.HTTPClient,
		startTime:                   time.Now(),
		stopChan:                    make(chan struct{}),
		tickerDone:                  make(chan struct{}),
		previousLogsBytesRead:       make(map[string]int64),
		lastLogsCollectionTime:      time.Now(),
		previousDogstatsdSamples:    make(map[string]int64),
		lastDogstatsdCollectionTime: time.Now(),
		previousTracesReceived:      make(map[string]int64),
		lastTracesCollectionTime:    time.Now(),
	}

	deps.Lc.Append(fx.Hook{
		OnStart: d.start,
		OnStop:  d.stop,
	})

	return d
}

func (d *doctorImpl) start(_ context.Context) error {
	d.log.Info("Doctor component started")

	// Compute initial status immediately
	d.updateCachedStatus()

	// Start background ticker to update stats every second
	d.ticker = time.NewTicker(1 * time.Second)
	go d.statsCollectionLoop()

	return nil
}

func (d *doctorImpl) stop(_ context.Context) error {
	d.log.Info("Doctor component stopping")

	// Stop the ticker and wait for goroutine to finish
	if d.ticker != nil {
		d.ticker.Stop()
	}
	close(d.stopChan)
	<-d.tickerDone

	d.log.Info("Doctor component stopped")
	return nil
}

// statsCollectionLoop runs in background and updates cached stats every second
func (d *doctorImpl) statsCollectionLoop() {
	defer close(d.tickerDone)

	for {
		select {
		case <-d.ticker.C:
			// Update cached status every second
			d.updateCachedStatus()
		case <-d.stopChan:
			// Stop requested
			return
		}
	}
}

// updateCachedStatus computes stats and updates the cached status
func (d *doctorImpl) updateCachedStatus() {
	status := &def.DoctorStatus{
		Timestamp: time.Now(),
		Ingestion: d.collectIngestionStatus(),
		Agent:     d.collectAgentStatus(),
		Intake:    d.collectIntakeStatus(),
		Services:  d.collectServicesStatus(),
	}

	d.statusMu.Lock()
	d.cachedStatus = status
	d.statusMu.Unlock()
}

// GetStatus returns the cached doctor status (updated every second by background ticker)
func (d *doctorImpl) GetStatus() *def.DoctorStatus {
	d.statusMu.RLock()
	defer d.statusMu.RUnlock()

	// Return cached status (nil-safe)
	if d.cachedStatus == nil {
		// Fallback if not yet initialized (shouldn't happen in normal operation)
		return &def.DoctorStatus{
			Timestamp: time.Now(),
		}
	}

	return d.cachedStatus
}

// handleDoctor is the HTTP handler for the /agent/doctor endpoint
func (d *doctorImpl) handleDoctor(w http.ResponseWriter, _ *http.Request) {
	status := d.GetStatus()

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(status); err != nil {
		d.log.Errorf("Error encoding doctor status: %v", err)
		httputils.SetJSONError(w, err, http.StatusInternalServerError)
	}
}

func (d *doctorImpl) collectAgentStatus() def.AgentStatus {
	hostname, _ := d.hostname.Get(context.Background())

	return def.AgentStatus{
		Running:        true, // If we're responding, we're running
		Version:        version.AgentVersion,
		Hostname:       hostname,
		Uptime:         time.Since(d.startTime),
		ErrorsLast5Min: 0, // TODO: implement error counting
		Health:         d.collectHealthStatus(),
		Tags:           d.collectTags(),
	}
}

func (d *doctorImpl) collectTags() []string {
	// TODO: Integrate with tagger component
	tags := []string{}

	// Get basic tags from config
	if configTags := d.config.GetStringSlice("tags"); len(configTags) > 0 {
		tags = append(tags, configTags...)
	}

	return tags
}
