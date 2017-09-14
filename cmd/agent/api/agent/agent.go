// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2017 Datadog, Inc.

// Package agent implements the api endpoints for the `/agent` prefix.
// This group of endpoints is meant to provide high-level functionalities
// at the agent level.
package agent

import (
	"encoding/json"
	"net/http"

	log "github.com/cihub/seelog"

	apicommon "github.com/DataDog/datadog-agent/cmd/agent/api/common"
	"github.com/DataDog/datadog-agent/cmd/agent/common"
	"github.com/DataDog/datadog-agent/pkg/flare"
	"github.com/DataDog/datadog-agent/pkg/status"
	"github.com/DataDog/datadog-agent/pkg/util"
	"github.com/DataDog/datadog-agent/pkg/version"
	"github.com/gorilla/mux"
)

// SetupHandlers adds the specific handlers for /agent endpoints
func SetupHandlers(r *mux.Router) {
	r.HandleFunc("/version", getVersion).Methods("GET")
	r.HandleFunc("/hostname", getHostname).Methods("GET")
	r.HandleFunc("/flare", makeFlare).Methods("POST")
	r.HandleFunc("/jmxstatus", setJMXStatus).Methods("POST")
	r.HandleFunc("/jmxconfigs", getJMXConfigs).Methods("GET")
	r.HandleFunc("/status", getStatus).Methods("GET")
	r.HandleFunc("/status/formatted", getFormattedStatus).Methods("GET")
}

func getVersion(w http.ResponseWriter, r *http.Request) {
	if err := apicommon.Validate(w, r); err != nil {
		return
	}
	w.Header().Set("Content-Type", "application/json")
	av, _ := version.New(version.AgentVersion)
	j, _ := json.Marshal(av)
	w.Write(j)
}

func getHostname(w http.ResponseWriter, r *http.Request) {
	if err := apicommon.Validate(w, r); err != nil {
		return
	}
	w.Header().Set("Content-Type", "application/json")
	hname, err := util.GetHostname()
	if err != nil {
		log.Warnf("Error getting hostname: %s\n", err) // or something like this
		hname = ""
	}
	j, _ := json.Marshal(hname)
	w.Write(j)
}

func makeFlare(w http.ResponseWriter, r *http.Request) {
	if err := apicommon.Validate(w, r); err != nil {
		return
	}

	log.Infof("Making a flare")
	filePath, err := flare.CreateArchive(false, common.GetDistPath(), common.PyChecksPath)
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

func getJMXConfigs(w http.ResponseWriter, r *http.Request) {
	if err := apicommon.Validate(w, r); err != nil {
		return
	}

	w.Header().Set("Content-Type", "application/json")
	decoder := json.NewDecoder(r.Body)

	var tsjson map[string]interface{}
	err := decoder.Decode(&tsjson)
	if err != nil {
		log.Errorf("unable to parse jmx status: %s", err)
		http.Error(w, err.Error(), 500)
	}

	log.Debugf("Getting latest JMX Configs as of: %v", tsjson["timestamp"])
	// stub for now...
	j, _ := json.Marshal(map[string]interface{}{
		"configurations": map[string]interface{}{}})
	w.Write(j)
}

func setJMXStatus(w http.ResponseWriter, r *http.Request) {
	if err := apicommon.Validate(w, r); err != nil {
		return
	}

	decoder := json.NewDecoder(r.Body)

	var jmxStatus status.JMXStatus
	err := decoder.Decode(&jmxStatus)
	if err != nil {
		log.Errorf("unable to parse jmx status: %s", err)
		http.Error(w, err.Error(), 500)
	}

	status.SetJMXStatus(jmxStatus)
}

func getStatus(w http.ResponseWriter, r *http.Request) {
	if err := apicommon.Validate(w, r); err != nil {
		return
	}

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
	if err := apicommon.Validate(w, r); err != nil {
		return
	}

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
