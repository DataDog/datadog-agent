// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package agent implements the api endpoints for the `/agent` prefix.
// This group of endpoints is meant to provide high-level functionalities
// at the agent level.
package agent

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"time"

	"github.com/gorilla/mux"

	"github.com/DataDog/datadog-agent/cmd/agent/common"
	"github.com/DataDog/datadog-agent/cmd/agent/common/signals"

	"github.com/DataDog/datadog-agent/comp/api/api"
	"github.com/DataDog/datadog-agent/comp/api/api/utils"
	streamutils "github.com/DataDog/datadog-agent/comp/api/api/utils/stream"
	"github.com/DataDog/datadog-agent/comp/collector/collector"
	"github.com/DataDog/datadog-agent/comp/core/autodiscovery"
	"github.com/DataDog/datadog-agent/comp/core/gui"
	"github.com/DataDog/datadog-agent/comp/core/secrets"
	"github.com/DataDog/datadog-agent/comp/core/settings"
	"github.com/DataDog/datadog-agent/comp/core/status"

	"github.com/DataDog/datadog-agent/comp/core/workloadmeta"
	logsAgent "github.com/DataDog/datadog-agent/comp/logs/agent"
	"github.com/DataDog/datadog-agent/comp/metadata/host"
	"github.com/DataDog/datadog-agent/comp/metadata/inventoryagent"
	"github.com/DataDog/datadog-agent/comp/metadata/inventorychecks"
	"github.com/DataDog/datadog-agent/comp/metadata/inventoryhost"
	"github.com/DataDog/datadog-agent/comp/metadata/packagesigning"
	"github.com/DataDog/datadog-agent/pkg/aggregator/sender"
	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/diagnose"
	"github.com/DataDog/datadog-agent/pkg/diagnose/diagnosis"
	"github.com/DataDog/datadog-agent/pkg/gohai"
	"github.com/DataDog/datadog-agent/pkg/status/health"
	"github.com/DataDog/datadog-agent/pkg/util/hostname"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/datadog-agent/pkg/util/optional"
	"github.com/DataDog/datadog-agent/pkg/util/scrubber"
)

var mimeTypeMap = map[string]string{
	"text": "text/plain",
	"json": "application/json",
}

// SetupHandlers adds the specific handlers for /agent endpoints
func SetupHandlers(
	r *mux.Router,
	wmeta workloadmeta.Component,
	logsAgent optional.Option[logsAgent.Component],
	senderManager sender.DiagnoseSenderManager,
	hostMetadata host.Component,
	invAgent inventoryagent.Component,
	invHost inventoryhost.Component,
	secretResolver secrets.Component,
	invChecks inventorychecks.Component,
	pkgSigning packagesigning.Component,
	statusComponent status.Component,
	collector optional.Option[collector.Component],
	ac autodiscovery.Component,
	gui optional.Option[gui.Component],
	settings settings.Component,
	providers []api.EndpointProvider,
) *mux.Router {

	// Register the handlers from the component providers
	for _, p := range providers {
		r.HandleFunc(p.Route, p.HandlerFunc).Methods(p.Methods...)
	}

	// TODO: move these to a component that is registerable
	r.HandleFunc("/version", common.GetVersion).Methods("GET")
	r.HandleFunc("/hostname", getHostname).Methods("GET")
	r.HandleFunc("/stop", stopAgent).Methods("POST")
	r.HandleFunc("/status", func(w http.ResponseWriter, r *http.Request) {
		getStatus(w, r, statusComponent, "")
	}).Methods("GET")
	r.HandleFunc("/status/health", getHealth).Methods("GET")
	r.HandleFunc("/{component}/status", func(w http.ResponseWriter, r *http.Request) { componentStatusGetterHandler(w, r, statusComponent) }).Methods("GET")
	r.HandleFunc("/{component}/status", componentStatusHandler).Methods("POST")
	r.HandleFunc("/{component}/configs", componentConfigHandler).Methods("GET")
	r.HandleFunc("/gui/csrf-token", func(w http.ResponseWriter, _ *http.Request) { getCSRFToken(w, gui) }).Methods("GET")
	r.HandleFunc("/config", settings.GetFullConfig("")).Methods("GET")
	r.HandleFunc("/config/list-runtime", settings.ListConfigurable).Methods("GET")
	r.HandleFunc("/config/{setting}", func(w http.ResponseWriter, r *http.Request) {
		vars := mux.Vars(r)
		setting := vars["setting"]
		settings.GetValue(setting, w, r)
	}).Methods("GET")
	r.HandleFunc("/config/{setting}", func(w http.ResponseWriter, r *http.Request) {
		vars := mux.Vars(r)
		setting := vars["setting"]
		settings.SetValue(setting, w, r)
	}).Methods("POST")
	r.HandleFunc("/secrets", func(w http.ResponseWriter, r *http.Request) { secretInfo(w, r, secretResolver) }).Methods("GET")
	r.HandleFunc("/secret/refresh", func(w http.ResponseWriter, r *http.Request) { secretRefresh(w, r, secretResolver) }).Methods("GET")
	r.HandleFunc("/metadata/gohai", metadataPayloadGohai).Methods("GET")
	r.HandleFunc("/metadata/v5", func(w http.ResponseWriter, r *http.Request) { metadataPayloadV5(w, r, hostMetadata) }).Methods("GET")
	r.HandleFunc("/metadata/inventory-checks", func(w http.ResponseWriter, r *http.Request) { metadataPayloadInvChecks(w, r, invChecks) }).Methods("GET")
	r.HandleFunc("/metadata/inventory-agent", func(w http.ResponseWriter, r *http.Request) { metadataPayloadInvAgent(w, r, invAgent) }).Methods("GET")
	r.HandleFunc("/metadata/inventory-host", func(w http.ResponseWriter, r *http.Request) { metadataPayloadInvHost(w, r, invHost) }).Methods("GET")
	r.HandleFunc("/metadata/package-signing", func(w http.ResponseWriter, r *http.Request) { metadataPayloadPkgSigning(w, r, pkgSigning) }).Methods("GET")
	r.HandleFunc("/diagnose", func(w http.ResponseWriter, r *http.Request) {
		diagnoseDeps := diagnose.NewSuitesDeps(senderManager, collector, secretResolver, optional.NewOption(wmeta), optional.NewOption[autodiscovery.Component](ac))
		getDiagnose(w, r, diagnoseDeps)
	}).Methods("POST")

	if logsAgent, ok := logsAgent.Get(); ok {
		r.HandleFunc("/stream-logs", streamLogs(logsAgent)).Methods("POST")
	}

	return r
}

