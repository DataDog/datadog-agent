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
	"fmt"
	"io/ioutil"
	"net"
	"net/http"
	"sort"
	"time"

	"github.com/gorilla/mux"

	"github.com/DataDog/datadog-agent/cmd/agent/api/response"
	"github.com/DataDog/datadog-agent/cmd/agent/common"
	"github.com/DataDog/datadog-agent/cmd/agent/common/signals"
	"github.com/DataDog/datadog-agent/cmd/agent/gui"
	"github.com/DataDog/datadog-agent/pkg/autodiscovery"
	"github.com/DataDog/datadog-agent/pkg/autodiscovery/integration"
	"github.com/DataDog/datadog-agent/pkg/config"
	settingshttp "github.com/DataDog/datadog-agent/pkg/config/settings/http"
	"github.com/DataDog/datadog-agent/pkg/flare"
	"github.com/DataDog/datadog-agent/pkg/logs"
	"github.com/DataDog/datadog-agent/pkg/logs/diagnostic"
	"github.com/DataDog/datadog-agent/pkg/secrets"
	"github.com/DataDog/datadog-agent/pkg/status"
	"github.com/DataDog/datadog-agent/pkg/status/health"
	"github.com/DataDog/datadog-agent/pkg/tagger"
	"github.com/DataDog/datadog-agent/pkg/tagger/collectors"
	"github.com/DataDog/datadog-agent/pkg/util"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

type contextKey struct {
	key string
}

// ConnContextKey key to reference the http connection from the request context
var ConnContextKey = &contextKey{"http-connection"}

// SetupHandlers adds the specific handlers for /agent endpoints
func SetupHandlers(r *mux.Router) *mux.Router {
	r.HandleFunc("/version", common.GetVersion).Methods("GET")
	r.HandleFunc("/hostname", getHostname).Methods("GET")
	r.HandleFunc("/flare", makeFlare).Methods("POST")
	r.HandleFunc("/stop", stopAgent).Methods("POST")
	r.HandleFunc("/status", getStatus).Methods("GET")
	r.HandleFunc("/stream-logs", streamLogs).Methods("POST")
	r.HandleFunc("/dogstatsd-stats", getDogstatsdStats).Methods("GET")
	r.HandleFunc("/status/formatted", getFormattedStatus).Methods("GET")
	r.HandleFunc("/status/health", getHealth).Methods("GET")
	r.HandleFunc("/{component}/status", componentStatusGetterHandler).Methods("GET")
	r.HandleFunc("/{component}/status", componentStatusHandler).Methods("POST")
	r.HandleFunc("/{component}/configs", componentConfigHandler).Methods("GET")
	r.HandleFunc("/gui/csrf-token", getCSRFToken).Methods("GET")
	r.HandleFunc("/config-check", getConfigCheck).Methods("GET")
	r.HandleFunc("/config", settingshttp.Server.GetFull("")).Methods("GET")
	r.HandleFunc("/config/list-runtime", settingshttp.Server.ListConfigurable).Methods("GET")
	r.HandleFunc("/config/{setting}", settingshttp.Server.GetValue).Methods("GET")
	r.HandleFunc("/config/{setting}", settingshttp.Server.SetValue).Methods("POST")
	r.HandleFunc("/tagger-list", getTaggerList).Methods("GET")
	r.HandleFunc("/secrets", secretInfo).Methods("GET")

	return r
}

func stopAgent(w http.ResponseWriter, r *http.Request) {
	signals.Stopper <- true
	w.Header().Set("Content-Type", "application/json")
	j, _ := json.Marshal("")
	w.Write(j)
}

func getHostname(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	hname, err := util.GetHostname(r.Context())
	if err != nil {
		log.Warnf("Error getting hostname: %s\n", err) // or something like this
		hname = ""
	}
	j, _ := json.Marshal(hname)
	w.Write(j)
}

func makeFlare(w http.ResponseWriter, r *http.Request) {
	var profile flare.ProfileData

	if r.Body != http.NoBody {
		body, err := ioutil.ReadAll(r.Body)
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
		logFile = common.DefaultLogFile
	}
	jmxLogFile := config.Datadog.GetString("jmx_log_file")
	if jmxLogFile == "" {
		jmxLogFile = common.DefaultJmxLogFile
	}
	log.Infof("Making a flare")
	filePath, err := flare.CreateArchive(false, common.GetDistPath(), common.PyChecksPath, []string{logFile, jmxLogFile}, profile)
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
		body, err := ioutil.ReadAll(r.Body)
		if err != nil {
			http.Error(w, log.Errorf("Error while reading HTTP request body: %s", err).Error(), 500)
			return
		}

		if err := json.Unmarshal(body, &filters); err != nil {
			http.Error(w, log.Errorf("Error while unmarshaling JSON from request body: %s", err).Error(), 500)
			return
		}
	}

	conn := GetConnection(r)

	// Override the default server timeouts so the connection never times out
	_ = conn.SetDeadline(time.Time{})
	_ = conn.SetWriteDeadline(time.Time{})

	done := make(chan struct{})
	defer close(done)
	logChan := logMessageReceiver.Filter(&filters, done)
	flushTimer := time.NewTicker(time.Second)
	for {
		// Handlers for detecting a closed connection (from either the server or client)
		select {
		case <-w.(http.CloseNotifier).CloseNotify():
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

func getDogstatsdStats(w http.ResponseWriter, r *http.Request) {
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
	if common.DSD == nil {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{}`))
		return
	}

	jsonStats, err := common.DSD.GetJSONDebugStats()
	if err != nil {
		log.Errorf("Error getting marshalled Dogstatsd stats: %s", err)
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

func getCSRFToken(w http.ResponseWriter, r *http.Request) {
	w.Write([]byte(gui.CsrfToken))
}

func getConfigCheck(w http.ResponseWriter, r *http.Request) {
	var response response.ConfigCheckResponse

	if common.AC == nil {
		log.Errorf("Trying to use /config-check before the agent has been initialized.")
		body, _ := json.Marshal(map[string]string{"error": "agent not initialized"})
		http.Error(w, string(body), 503)
		return
	}

	configs := common.AC.GetLoadedConfigs()
	configSlice := make([]integration.Config, 0)
	for _, config := range configs {
		configSlice = append(configSlice, config)
	}
	sort.Slice(configSlice, func(i, j int) bool {
		return configSlice[i].Name < configSlice[j].Name
	})
	response.Configs = configSlice
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
	// query at the highest cardinality between checks and dogstatsd cardinalities
	cardinality := collectors.TagCardinality(max(int(tagger.ChecksCardinality), int(tagger.DogstatsdCardinality)))
	response := tagger.List(cardinality)

	jsonTags, err := json.Marshal(response)
	if err != nil {
		log.Errorf("Unable to marshal tagger list response: %s", err)
		body, _ := json.Marshal(map[string]string{"error": err.Error()})
		http.Error(w, string(body), 500)
		return
	}
	w.Write(jsonTags)
}

func secretInfo(w http.ResponseWriter, r *http.Request) {
	info, err := secrets.GetDebugInfo()
	if err != nil {
		body, _ := json.Marshal(map[string]string{"error": err.Error()})
		http.Error(w, string(body), 500)
		return
	}

	jsonInfo, err := json.Marshal(info)
	if err != nil {
		log.Errorf("Unable to marshal secrets info response: %s", err)
		body, _ := json.Marshal(map[string]string{"error": err.Error()})
		http.Error(w, string(body), 500)
		return
	}
	w.Write(jsonInfo)
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
	return r.Context().Value(ConnContextKey).(net.Conn)
}
