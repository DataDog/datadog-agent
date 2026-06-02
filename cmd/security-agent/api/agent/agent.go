// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package agent implements the api endpoints for the `/agent` prefix.
// This group of endpoints is meant to provide high-level functionalities
// at the agent level.
package agent

import (
	"encoding/json"
	"net/http"

	"github.com/DataDog/datadog-agent/cmd/agent/common/signals"
	secrets "github.com/DataDog/datadog-agent/comp/core/secrets/def"
	settings "github.com/DataDog/datadog-agent/comp/core/settings/def"
	"github.com/DataDog/datadog-agent/comp/core/status"
	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	"github.com/DataDog/datadog-agent/pkg/api/coverage"
	"github.com/DataDog/datadog-agent/pkg/api/version"
	pkgconfigsetup "github.com/DataDog/datadog-agent/pkg/config/setup"
	"github.com/DataDog/datadog-agent/pkg/flare/securityagent"
	"github.com/DataDog/datadog-agent/pkg/status/health"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// Agent handles REST API calls
type Agent struct {
	statusComponent status.Component
	settings        settings.Component
	wmeta           workloadmeta.Component
	secrets         secrets.Component
}

// NewAgent returns a new Agent
func NewAgent(statusComponent status.Component, settings settings.Component, wmeta workloadmeta.Component, secrets secrets.Component) *Agent {
	return &Agent{
		statusComponent: statusComponent,
		settings:        settings,
		wmeta:           wmeta,
		secrets:         secrets,
	}
}

// SetupHandlers adds the specific handlers for /agent endpoints
func (a *Agent) SetupHandlers(r *http.ServeMux) {
	r.HandleFunc("GET /version", version.Get)
	r.HandleFunc("POST /flare", a.makeFlare)
	r.HandleFunc("POST /stop", a.stopAgent)
	r.HandleFunc("GET /status", a.getStatus)
	r.HandleFunc("GET /status/health", a.getHealth)
	r.HandleFunc("GET /config", a.settings.GetFullConfig(""))
	r.HandleFunc("GET /config/without-defaults", a.settings.GetFullConfigWithoutDefaults(""))
	// FIXME: this returns the entire datadog.yaml and not just security-agent.yaml config
	r.HandleFunc("GET /config/by-source", a.settings.GetFullConfigBySource())
	r.HandleFunc("GET /config/list-runtime", a.settings.ListConfigurable)
	r.HandleFunc("GET /config/{setting}", a.settings.GetValue)
	r.HandleFunc("POST /config/{setting}", a.settings.SetValue)
	r.HandleFunc("GET /workload-list", func(w http.ResponseWriter, r *http.Request) {
		verbose := r.URL.Query().Get("verbose") == "true"
		workloadList(w, verbose, a.wmeta)
	})
	r.HandleFunc("GET /secret/refresh", a.refreshSecrets)

	// Special handler to compute running agent Code coverage
	coverage.SetupCoverageHandler(r)
}

func workloadList(w http.ResponseWriter, verbose bool, wmeta workloadmeta.Component) {
	response := wmeta.Dump(verbose)
	jsonDump, err := json.Marshal(response)
	if err != nil {
		err := log.Errorf("Unable to marshal workload list response: %v", err)
		w.Header().Set("Content-Type", "application/json")
		body, _ := json.Marshal(map[string]string{"error": err.Error()})
		http.Error(w, string(body), 500)
		return
	}

	w.Write(jsonDump)
}

func (a *Agent) stopAgent(w http.ResponseWriter, _ *http.Request) {
	signals.Stopper <- true
	w.Header().Set("Content-Type", "application/json")
	j, err := json.Marshal("")
	if err != nil {
		log.Warnf("Failed to serialize json: %v", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Write(j)
}

func (a *Agent) getStatus(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	format := r.URL.Query().Get("format")

	s, err := a.statusComponent.GetStatus(format, false)
	if err != nil {
		log.Errorf("Error getting status. Error: %v, Status: %v", err, s)
		body, _ := json.Marshal(map[string]string{"error": err.Error()})
		http.Error(w, string(body), 500)
		return
	}

	w.Write(s)
}

func (a *Agent) getHealth(w http.ResponseWriter, _ *http.Request) {
	h := health.GetReady()

	if len(h.Unhealthy) > 0 {
		log.Debugf("Healthcheck failed on: %v", h.Unhealthy)
	}

	jsonHealth, err := json.Marshal(h)
	if err != nil {
		log.Errorf("Error marshalling status. Error: %v, Status: %v", err, h)
		body, _ := json.Marshal(map[string]string{"error": err.Error()})
		http.Error(w, string(body), 500)
		return
	}

	w.Write(jsonHealth)
}

func (a *Agent) makeFlare(w http.ResponseWriter, _ *http.Request) {
	log.Infof("Making a flare")
	w.Header().Set("Content-Type", "application/json")
	logFile := pkgconfigsetup.Datadog().GetString("security_agent.log_file")

	filePath, err := securityagent.CreateSecurityAgentArchive(false, logFile, a.statusComponent)
	if err != nil || filePath == "" {
		if err != nil {
			log.Errorf("The flare failed to be created: %s", err)
		} else {
			log.Warnf("The flare failed to be created")
		}
		http.Error(w, err.Error(), 500)
	}
	w.Write([]byte(filePath))
}

func (a *Agent) refreshSecrets(w http.ResponseWriter, _ *http.Request) {
	res, err := a.secrets.RefreshNow()
	if err != nil {
		log.Errorf("error while refreshing secrets: %s", err)
		w.Header().Set("Content-Type", "application/json")
		body, _ := json.Marshal(map[string]string{"error": err.Error()})
		http.Error(w, string(body), http.StatusInternalServerError)
		return
	}
	w.Write([]byte(res))
}
