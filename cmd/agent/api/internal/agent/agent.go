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
	"fmt"
	"io"
	"net"
	"net/http"
	"sort"
	"time"

	"github.com/gorilla/mux"

	"github.com/DataDog/datadog-agent/cmd/agent/api/response"
	"github.com/DataDog/datadog-agent/cmd/agent/common"
	"github.com/DataDog/datadog-agent/cmd/agent/common/signals"
	"github.com/DataDog/datadog-agent/cmd/agent/gui"
	"github.com/DataDog/datadog-agent/comp/core/flare"
	dogstatsdServer "github.com/DataDog/datadog-agent/comp/dogstatsd/server"
	dogstatsdDebug "github.com/DataDog/datadog-agent/comp/dogstatsd/serverDebug"
	"github.com/DataDog/datadog-agent/pkg/autodiscovery"
	"github.com/DataDog/datadog-agent/pkg/config"
	settingshttp "github.com/DataDog/datadog-agent/pkg/config/settings/http"
	pkgflare "github.com/DataDog/datadog-agent/pkg/flare"
	"github.com/DataDog/datadog-agent/pkg/logs"
	"github.com/DataDog/datadog-agent/pkg/logs/diagnostic"
	"github.com/DataDog/datadog-agent/pkg/metadata/inventories"
	v5 "github.com/DataDog/datadog-agent/pkg/metadata/v5"
	"github.com/DataDog/datadog-agent/pkg/secrets"
	"github.com/DataDog/datadog-agent/pkg/status"
	"github.com/DataDog/datadog-agent/pkg/status/health"
	"github.com/DataDog/datadog-agent/pkg/tagger"
	"github.com/DataDog/datadog-agent/pkg/tagger/collectors"
	"github.com/DataDog/datadog-agent/pkg/util/grpc"
	"github.com/DataDog/datadog-agent/pkg/util/hostname"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/datadog-agent/pkg/util/scrubber"
	"github.com/DataDog/datadog-agent/pkg/workloadmeta"
)

// SetupHandlers adds the specific handlers for /agent endpoints
func SetupHandlers(r *mux.Router, flare flare.Component, server dogstatsdServer.Component, serverDebug dogstatsdDebug.Component) *mux.Router {
	r.HandleFunc("/version", common.GetVersion).Methods("GET")
	r.HandleFunc("/hostname", getHostname).Methods("GET")
	r.HandleFunc("/flare", func(w http.ResponseWriter, r *http.Request) { makeFlare(w, r, flare) }).Methods("POST")
	r.HandleFunc("/stop", stopAgent).Methods("POST")
	r.HandleFunc("/status", getStatus).Methods("GET")
	r.HandleFunc("/stream-logs", streamLogs).Methods("POST")
	r.HandleFunc("/status/formatted", getFormattedStatus).Methods("GET")
	r.HandleFunc("/status/health", getHealth).Methods("GET")
	r.HandleFunc("/{component}/status", componentStatusGetterHandler).Methods("GET")
	r.HandleFunc("/{component}/status", componentStatusHandler).Methods("POST")
	r.HandleFunc("/{component}/configs", componentConfigHandler).Methods("GET")
	r.HandleFunc("/gui/csrf-token", getCSRFToken).Methods("GET")
	r.HandleFunc("/config-check", getConfigCheck).Methods("GET")
	r.HandleFunc("/config", settingshttp.Server.GetFullDatadogConfig("")).Methods("GET")
	r.HandleFunc("/config/list-runtime", settingshttp.Server.ListConfigurable).Methods("GET")
	r.HandleFunc("/config/{setting}", settingshttp.Server.GetValue).Methods("GET")
	r.HandleFunc("/config/{setting}", settingshttp.Server.SetValue).Methods("POST")
	r.HandleFunc("/tagger-list", getTaggerList).Methods("GET")
	r.HandleFunc("/workload-list", getWorkloadList).Methods("GET")
	r.HandleFunc("/secrets", secretInfo).Methods("GET")
	r.HandleFunc("/metadata/{payload}", metadataPayload).Methods("GET")

	// Some agent subcommands do not provide these dependencies (such as JMX)
	if server != nil && serverDebug != nil {
		r.HandleFunc("/dogstatsd-stats", func(w http.ResponseWriter, r *http.Request) { getDogstatsdStats(w, r, server, serverDebug) }).Methods("GET")
	}

	return r
}

func setJSONError(w http.ResponseWriter, err error, errorCode int) {
	w.Header().Set("Content-Type", "application/json")
	body, _ := json.Marshal(map[string]string{"error": err.Error()})
	http.Error(w, string(body), errorCode)
}

