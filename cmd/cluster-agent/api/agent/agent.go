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
	"io"
	"net/http"

	"github.com/gorilla/mux"

	"github.com/DataDog/datadog-agent/cmd/agent/common/signals"
	"github.com/DataDog/datadog-agent/comp/core/autodiscovery"
	diagnose "github.com/DataDog/datadog-agent/comp/core/diagnose/def"
	"github.com/DataDog/datadog-agent/comp/core/settings"
	"github.com/DataDog/datadog-agent/comp/core/status"
	tagger "github.com/DataDog/datadog-agent/comp/core/tagger/def"
	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	dcametadata "github.com/DataDog/datadog-agent/comp/metadata/clusteragent/def"
	autoscalingWorkload "github.com/DataDog/datadog-agent/pkg/clusteragent/autoscaling/workload"
	pkgconfigsetup "github.com/DataDog/datadog-agent/pkg/config/setup"
	clusterAgentFlare "github.com/DataDog/datadog-agent/pkg/flare/clusteragent"
	"github.com/DataDog/datadog-agent/pkg/status/health"
	"github.com/DataDog/datadog-agent/pkg/util/defaultpaths"
	"github.com/DataDog/datadog-agent/pkg/util/hostname"
	httputils "github.com/DataDog/datadog-agent/pkg/util/http"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/datadog-agent/pkg/version"
)

// SetupHandlers adds the specific handlers for cluster agent endpoints
func SetupHandlers(r *mux.Router, wmeta workloadmeta.Component, ac autodiscovery.Component, statusComponent status.Component, settings settings.Component, taggerComp tagger.Component, diagnoseComponent diagnose.Component, dcametadataComp dcametadata.Component) {
	r.HandleFunc("/version", getVersion).Methods("GET")
	r.HandleFunc("/hostname", getHostname).Methods("GET")
	r.HandleFunc("/flare", func(w http.ResponseWriter, r *http.Request) {
		makeFlare(w, r, statusComponent, diagnoseComponent)
	}).Methods("POST")
	r.HandleFunc("/stop", stopAgent).Methods("POST")
	r.HandleFunc("/status", func(w http.ResponseWriter, r *http.Request) { getStatus(w, r, statusComponent) }).Methods("GET")
	r.HandleFunc("/status/health", getHealth).Methods("GET")
	r.HandleFunc("/config-check", func(w http.ResponseWriter, r *http.Request) {
		getConfigCheck(w, r, ac)
	}).Methods("GET")
	r.HandleFunc("/config", settings.GetFullConfig("")).Methods("GET")
	r.HandleFunc("/config/by-source", settings.GetFullConfigBySource()).Methods("GET")
	r.HandleFunc("/config/list-runtime", settings.ListConfigurable).Methods("GET")
	r.HandleFunc("/config/{setting}", settings.GetValue).Methods("GET")
	r.HandleFunc("/config/{setting}", settings.SetValue).Methods("POST")
	r.HandleFunc("/autoscaler-list", func(w http.ResponseWriter, r *http.Request) { getAutoscalerList(w, r) }).Methods("GET")
	r.HandleFunc("/tagger-list", func(w http.ResponseWriter, r *http.Request) { getTaggerList(w, r, taggerComp) }).Methods("GET")
	r.HandleFunc("/workload-list", func(w http.ResponseWriter, r *http.Request) {
		getWorkloadList(w, r, wmeta)
	}).Methods("GET")
	r.HandleFunc("/metadata/cluster-agent", dcametadataComp.WritePayloadAsJSON).Methods("GET")
}

func getStatus(w http.ResponseWriter, r *http.Request, statusComponent status.Component) {
	log.Info("Got a request for the status. Making status.")
	verbose := r.URL.Query().Get("verbose") == "true"
	format := r.URL.Query().Get("format")
	s, err := statusComponent.GetStatus(format, verbose)
	w.Header().Set("Content-Type", "application/json")
	if err != nil {
		log.Errorf("Error getting status. Error: %v, Status: %v", err, s)
		httputils.SetJSONError(w, err, 500)
		return
	}
	w.Write(s)
}

//nolint:revive // TODO(CINT) Fix revive linter
func getHealth(w http.ResponseWriter, _ *http.Request) {
	h := health.GetReady()

	if len(h.Unhealthy) > 0 {
		log.Debugf("Healthcheck failed on: %v", h.Unhealthy)
	}

	jsonHealth, err := json.Marshal(h)
	if err != nil {
		log.Errorf("Error marshalling status. Error: %v, Status: %v", err, h)
		httputils.SetJSONError(w, err, 500)
		return
	}

	w.Write(jsonHealth)
}