func stopAgent(w http.ResponseWriter, _ *http.Request) {
	signals.Stopper <- true
	w.Header().Set("Content-Type", "application/json")
	j, _ := json.Marshal("")
	w.Write(j)
}

func getHostname(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	hname, err := hostname.Get(r.Context())
	if err != nil {
		log.Warnf("Error getting hostname: %s\n", err) // or something like this
		hname = ""
	}
	j, _ := json.Marshal(hname)
	w.Write(j)
}

func componentConfigHandler(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	component := vars["component"]
	switch component {
	case "jmx":
		getJMXConfigs(w, r)
	default:
		http.Error(w, log.Errorf("bad url or resource does not exist").Error(), 404)
	}
}

func componentStatusGetterHandler(w http.ResponseWriter, r *http.Request, status status.Component) {
	vars := mux.Vars(r)
	component := vars["component"]
	switch component {
	case "py":
		getPythonStatus(w, r)
	default:
		getStatus(w, r, status, component)
	}
}

func componentStatusHandler(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	component := vars["component"]
	switch component {
	case "jmx":
		setJMXStatus(w, r)
	default:
		http.Error(w, log.Errorf("bad url or resource does not exist").Error(), 404)
	}
}

func getStatus(w http.ResponseWriter, r *http.Request, statusComponent status.Component, section string) {
	log.Info("Got a request for the status. Making status.")
	verbose := r.URL.Query().Get("verbose") == "true"
	format := r.URL.Query().Get("format")
	var contentType string
	var s []byte

	contentType, ok := mimeTypeMap[format]

	if !ok {
		log.Warn("Got a request with invalid format parameter. Defaulting to 'text' format")
		format = "text"
		contentType = mimeTypeMap[format]
	}
	w.Header().Set("Content-Type", contentType)

	var err error
	if len(section) > 0 {
		s, err = statusComponent.GetStatusBySections([]string{section}, format, verbose)
	} else {
		s, err = statusComponent.GetStatus(format, verbose)
	}

	if err != nil {
		if format == "text" {
			http.Error(w, log.Errorf("Error getting status. Error: %v.", err).Error(), 500)
			return
		}

		utils.SetJSONError(w, log.Errorf("Error getting status. Error: %v, Status: %v", err, s), 500)
		return
	}

	w.Write(s)
}

// TODO: logsAgent is a module so have to make the api component a module too
func streamLogs(logsAgent logsAgent.Component) func(w http.ResponseWriter, r *http.Request) {
	return streamutils.GetStreamFunc(func() streamutils.MessageReceiver { return logsAgent.GetMessageReceiver() }, "logs", "logs agent")
}

func getHealth(w http.ResponseWriter, _ *http.Request) {
	h := health.GetReady()

	if len(h.Unhealthy) > 0 {
		log.Debugf("Healthcheck failed on: %v", h.Unhealthy)
	}

	jsonHealth, err := json.Marshal(h)
	if err != nil {
		log.Errorf("Error marshalling status. Error: %v, Status: %v", err, h)
		utils.SetJSONError(w, err, 500)
		return
	}

	w.Write(jsonHealth)
}

func getCSRFToken(w http.ResponseWriter, optGui optional.Option[gui.Component]) {
	// WARNING: GUI comp currently not provided to JMX
	gui, guiExist := optGui.Get()
	if !guiExist {
		return
	}
	w.Write([]byte(gui.GetCSRFToken()))
}