func stopAgent(w http.ResponseWriter, r *http.Request) {
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

func makeFlare(w http.ResponseWriter, r *http.Request, flare flare.Component) {
	var profile pkgflare.ProfileData

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

	// Reset the `server_timeout` deadline for this connection as creating a flare can take some time
	conn := GetConnection(r)
	_ = conn.SetDeadline(time.Time{})

	logFile := config.Datadog.GetString("log_file")
	if logFile == "" {
		logFile = common.DefaultLogFile
	}
	jmxLogFile := config.Datadog.GetString("jmx_log_file")
	if jmxLogFile == "" {
		jmxLogFile = common.DefaultJmxLogFile
	}

	// If we're not in an FX app we fallback to pkgflare implementation. Once all app have been migrated to flare we
	// could remove this.
	var filePath string
	var err error
	log.Infof("Making a flare")
	if flare != nil {
		filePath, err = flare.Create(false, common.GetDistPath(), common.PyChecksPath, []string{logFile, jmxLogFile}, profile, nil)
	} else {
		filePath, err = pkgflare.CreateArchive(false, common.GetDistPath(), common.PyChecksPath, []string{logFile, jmxLogFile}, profile, nil)
	}

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
		http.Error(w, log.Errorf("bad url or resource does not exist").Error(), 404)
	}
}