//nolint:revive // TODO(CINT) Fix revive linter
func stopAgent(w http.ResponseWriter, _ *http.Request) {
	signals.Stopper <- true
	w.Header().Set("Content-Type", "application/json")
	j, _ := json.Marshal("")
	w.Write(j)
}

//nolint:revive // TODO(CINT) Fix revive linter
func getVersion(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	av, err := version.Agent()
	if err != nil {
		httputils.SetJSONError(w, err, 500)
		return
	}
	j, err := json.Marshal(av)
	if err != nil {
		httputils.SetJSONError(w, err, 500)
		return
	}
	w.Write(j)
}

func getHostname(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	hname, err := hostname.Get(r.Context())
	if err != nil {
		log.Warnf("Error getting hostname: %s", err)
		hname = ""
	}
	j, err := json.Marshal(hname)
	if err != nil {
		httputils.SetJSONError(w, err, 500)
		return
	}
	w.Write(j)
}

func makeFlare(w http.ResponseWriter, r *http.Request, statusComponent status.Component, diagnoseComponent diagnose.Component) {
	log.Infof("Making a flare")
	w.Header().Set("Content-Type", "application/json")

	var profile clusterAgentFlare.ProfileData

	if r.Body != http.NoBody {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			http.Error(w, log.Errorf("Error while reading HTTP request body: %s", err).Error(), 500)
			return
		}

		if err := json.Unmarshal(body, &profile); err != nil {
			http.Error(w, log.Errorf("Error while unmarshaling JSON from request body: %s", err).Error(), 500)
			return
		}
	}

	logFile := pkgconfigsetup.Datadog().GetString("log_file")
	if logFile == "" {
		logFile = defaultpaths.DCALogFile
	}
	filePath, err := clusterAgentFlare.CreateDCAArchive(false, defaultpaths.GetDistPath(), logFile, profile, statusComponent, diagnoseComponent)
	if err != nil || filePath == "" {
		if err != nil {
			log.Errorf("The flare failed to be created: %s", err)
		} else {
			log.Warnf("The flare failed to be created")
		}
		httputils.SetJSONError(w, err, 500)
		return
	}
	w.Write([]byte(filePath))
}

//nolint:revive // TODO(CINT) Fix revive linter
func getConfigCheck(w http.ResponseWriter, _ *http.Request, ac autodiscovery.Component) {
	w.Header().Set("Content-Type", "application/json")

	configCheck := ac.GetConfigCheck()

	configCheckBytes, err := json.Marshal(configCheck)
	if err != nil {
		httputils.SetJSONError(w, log.Errorf("Unable to marshal config check response: %s", err), 500)
		return
	}

	w.Write(configCheckBytes)
}

func getAutoscalerList(w http.ResponseWriter, r *http.Request) {
	autoscalerList := autoscalingWorkload.Dump()
	if autoscalerList == nil {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	autoscalerListBytes, err := json.Marshal(*autoscalerList)
	if err != nil {
		log.Errorf("Unable to marshal autoscaler list response: %s", err)
		httputils.SetJSONError(w, log.Errorf("Unable to marshal autoscaler list response: %s", err), 500)
		return
	}

	w.Write(autoscalerListBytes)
}

//nolint:revive // TODO(CINT) Fix revive linter
func getTaggerList(w http.ResponseWriter, _ *http.Request, taggerComp tagger.Component) {
	response := taggerComp.List()

	jsonTags, err := json.Marshal(response)
	if err != nil {
		httputils.SetJSONError(w, log.Errorf("Unable to marshal tagger list response: %s", err), 500)
		return
	}
	w.Write(jsonTags)
}

func getWorkloadList(w http.ResponseWriter, r *http.Request, wmeta workloadmeta.Component) {
	verbose := false
	params := r.URL.Query()
	if v, ok := params["verbose"]; ok {
		if len(v) >= 1 && v[0] == "true" {
			verbose = true
		}
	}

	response := wmeta.Dump(verbose)
	jsonDump, err := json.Marshal(response)
	if err != nil {
		httputils.SetJSONError(w, log.Errorf("Unable to marshal workload list response: %v", err), 500)
		return
	}

	w.Write(jsonDump)
}
