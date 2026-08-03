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

	"github.com/DataDog/datadog-agent/cmd/agent/common/signals"
	autodiscovery "github.com/DataDog/datadog-agent/comp/core/autodiscovery/def"
	diagnose "github.com/DataDog/datadog-agent/comp/core/diagnose/def"
	flaretypes "github.com/DataDog/datadog-agent/comp/core/flare/types"
	ipc "github.com/DataDog/datadog-agent/comp/core/ipc/def"
	settings "github.com/DataDog/datadog-agent/comp/core/settings/def"
	"github.com/DataDog/datadog-agent/comp/core/status"
	tagger "github.com/DataDog/datadog-agent/comp/core/tagger/def"
	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	dcametadata "github.com/DataDog/datadog-agent/comp/metadata/clusteragent/def"
	clusterchecksmetadata "github.com/DataDog/datadog-agent/comp/metadata/clusterchecks/def"

	"github.com/DataDog/datadog-agent/pkg/api/coverage"
	autoscalingWorkload "github.com/DataDog/datadog-agent/pkg/clusteragent/autoscaling/workload"
	localautoscalingworkload "github.com/DataDog/datadog-agent/pkg/clusteragent/autoscaling/workload/loadstore"
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
func SetupHandlers(r *http.ServeMux, wmeta workloadmeta.Component, ac autodiscovery.Component, statusComponent status.Component, settings settings.Component, taggerComp tagger.Component, diagnoseComponent diagnose.Component, dcametadataComp dcametadata.Component, clusterChecksMetadataComp clusterchecksmetadata.Component, ipc ipc.Component) {
	r.HandleFunc("GET /version", getVersion)
	r.HandleFunc("GET /hostname", getHostname)
	r.HandleFunc("POST /flare", func(w http.ResponseWriter, r *http.Request) {
		makeFlare(w, r, statusComponent, diagnoseComponent, ipc)
	})
	r.HandleFunc("POST /stop", stopAgent)
	r.HandleFunc("GET /status", func(w http.ResponseWriter, r *http.Request) { getStatus(w, r, statusComponent) })
	r.HandleFunc("GET /status/section/{component}", func(w http.ResponseWriter, r *http.Request) { getStatusSection(w, r, statusComponent) })
	r.HandleFunc("GET /status/sections", func(w http.ResponseWriter, r *http.Request) { getStatusSections(w, r, statusComponent) })
	r.HandleFunc("GET /status/health", getHealth)
	r.HandleFunc("GET /config-check", func(w http.ResponseWriter, r *http.Request) {
		getConfigCheck(w, r, ac)
	})
	r.HandleFunc("GET /config", settings.GetFullConfig(""))
	r.HandleFunc("GET /config/without-defaults", settings.GetFullConfigWithoutDefaults(""))
	r.HandleFunc("GET /config/by-source", settings.GetFullConfigBySource())
	r.HandleFunc("GET /config/list-runtime", settings.ListConfigurable)
	r.HandleFunc("GET /config/{setting}", settings.GetValue)
	r.HandleFunc("POST /config/{setting}", settings.SetValue)
	r.HandleFunc("GET /autoscaler-list", func(w http.ResponseWriter, r *http.Request) { getAutoscalerList(w, r) })
	r.HandleFunc("GET /local-autoscaling-check", func(w http.ResponseWriter, r *http.Request) { getLocalAutoscalingWorkloadCheck(w, r) })
	r.HandleFunc("GET /tagger-list", func(w http.ResponseWriter, r *http.Request) { getTaggerList(w, r, taggerComp) })
	r.HandleFunc("GET /workload-list", func(w http.ResponseWriter, r *http.Request) {
		getWorkloadList(w, r, wmeta)
	})
	r.HandleFunc("GET /metadata/cluster-agent", dcametadataComp.WritePayloadAsJSON)
	r.HandleFunc("GET /metadata/cluster-checks", clusterChecksMetadataComp.WritePayloadAsJSON)

	// Special handler to compute running agent Code coverage
	coverage.SetupCoverageHandler(r)
}

func getStatus(w http.ResponseWriter, r *http.Request, statusComponent status.Component) {
	writeStatus(w, r, statusComponent, "")
}

func getStatusSection(w http.ResponseWriter, r *http.Request, statusComponent status.Component) {
	writeStatus(w, r, statusComponent, r.PathValue("component"))
}

func writeStatus(w http.ResponseWriter, r *http.Request, statusComponent status.Component, section string) {
	log.Info("Got a request for the status. Making status.")
	verbose := r.URL.Query().Get("verbose") == "true"
	format := r.URL.Query().Get("format")
	var s []byte
	var err error
	if section == "" {
		s, err = statusComponent.GetStatus(format, verbose)
	} else {
		s, err = statusComponent.GetStatusBySections([]string{section}, format, verbose)
	}
	w.Header().Set("Content-Type", "application/json")
	if err != nil {
		log.Errorf("Error getting status. Error: %v, Status: %v", err, s)
		httputils.SetJSONError(w, err, 500)
		return
	}
	w.Write(s)
}

func getStatusSections(w http.ResponseWriter, _ *http.Request, statusComponent status.Component) {
	log.Info("Got a request for the status sections.")
	w.Header().Set("Content-Type", "application/json")
	sections, _ := json.Marshal(statusComponent.GetSections())
	_, _ = w.Write(sections)
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

func makeFlare(w http.ResponseWriter, r *http.Request, statusComponent status.Component, diagnoseComponent diagnose.Component, ipc ipc.Component) {
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
		logFile = defaultpaths.GetDefaultDCALogFile()
	}
	filePath, err := clusterAgentFlare.CreateDCAArchive(false, defaultpaths.GetDistPath(), logFile, profile, flaretypes.FlareArgs{}, statusComponent, diagnoseComponent, ipc)
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

func getAutoscalerList(w http.ResponseWriter, _ *http.Request) {
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
	params := r.URL.Query()

	jsonDump, err := workloadmeta.BuildWorkloadResponse(
		wmeta,
		params.Get("verbose") == "true",
		params.Get("search"),
		params.Get("format") == "json",
	)
	if err != nil {
		httputils.SetJSONError(w, log.Errorf("Unable to build workload list response: %v", err), 500)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.Write(jsonDump)
}

func getLocalAutoscalingWorkloadCheck(w http.ResponseWriter, r *http.Request) {
	response := localautoscalingworkload.GetAutoscalingWorkloadCheck(r.Context())
	if response == nil {
		log.Debugf("No local autoscaling entities found")
		w.WriteHeader(http.StatusNoContent)
		return
	}
	jsonResponse, err := json.Marshal(response)
	if err != nil {
		httputils.SetJSONError(w, log.Errorf("Unable to marshal autoscaling check response: %v", err), 500)
		return
	}
	w.Write(jsonResponse)
}
