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
	"sort"

	"github.com/gorilla/mux"

	"github.com/DataDog/datadog-agent/cmd/agent/common/path"
	"github.com/DataDog/datadog-agent/cmd/agent/common/signals"
	"github.com/DataDog/datadog-agent/comp/api/api/apiimpl/response"
	"github.com/DataDog/datadog-agent/comp/collector/collector"
	"github.com/DataDog/datadog-agent/comp/core/autodiscovery"
	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/autodiscoveryimpl"
	flaretypes "github.com/DataDog/datadog-agent/comp/core/flare/types"
	"github.com/DataDog/datadog-agent/comp/core/secrets"
	"github.com/DataDog/datadog-agent/comp/core/status"
	"github.com/DataDog/datadog-agent/comp/core/tagger"
	"github.com/DataDog/datadog-agent/comp/core/tagger/collectors"
	"github.com/DataDog/datadog-agent/comp/core/workloadmeta"
	"github.com/DataDog/datadog-agent/pkg/aggregator/sender"
	"github.com/DataDog/datadog-agent/pkg/config"
	settingshttp "github.com/DataDog/datadog-agent/pkg/config/settings/http"
	"github.com/DataDog/datadog-agent/pkg/diagnose"
	"github.com/DataDog/datadog-agent/pkg/flare"
	"github.com/DataDog/datadog-agent/pkg/status/health"
	"github.com/DataDog/datadog-agent/pkg/util/hostname"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/datadog-agent/pkg/util/optional"
	"github.com/DataDog/datadog-agent/pkg/version"
)

// SetupHandlers adds the specific handlers for cluster agent endpoints
func SetupHandlers(r *mux.Router, wmeta workloadmeta.Component, ac autodiscovery.Component, senderManager sender.DiagnoseSenderManager, collector optional.Option[collector.Component], statusComponent status.Component, secretResolver secrets.Component) {
	r.HandleFunc("/version", getVersion).Methods("GET")
	r.HandleFunc("/hostname", getHostname).Methods("GET")
	r.HandleFunc("/flare", func(w http.ResponseWriter, r *http.Request) {
		makeFlare(w, r, senderManager, collector, secretResolver, wmeta, statusComponent, ac)
	}).Methods("POST")
	r.HandleFunc("/stop", stopAgent).Methods("POST")
	r.HandleFunc("/status", func(w http.ResponseWriter, r *http.Request) { getStatus(w, r, statusComponent) }).Methods("GET")
	r.HandleFunc("/status/health", getHealth).Methods("GET")
	r.HandleFunc("/config-check", func(w http.ResponseWriter, r *http.Request) {
		getConfigCheck(w, r, ac)
	}).Methods("GET")
	r.HandleFunc("/config", settingshttp.Server.GetFullDatadogConfig("")).Methods("GET")
	r.HandleFunc("/config/list-runtime", settingshttp.Server.ListConfigurable).Methods("GET")
	r.HandleFunc("/config/{setting}", settingshttp.Server.GetValue).Methods("GET")
	r.HandleFunc("/config/{setting}", settingshttp.Server.SetValue).Methods("POST")
	r.HandleFunc("/tagger-list", getTaggerList).Methods("GET")
	r.HandleFunc("/workload-list", func(w http.ResponseWriter, r *http.Request) {
		getWorkloadList(w, r, wmeta)
	}).Methods("GET")
}

func getStatus(w http.ResponseWriter, r *http.Request, statusComponent status.Component) {
	log.Info("Got a request for the status. Making status.")
	verbose := r.URL.Query().Get("verbose") == "true"
	format := r.URL.Query().Get("format")
	s, err := statusComponent.GetStatus(format, verbose)
	w.Header().Set("Content-Type", "application/json")
	if err != nil {
		log.Errorf("Error getting status. Error: %v, Status: %v", err, s)
		setJSONError(w, err, 500)
		return
	}
	w.Write(s)
}

