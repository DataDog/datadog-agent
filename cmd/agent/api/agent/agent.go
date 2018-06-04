// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018 Datadog, Inc.

// Package agent implements the api endpoints for the `/agent` prefix.
// This group of endpoints is meant to provide high-level functionalities
// at the agent level.
package agent

import (
	"encoding/json"
	"fmt"
	"net/http"
	"sort"

	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/gorilla/mux"

	"github.com/DataDog/datadog-agent/cmd/agent/api/response"
	"github.com/DataDog/datadog-agent/cmd/agent/common"
	"github.com/DataDog/datadog-agent/cmd/agent/common/signals"
	"github.com/DataDog/datadog-agent/cmd/agent/gui"
	"github.com/DataDog/datadog-agent/pkg/autodiscovery"
	"github.com/DataDog/datadog-agent/pkg/collector/py"
	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/flare"
	"github.com/DataDog/datadog-agent/pkg/status"
	"github.com/DataDog/datadog-agent/pkg/status/health"
	"github.com/DataDog/datadog-agent/pkg/tagger"
	"github.com/DataDog/datadog-agent/pkg/util"
	"github.com/DataDog/datadog-agent/pkg/version"
)

// SetupHandlers adds the specific handlers for /agent endpoints
func SetupHandlers(r *mux.Router) {
	r.HandleFunc("/version", getVersion).Methods("GET")
	r.HandleFunc("/hostname", getHostname).Methods("GET")
	r.HandleFunc("/flare", makeFlare).Methods("POST")
	r.HandleFunc("/stop", stopAgent).Methods("POST")
	r.HandleFunc("/status", getStatus).Methods("GET")
	r.HandleFunc("/status/formatted", getFormattedStatus).Methods("GET")
	r.HandleFunc("/status/health", getHealth).Methods("GET")
	r.HandleFunc("/{component}/status", componentStatusGetterHandler).Methods("GET")
	r.HandleFunc("/{component}/status", componentStatusHandler).Methods("POST")
	r.HandleFunc("/{component}/configs", componentConfigHandler).Methods("GET")
	r.HandleFunc("/gui/csrf-token", getCSRFToken).Methods("GET")
	r.HandleFunc("/config-check", getConfigCheck).Methods("GET")
	r.HandleFunc("/tagger-list", getTaggerList).Methods("GET")
}

func stopAgent(w http.ResponseWriter, r *http.Request) {
	signals.Stopper <- true
	w.Header().Set("Content-Type", "application/json")
	j, _ := json.Marshal("")
	w.Write(j)
}

func getVersion(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	av, _ := version.New(version.AgentVersion, version.Commit)
	j, _ := json.Marshal(av)
	w.Write(j)
}

func getHostname(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	hname, err := util.GetHostname()
	if err != nil {
		log.Warnf("Error getting hostname: %s\n", err) // or something like this
		hname = ""
	}
	j, _ := json.Marshal(hname)
	w.Write(j)
}

func getPythonStatus(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	pyStats, err := py.GetPythonInterpreterMemoryUsage()
	if err != nil {
		log.Warnf("Error getting python stats: %s\n", err) // or something like this
		http.Error(w, err.Error(), 500)
	}

	j, _ := json.Marshal(pyStats)
	w.Write(j)
}

func makeFlare(w http.ResponseWriter, r *http.Request) {
	logFile := config.Datadog.GetString("log_file")
	if logFile == "" {
		logFile = common.DefaultLogFile
	}

	log.Infof("Making a flare")
	filePath, err := flare.CreateArchive(false, common.GetDistPath(), common.PyChecksPath, logFile)
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

func componentConfigHandler(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	component := vars["component"]
	switch component {
	case "jmx":
		getJMXConfigs(w, r)
	default:
		err := fmt.Errorf("bad url or resource does not exist")
		log.Errorf("%s", err.Error())
		http.Error(w, err.Error(), 404)
	}
}

func componentStatusGetterHandler(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	component := vars["component"]
	switch component {
	case "py":
		getPythonStatus(w, r)
	default:
		err := fmt.Errorf("bad url or resource does not exist")
		log.Errorf("%s", err.Error())
		http.Error(w, err.Error(), 404)
	}
}

func componentStatusHandler(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	component := vars["component"]
	switch component {
	case "jmx":
		setJMXStatus(w, r)
	default:
		err := fmt.Errorf("bad url or resource does not exist")
		log.Errorf("%s", err.Error())
		http.Error(w, err.Error(), 404)
	}
}

func getStatus(w http.ResponseWriter, r *http.Request) {
	log.Info("Got a request for the status. Making status.")
	s, err := status.GetStatus()
	w.Header().Set("Content-Type", "application/json")
	if err != nil {
		log.Errorf("Error getting status. Error: %v, Status: %v", err, s)
		body, _ := json.Marshal(map[string]string{"error": err.Error()})
		http.Error(w, string(body), 500)
		return
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

func getFormattedStatus(w http.ResponseWriter, r *http.Request) {
	log.Info("Got a request for the formatted status. Making formatted status.")
	s, err := status.GetAndFormatStatus()
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		log.Errorf("Error getting status. Error: %v, Status: %v", err, s)
		body, _ := json.Marshal(map[string]string{"error": err.Error()})
		http.Error(w, string(body), 500)
		return
	}

	w.Write(s)
}

func getHealth(w http.ResponseWriter, r *http.Request) {
	h := health.GetStatus()

	jsonHealth, err := json.Marshal(h)
	if err != nil {
		log.Errorf("Error marshalling status. Error: %v, Status: %v", err, h)
		body, _ := json.Marshal(map[string]string{"error": err.Error()})
		http.Error(w, string(body), 500)
		return
	}

	w.Write(jsonHealth)
}

func getCSRFToken(w http.ResponseWriter, r *http.Request) {
	w.Write([]byte(gui.CsrfToken))
}

func getConfigCheck(w http.ResponseWriter, r *http.Request) {
	var response response.ConfigCheckResponse

	configs := common.AC.GetLoadedConfigs()
	sort.Slice(configs, func(i, j int) bool {
		return configs[i].Name < configs[j].Name
	})
	response.Configs = configs
	response.ResolveWarnings = autodiscovery.GetResolveWarnings()
	response.ConfigErrors = autodiscovery.GetConfigErrors()
	response.Unresolved = common.AC.GetUnresolvedTemplates()

	jsonConfig, err := json.Marshal(response)
	if err != nil {
		log.Errorf("Unable to marshal config check response: %s", err)
		body, _ := json.Marshal(map[string]string{"error": err.Error()})
		http.Error(w, string(body), 500)
		return
	}

	w.Write(jsonConfig)
}

func getTaggerList(w http.ResponseWriter, r *http.Request) {
	response := tagger.List(tagger.IsFullCardinality())

	jsonTags, err := json.Marshal(response)
	if err != nil {
		log.Errorf("Unable to marshal tagger list response: %s", err)
		body, _ := json.Marshal(map[string]string{"error": err.Error()})
		http.Error(w, string(body), 500)
		return
	}
	w.Write(jsonTags)
}
