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

	Lc        fx.Lifecycle
	Config    config.Component
	Log       log.Component
	Hostname  hostnameinterface.Component
	Collector option.Option[collector.Component]
}

type doctorImpl struct {
	log       log.Component
	config    config.Component
	hostname  hostnameinterface.Component
	collector option.Option[collector.Component]
	startTime time.Time

	// Delta tracking for logs rate calculation
	logsDeltaMu            sync.Mutex
	previousLogsBytesRead  map[string]int64 // Map of service name to bytes read
	lastLogsCollectionTime time.Time
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
		log:                    deps.Log,
		config:                 deps.Config,
		hostname:               deps.Hostname,
		collector:              deps.Collector,
		startTime:              time.Now(),
		previousLogsBytesRead:  make(map[string]int64),
		lastLogsCollectionTime: time.Now(),
	}

	deps.Lc.Append(fx.Hook{
		OnStart: d.start,
		OnStop:  d.stop,
	})

	return d
}

func (d *doctorImpl) start(_ context.Context) error {
	d.log.Info("Doctor component started")
	return nil
}

func (d *doctorImpl) stop(_ context.Context) error {
	d.log.Info("Doctor component stopped")
	return nil
}

// GetStatus returns the current doctor status by aggregating telemetry
func (d *doctorImpl) GetStatus() *def.DoctorStatus {
	return &def.DoctorStatus{
		Timestamp: time.Now(),
		Ingestion: d.collectIngestionStatus(),
		Agent:     d.collectAgentStatus(),
		Intake:    d.collectIntakeStatus(),
		Services:  d.collectServicesStatus(),
	}
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