//nolint:revive // TODO(CINT) Fix revive linter
func getHealth(w http.ResponseWriter, r *http.Request) {
	h := health.GetReady()

	if len(h.Unhealthy) > 0 {
		log.Debugf("Healthcheck failed on: %v", h.Unhealthy)
	}

	jsonHealth, err := json.Marshal(h)
	if err != nil {
		log.Errorf("Error marshalling status. Error: %v, Status: %v", err, h)
		setJSONError(w, err, 500)
		return
	}

	w.Write(jsonHealth)
}

//nolint:revive // TODO(CINT) Fix revive linter
func stopAgent(w http.ResponseWriter, r *http.Request) {
	signals.Stopper <- true
	w.Header().Set("Content-Type", "application/json")
	j, _ := json.Marshal("")
	w.Write(j)
}

//nolint:revive // TODO(CINT) Fix revive linter
func getVersion(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	av, err := version.Agent()
	if err != nil {
		setJSONError(w, err, 500)
		return
	}
	j, err := json.Marshal(av)
	if err != nil {
		setJSONError(w, err, 500)
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
		setJSONError(w, err, 500)
		return
	}
	w.Write(j)
}

func makeFlare(w http.ResponseWriter, r *http.Request, senderManager sender.DiagnoseSenderManager, collector optional.Option[collector.Component], secretResolver secrets.Component, wmeta workloadmeta.Component, statusComponent status.Component, ac autodiscovery.Component) {
	log.Infof("Making a flare")
	w.Header().Set("Content-Type", "application/json")

	var profile flaretypes.ProfileData

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

	logFile := config.Datadog.GetString("log_file")
	if logFile == "" {
		logFile = path.DefaultDCALogFile
	}
	diagnoseDeps := diagnose.NewSuitesDeps(senderManager, collector, secretResolver, optional.NewOption(wmeta), optional.NewOption[autodiscovery.Component](ac))
	filePath, err := flare.CreateDCAArchive(false, path.GetDistPath(), logFile, profile, diagnoseDeps, statusComponent)
	if err != nil || filePath == "" {
		if err != nil {
			log.Errorf("The flare failed to be created: %s", err)
		} else {
			log.Warnf("The flare failed to be created")
		}
		setJSONError(w, err, 500)
		return
	}
	w.Write([]byte(filePath))
}

//nolint:revive // TODO(CINT) Fix revive linter
func getConfigCheck(w http.ResponseWriter, r *http.Request, ac autodiscovery.Component) {
	var response response.ConfigCheckResponse

	configSlice := ac.LoadedConfigs()
	sort.Slice(configSlice, func(i, j int) bool {
		return configSlice[i].Name < configSlice[j].Name
	})
	response.Configs = configSlice
	response.ResolveWarnings = autodiscoveryimpl.GetResolveWarnings()
	response.ConfigErrors = autodiscoveryimpl.GetConfigErrors()
	response.Unresolved = ac.GetUnresolvedTemplates()

	jsonConfig, err := json.Marshal(response)
	if err != nil {
		log.Errorf("Unable to marshal config check response: %s", err)
		setJSONError(w, err, 500)
		return
	}

	w.Write(jsonConfig)
}

//nolint:revive // TODO(CINT) Fix revive linter
func getTaggerList(w http.ResponseWriter, r *http.Request) {
	response := tagger.List(collectors.HighCardinality)

	jsonTags, err := json.Marshal(response)
	if err != nil {
		setJSONError(w, log.Errorf("Unable to marshal tagger list response: %s", err), 500)
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
		setJSONError(w, log.Errorf("Unable to marshal workload list response: %v", err), 500)
		return
	}

	w.Write(jsonDump)
}

func setJSONError(w http.ResponseWriter, err error, errorCode int) {
	w.Header().Set("Content-Type", "application/json")
	body, _ := json.Marshal(map[string]string{"error": err.Error()})
	http.Error(w, string(body), errorCode)
}