func componentStatusGetterHandler(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	component := vars["component"]
	switch component {
	case "py":
		getPythonStatus(w, r)
	default:
		http.Error(w, log.Errorf("bad url or resource does not exist").Error(), 404)
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

func getStatus(w http.ResponseWriter, r *http.Request) {
	log.Info("Got a request for the status. Making status.")
	s, err := status.GetStatus()
	w.Header().Set("Content-Type", "application/json")
	if err != nil {
		setJSONError(w, log.Errorf("Error getting status. Error: %v, Status: %v", err, s), 500)
		return
	}

	jsonStats, err := json.Marshal(s)
	if err != nil {
		setJSONError(w, log.Errorf("Error marshalling status. Error: %v, Status: %v", err, s), 500)
		return
	}

	w.Write(jsonStats)
}

func streamLogs(w http.ResponseWriter, r *http.Request) {
	log.Info("Got a request for stream logs.")
	w.Header().Set("Transfer-Encoding", "chunked")

	logMessageReceiver := logs.GetMessageReceiver()

	flusher, ok := w.(http.Flusher)
	if !ok {
		log.Errorf("Expected a Flusher type, got: %v", w)
		return
	}

	if logMessageReceiver == nil {
		http.Error(w, "The logs agent is not running", 405)
		flusher.Flush()
		log.Info("Logs agent is not running - can't stream logs")
		return
	}

	if !logMessageReceiver.SetEnabled(true) {
		http.Error(w, "Another client is already streaming logs.", 405)
		flusher.Flush()
		log.Info("Logs are already streaming. Dropping connection.")
		return
	}
	defer logMessageReceiver.SetEnabled(false)

	var filters diagnostic.Filters

	if r.Body != http.NoBody {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			http.Error(w, log.Errorf("Error while reading HTTP request body: %s", err).Error(), 500)
			return
		}

		if err := json.Unmarshal(body, &filters); err != nil {
			http.Error(w, log.Errorf("Error while unmarshaling JSON from request body: %s", err).Error(), 500)
			return
		}
	}

	// Reset the `server_timeout` deadline for this connection as streaming holds the connection open.
	conn := GetConnection(r)
	_ = conn.SetDeadline(time.Time{})

	done := make(chan struct{})
	defer close(done)
	logChan := logMessageReceiver.Filter(&filters, done)
	flushTimer := time.NewTicker(time.Second)
	for {
		// Handlers for detecting a closed connection (from either the server or client)
		select {
		case <-w.(http.CloseNotifier).CloseNotify(): //nolint
			return
		case <-r.Context().Done():
			return
		case line := <-logChan:
			fmt.Fprint(w, line)
		case <-flushTimer.C:
			// The buffer will flush on its own most of the time, but when we run out of logs flush so the client is up to date.
			flusher.Flush()
		}
	}
}

func getDogstatsdStats(w http.ResponseWriter, r *http.Request, dogstatsdServer dogstatsdServer.Component, serverDebug dogstatsdDebug.Component) {
	log.Info("Got a request for the Dogstatsd stats.")

	if !config.Datadog.GetBool("use_dogstatsd") {
		w.Header().Set("Content-Type", "application/json")
		body, _ := json.Marshal(map[string]string{
			"error":      "Dogstatsd not enabled in the Agent configuration",
			"error_type": "no server",
		})
		w.WriteHeader(400)
		w.Write(body)
		return
	}

	if !config.Datadog.GetBool("dogstatsd_metrics_stats_enable") {
		w.Header().Set("Content-Type", "application/json")
		body, _ := json.Marshal(map[string]string{
			"error":      "Dogstatsd metrics stats not enabled in the Agent configuration",
			"error_type": "not enabled",
		})
		w.WriteHeader(400)
		w.Write(body)
		return
	}

	// Weird state that should not happen: dogstatsd is enabled
	// but the server has not been successfully initialized.
	// Return no data.
	if !dogstatsdServer.IsRunning() {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{}`))
		return
	}

	jsonStats, err := serverDebug.GetJSONDebugStats()
	if err != nil {
		setJSONError(w, log.Errorf("Error getting marshalled Dogstatsd stats: %s", err), 500)
		return
	}

	w.Write(jsonStats)
}

func getFormattedStatus(w http.ResponseWriter, r *http.Request) {
	log.Info("Got a request for the formatted status. Making formatted status.")
	s, err := status.GetAndFormatStatus()
	if err != nil {
		setJSONError(w, log.Errorf("Error getting status: %v %v", err, s), 500)
		return
	}

	w.Write(s)
}

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

func getCSRFToken(w http.ResponseWriter, r *http.Request) {
	w.Write([]byte(gui.CsrfToken))
}

func getConfigCheck(w http.ResponseWriter, r *http.Request) {
	var response response.ConfigCheckResponse

	if common.AC == nil {
		log.Errorf("Trying to use /config-check before the agent has been initialized.")
		setJSONError(w, fmt.Errorf("agent not initialized"), 503)
		return
	}

	configSlice := common.AC.LoadedConfigs()
	sort.Slice(configSlice, func(i, j int) bool {
		return configSlice[i].Name < configSlice[j].Name
	})
	response.Configs = configSlice
	response.ResolveWarnings = autodiscovery.GetResolveWarnings()
	response.ConfigErrors = autodiscovery.GetConfigErrors()
	response.Unresolved = common.AC.GetUnresolvedTemplates()

	jsonConfig, err := json.Marshal(response)
	if err != nil {
		setJSONError(w, log.Errorf("Unable to marshal config check response: %s", err), 500)
		return
	}

	w.Write(jsonConfig)
}

func getTaggerList(w http.ResponseWriter, r *http.Request) {
	// query at the highest cardinality between checks and dogstatsd cardinalities
	cardinality := collectors.TagCardinality(max(int(tagger.ChecksCardinality), int(tagger.DogstatsdCardinality)))
	response := tagger.List(cardinality)

	jsonTags, err := json.Marshal(response)
	if err != nil {
		setJSONError(w, log.Errorf("Unable to marshal tagger list response: %s", err), 500)
		return
	}
	w.Write(jsonTags)
}

func getWorkloadList(w http.ResponseWriter, r *http.Request) {
	verbose := false
	params := r.URL.Query()
	if v, ok := params["verbose"]; ok {
		if len(v) >= 1 && v[0] == "true" {
			verbose = true
		}
	}

	response := workloadmeta.GetGlobalStore().Dump(verbose)
	jsonDump, err := json.Marshal(response)
	if err != nil {
		setJSONError(w, log.Errorf("Unable to marshal workload list response: %v", err), 500)
		return
	}

	w.Write(jsonDump)
}

func secretInfo(w http.ResponseWriter, r *http.Request) {
	secrets.GetDebugInfo(w)
}

func metadataPayload(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	payloadType := vars["payload"]

	var scrubbed []byte
	var err error

	switch payloadType {
	case "v5":
		ctx := context.Background()
		hostnameDetected, err := hostname.GetWithProvider(ctx)
		if err != nil {
			setJSONError(w, err, 500)
			return
		}

		payload := v5.GetPayload(ctx, hostnameDetected)
		jsonPayload, err := json.MarshalIndent(payload, "", "    ")
		if err != nil {
			setJSONError(w, log.Errorf("Unable to marshal v5 metadata payload: %s", err), 500)
			return
		}

		scrubbed, err = scrubber.ScrubBytes(jsonPayload)
		if err != nil {
			setJSONError(w, log.Errorf("Unable to scrub metadata payload: %s", err), 500)
			return
		}
	case "inventory":
		// GetLastPayload already return scrubbed data
		scrubbed, err = inventories.GetLastPayload()
		if err != nil {
			setJSONError(w, err, 500)
			return
		}
	default:
		setJSONError(w, log.Errorf("Unknown metadata payload requested: %s", payloadType), 500)
		return
	}

	w.Write(scrubbed)
}

// max returns the maximum value between a and b.
func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

// GetConnection returns the connection for the request
func GetConnection(r *http.Request) net.Conn {
	return r.Context().Value(grpc.ConnContextKey).(net.Conn)
}
