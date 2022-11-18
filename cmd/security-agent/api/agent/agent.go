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

	"github.com/gorilla/mux"

	"github.com/DataDog/datadog-agent/cmd/agent/common"
	"github.com/DataDog/datadog-agent/cmd/agent/common/signals"
	compagent "github.com/DataDog/datadog-agent/pkg/compliance/agent"
	"github.com/DataDog/datadog-agent/pkg/config"
	settingshttp "github.com/DataDog/datadog-agent/pkg/config/settings/http"
	"github.com/DataDog/datadog-agent/pkg/flare"
	secagent "github.com/DataDog/datadog-agent/pkg/security/agent"
	"github.com/DataDog/datadog-agent/pkg/status"
	"github.com/DataDog/datadog-agent/pkg/status/health"
	"github.com/DataDog/datadog-agent/pkg/util/hostname"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// Agent handles REST API calls
type Agent struct {
	runtimeAgent    *secagent.RuntimeSecurityAgent
	complianceAgent *compagent.Agent
}

// NewAgent returns a new Agent
func NewAgent(runtimeAgent *secagent.RuntimeSecurityAgent, complianceAgent *compagent.Agent) *Agent {
	return &Agent{
		runtimeAgent:    runtimeAgent,
		complianceAgent: complianceAgent,
	}
}

// SetupHandlers adds the specific handlers for /agent endpoints
func (a *Agent) SetupHandlers(r *mux.Router) {
	r.HandleFunc("/version", common.GetVersion).Methods("GET")
	r.HandleFunc("/flare", a.makeFlare).Methods("POST")
	r.HandleFunc("/hostname", a.getHostname).Methods("GET")
	r.HandleFunc("/stop", a.stopAgent).Methods("POST")
	r.HandleFunc("/status", a.getStatus).Methods("GET")
	r.HandleFunc("/status/health", a.getHealth).Methods("GET")
	r.HandleFunc("/config", settingshttp.Server.GetFull("")).Methods("GET")
	r.HandleFunc("/config/list-runtime", settingshttp.Server.ListConfigurable).Methods("GET")
	r.HandleFunc("/config/{setting}", settingshttp.Server.GetValue).Methods("GET")
	r.HandleFunc("/config/{setting}", settingshttp.Server.SetValue).Methods("POST")
}

func (a *Agent) stopAgent(w http.ResponseWriter, r *http.Request) {
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

func (a *Agent) getHostname(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	hname, err := hostname.Get(r.Context())
	if err != nil {
		log.Warnf("Error getting hostname: %s\n", err) // or something like this
		hname = ""
	}
	j, err := json.Marshal(hname)
	if err != nil {
		log.Warnf("Failed to serialize json: %v", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Write(j)
}

func (a *Agent) getStatus(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	s, err := status.GetStatus()
	if err != nil {
		log.Errorf("Error getting status. Error: %v, Status: %v", err, s)
		body, _ := json.Marshal(map[string]string{"error": err.Error()})
		http.Error(w, string(body), 500)
		return
	}

	if a.runtimeAgent != nil {
		s["runtimeSecurityStatus"] = a.runtimeAgent.GetStatus()
	}

	if a.complianceAgent != nil {
		s["complianceStatus"] = a.complianceAgent.GetStatus()
	}

	jsonStats, err := json.Marshal(s)
	if err != nil {
		log.Errorf("Error marshalling status. Error: %v, Status: %v", err, s)
		body, _ := json.Marshal(map[string]string{"error": err.Error()})
		http.Error(w, string(body), 500)
		return
	}

	w.Write(jsonStats)
}

func (a *Agent) getHealth(w http.ResponseWriter, r *http.Request) {
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

func (a *Agent) makeFlare(w http.ResponseWriter, r *http.Request) {
	log.Infof("Making a flare")
	w.Header().Set("Content-Type", "application/json")
	logFile := config.Datadog.GetString("security_agent.log_file")

	var runtimeAgentStatus, complianceStatus map[string]interface{}
	if a.runtimeAgent != nil {
		runtimeAgentStatus = a.runtimeAgent.GetStatus()
	}

	if a.complianceAgent != nil {
		complianceStatus = a.complianceAgent.GetStatus()
	}

	filePath, err := flare.CreateSecurityAgentArchive(false, logFile, runtimeAgentStatus, complianceStatus)
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