func secretInfo(w http.ResponseWriter, _ *http.Request, secretResolver secrets.Component) {
	secretResolver.GetDebugInfo(w)
}

func secretRefresh(w http.ResponseWriter, _ *http.Request, secretResolver secrets.Component) {
	result, err := secretResolver.Refresh()
	if err != nil {
		utils.SetJSONError(w, err, 500)
		return
	}
	w.Write([]byte(result))
}

func metadataPayloadV5(w http.ResponseWriter, _ *http.Request, hostMetadataComp host.Component) {
	jsonPayload, err := hostMetadataComp.GetPayloadAsJSON(context.Background())
	if err != nil {
		utils.SetJSONError(w, log.Errorf("Unable to marshal v5 metadata payload: %s", err), 500)
		return
	}

	scrubbed, err := scrubber.ScrubBytes(jsonPayload)
	if err != nil {
		utils.SetJSONError(w, log.Errorf("Unable to scrub metadata payload: %s", err), 500)
		return
	}
	w.Write(scrubbed)
}

func metadataPayloadGohai(w http.ResponseWriter, _ *http.Request) {
	payload := gohai.GetPayloadWithProcesses(config.IsContainerized())
	jsonPayload, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		utils.SetJSONError(w, log.Errorf("Unable to marshal gohai metadata payload: %s", err), 500)
		return
	}

	scrubbed, err := scrubber.ScrubBytes(jsonPayload)
	if err != nil {
		utils.SetJSONError(w, log.Errorf("Unable to scrub gohai metadata payload: %s", err), 500)
		return
	}
	w.Write(scrubbed)
}

func metadataPayloadInvChecks(w http.ResponseWriter, _ *http.Request, invChecks inventorychecks.Component) {
	// GetAsJSON already return scrubbed data
	scrubbed, err := invChecks.GetAsJSON()
	if err != nil {
		utils.SetJSONError(w, err, 500)
		return
	}
	w.Write(scrubbed)
}

func metadataPayloadInvAgent(w http.ResponseWriter, _ *http.Request, invAgent inventoryagent.Component) {
	// GetAsJSON already return scrubbed data
	scrubbed, err := invAgent.GetAsJSON()
	if err != nil {
		utils.SetJSONError(w, err, 500)
		return
	}
	w.Write(scrubbed)
}

func metadataPayloadInvHost(w http.ResponseWriter, _ *http.Request, invHost inventoryhost.Component) {
	// GetAsJSON already return scrubbed data
	scrubbed, err := invHost.GetAsJSON()
	if err != nil {
		utils.SetJSONError(w, err, 500)
		return
	}
	w.Write(scrubbed)
}

func metadataPayloadPkgSigning(w http.ResponseWriter, _ *http.Request, pkgSigning packagesigning.Component) {
	// GetAsJSON already return scrubbed data
	scrubbed, err := pkgSigning.GetAsJSON()
	if err != nil {
		utils.SetJSONError(w, err, 500)
		return
	}
	w.Write(scrubbed)
}

func getDiagnose(w http.ResponseWriter, r *http.Request, diagnoseDeps diagnose.SuitesDeps) {
	var diagCfg diagnosis.Config

	// Read parameters
	if r.Body != http.NoBody {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			http.Error(w, log.Errorf("Error while reading HTTP request body: %s", err).Error(), 500)
			return
		}

		if err := json.Unmarshal(body, &diagCfg); err != nil {
			http.Error(w, log.Errorf("Error while unmarshaling JSON from request body: %s", err).Error(), 500)
			return
		}
	}

	// Reset the `server_timeout` deadline for this connection as running diagnose code in Agent process can take some time
	conn := utils.GetConnection(r)
	_ = conn.SetDeadline(time.Time{})

	// Indicate that we are already running in Agent process (and flip RunLocal)
	diagCfg.RunLocal = true

	var diagnoses []diagnosis.Diagnoses
	var err error

	// Get diagnoses via API
	// TODO: Once API component will be refactored, clean these dependencies
	collector, ok := diagnoseDeps.Collector.Get()
	if ok {
		diagnoses, err = diagnose.RunInAgentProcess(diagCfg, diagnose.NewSuitesDepsInAgentProcess(collector))
	} else {
		ac, ok := diagnoseDeps.AC.Get()
		if ok {
			diagnoses, err = diagnose.RunInCLIProcess(diagCfg, diagnose.NewSuitesDepsInCLIProcess(diagnoseDeps.SenderManager, diagnoseDeps.SecretResolver, diagnoseDeps.WMeta, ac))
		} else {
			err = errors.New("collector or autoDiscovery not found")
		}
	}
	if err != nil {
		utils.SetJSONError(w, log.Errorf("Running diagnose in Agent process failed: %s", err), 500)
		return
	}

	// Serizalize diagnoses (and implicitly write result to the response)
	w.Header().Set("Content-Type", "application/json")
	err = json.NewEncoder(w).Encode(diagnoses)
	if err != nil {
		utils.SetJSONError(w, log.Errorf("Unable to marshal config check response: %s", err), 500)
	}
}
