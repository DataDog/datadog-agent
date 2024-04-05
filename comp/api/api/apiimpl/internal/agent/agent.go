// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package agent implements the api endpoints for the `/agent` prefix.
// This group of endpoints is meant to provide high-level functionalities
// at the agent level.
package agent

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"path"
	"sort"
	"time"

	"github.com/DataDog/zstd"
	"github.com/gorilla/mux"

	"github.com/DataDog/datadog-agent/cmd/agent/common"
	"github.com/DataDog/datadog-agent/cmd/agent/common/signals"

	"github.com/DataDog/datadog-agent/comp/aggregator/demultiplexer"
	"github.com/DataDog/datadog-agent/comp/api/api/apiimpl/response"
	"github.com/DataDog/datadog-agent/comp/collector/collector"
	"github.com/DataDog/datadog-agent/comp/core/autodiscovery"
	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/autodiscoveryimpl"
	"github.com/DataDog/datadog-agent/comp/core/flare"
	"github.com/DataDog/datadog-agent/comp/core/gui"
	"github.com/DataDog/datadog-agent/comp/core/secrets"
	"github.com/DataDog/datadog-agent/comp/core/status"
	"github.com/DataDog/datadog-agent/comp/core/tagger"
	"github.com/DataDog/datadog-agent/comp/core/tagger/collectors"
	"github.com/DataDog/datadog-agent/comp/core/workloadmeta"
	dogstatsdServer "github.com/DataDog/datadog-agent/comp/dogstatsd/server"
	dogstatsddebug "github.com/DataDog/datadog-agent/comp/dogstatsd/serverDebug"
	"github.com/DataDog/datadog-agent/comp/forwarder/eventplatformreceiver"
	logsAgent "github.com/DataDog/datadog-agent/comp/logs/agent"
	"github.com/DataDog/datadog-agent/comp/metadata/host"
	"github.com/DataDog/datadog-agent/comp/metadata/inventoryagent"
	"github.com/DataDog/datadog-agent/comp/metadata/inventorychecks"
	"github.com/DataDog/datadog-agent/comp/metadata/inventoryhost"
	"github.com/DataDog/datadog-agent/comp/metadata/packagesigning"
	"github.com/DataDog/datadog-agent/pkg/aggregator/sender"
	"github.com/DataDog/datadog-agent/pkg/config"
	settingshttp "github.com/DataDog/datadog-agent/pkg/config/settings/http"
	"github.com/DataDog/datadog-agent/pkg/diagnose"
	"github.com/DataDog/datadog-agent/pkg/diagnose/diagnosis"
	"github.com/DataDog/datadog-agent/pkg/gohai"
	"github.com/DataDog/datadog-agent/pkg/logs/diagnostic"
	"github.com/DataDog/datadog-agent/pkg/status/health"
	"github.com/DataDog/datadog-agent/pkg/util/grpc"
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
	flareComp flare.Component,
	server dogstatsdServer.Component,
	serverDebug dogstatsddebug.Component,
	wmeta workloadmeta.Component,
	logsAgent optional.Option[logsAgent.Component],
	senderManager sender.DiagnoseSenderManager,
	hostMetadata host.Component,
	invAgent inventoryagent.Component,
	demux demultiplexer.Component,
	invHost inventoryhost.Component,
	secretResolver secrets.Component,
	invChecks inventorychecks.Component,
	pkgSigning packagesigning.Component,
	statusComponent status.Component,
	collector optional.Option[collector.Component],
	eventPlatformReceiver eventplatformreceiver.Component,
	ac autodiscovery.Component,
	gui optional.Option[gui.Component],
) *mux.Router {

	r.HandleFunc("/version", common.GetVersion).Methods("GET")
	r.HandleFunc("/hostname", getHostname).Methods("GET")
	r.HandleFunc("/flare", func(w http.ResponseWriter, r *http.Request) { makeFlare(w, r, flareComp) }).Methods("POST")
	r.HandleFunc("/stop", stopAgent).Methods("POST")
	r.HandleFunc("/status", func(w http.ResponseWriter, r *http.Request) { getStatus(w, r, statusComponent) }).Methods("GET")
	r.HandleFunc("/stream-event-platform", streamEventPlatform(eventPlatformReceiver)).Methods("POST")
	r.HandleFunc("/status/health", getHealth).Methods("GET")
	r.HandleFunc("/{component}/status", componentStatusGetterHandler).Methods("GET")
	r.HandleFunc("/{component}/status", componentStatusHandler).Methods("POST")
	r.HandleFunc("/{component}/configs", componentConfigHandler).Methods("GET")
	r.HandleFunc("/gui/csrf-token", func(w http.ResponseWriter, _ *http.Request) { getCSRFToken(w, gui) }).Methods("GET")
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

	r.HandleFunc("/dogstatsd-contexts-dump", func(w http.ResponseWriter, r *http.Request) { dumpDogstatsdContexts(w, r, demux) }).Methods("POST")
	// Some agent subcommands do not provide these dependencies (such as JMX)
	if server != nil && serverDebug != nil {
		r.HandleFunc("/dogstatsd-stats", func(w http.ResponseWriter, r *http.Request) { getDogstatsdStats(w, r, server, serverDebug) }).Methods("GET")
	}

	if logsAgent, ok := logsAgent.Get(); ok {
		r.HandleFunc("/stream-logs", streamLogs(logsAgent)).Methods("POST")
	}

	return r
}

func setJSONError(w http.ResponseWriter, err error, errorCode int) {
	w.Header().Set("Content-Type", "application/json")
	body, _ := json.Marshal(map[string]string{"error": err.Error()})
	http.Error(w, string(body), errorCode)
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

func makeFlare(w http.ResponseWriter, r *http.Request, flareComp flare.Component) {
	var profile flare.ProfileData

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

	var filePath string
	var err error
	log.Infof("Making a flare")
	filePath, err = flareComp.Create(profile, nil)

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

func getStatus(w http.ResponseWriter, r *http.Request, statusComponent status.Component) {
	log.Info("Got a request for the status. Making status.")
	verbose := r.URL.Query().Get("verbose") == "true"
	format := r.URL.Query().Get("format")
	var contentType string

	contentType, ok := mimeTypeMap[format]

	if !ok {
		log.Warn("Got a request with invalid format parameter. Defaulting to 'text' format")
		format = "text"
		contentType = mimeTypeMap[format]
	}
	w.Header().Set("Content-Type", contentType)

	s, err := statusComponent.GetStatus(format, verbose)

	if err != nil {
		if format == "text" {
			http.Error(w, log.Errorf("Error getting status. Error: %v.", err).Error(), 500)
			return
		}

		setJSONError(w, log.Errorf("Error getting status. Error: %v, Status: %v", err, s), 500)
		return
	}

	w.Write(s)
}

func streamLogs(logsAgent logsAgent.Component) func(w http.ResponseWriter, r *http.Request) {
	return getStreamFunc(func() messageReceiver { return logsAgent.GetMessageReceiver() }, "logs", "logs agent")
}

func streamEventPlatform(eventPlatformReceiver eventplatformreceiver.Component) func(w http.ResponseWriter, r *http.Request) {
	return getStreamFunc(func() messageReceiver { return eventPlatformReceiver }, "event platform payloads", "agent")
}

type messageReceiver interface {
	SetEnabled(e bool) bool
	Filter(filters *diagnostic.Filters, done <-chan struct{}) <-chan string
}

func getStreamFunc(messageReceiverFunc func() messageReceiver, streamType, agentType string) func(w http.ResponseWriter, r *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {
		log.Infof("Got a request to stream %s.", streamType)
		w.Header().Set("Transfer-Encoding", "chunked")

		messageReceiver := messageReceiverFunc()

		flusher, ok := w.(http.Flusher)
		if !ok {
			log.Errorf("Expected a Flusher type, got: %v", w)
			return
		}

		if messageReceiver == nil {
			http.Error(w, fmt.Sprintf("The %s is not running", agentType), 405)
			flusher.Flush()
			log.Infof("The %s is not running - can't stream %s", agentType, streamType)
			return
		}

		if !messageReceiver.SetEnabled(true) {
			http.Error(w, fmt.Sprintf("Another client is already streaming %s.", streamType), 405)
			flusher.Flush()
			log.Infof("%s are already streaming. Dropping connection.", streamType)
			return
		}
		defer messageReceiver.SetEnabled(false)

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
		logChan := messageReceiver.Filter(&filters, done)
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
}

func getDogstatsdStats(w http.ResponseWriter, _ *http.Request, dogstatsdServer dogstatsdServer.Component, serverDebug dogstatsddebug.Component) {
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

func getHealth(w http.ResponseWriter, _ *http.Request) {
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

func getCSRFToken(w http.ResponseWriter, optGui optional.Option[gui.Component]) {
	// WARNING: GUI comp currently not provided to JMX
	gui, guiExist := optGui.Get()
	if !guiExist {
		return
	}
	w.Write([]byte(gui.GetCSRFToken()))
}

func getConfigCheck(w http.ResponseWriter, _ *http.Request, ac autodiscovery.Component) {
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
		setJSONError(w, log.Errorf("Unable to marshal config check response: %s", err), 500)
		return
	}

	w.Write(jsonConfig)
}

func getTaggerList(w http.ResponseWriter, _ *http.Request) {
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

func secretInfo(w http.ResponseWriter, _ *http.Request, secretResolver secrets.Component) {
	secretResolver.GetDebugInfo(w)
}

func secretRefresh(w http.ResponseWriter, _ *http.Request, secretResolver secrets.Component) {
	result, err := secretResolver.Refresh()
	if err != nil {
		setJSONError(w, err, 500)
		return
	}
	w.Write([]byte(result))
}

func metadataPayloadV5(w http.ResponseWriter, _ *http.Request, hostMetadataComp host.Component) {
	jsonPayload, err := hostMetadataComp.GetPayloadAsJSON(context.Background())
	if err != nil {
		setJSONError(w, log.Errorf("Unable to marshal v5 metadata payload: %s", err), 500)
		return
	}

	scrubbed, err := scrubber.ScrubBytes(jsonPayload)
	if err != nil {
		setJSONError(w, log.Errorf("Unable to scrub metadata payload: %s", err), 500)
		return
	}
	w.Write(scrubbed)
}

func metadataPayloadGohai(w http.ResponseWriter, _ *http.Request) {
	payload := gohai.GetPayloadWithProcesses(config.IsContainerized())
	jsonPayload, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		setJSONError(w, log.Errorf("Unable to marshal gohai metadata payload: %s", err), 500)
		return
	}

	scrubbed, err := scrubber.ScrubBytes(jsonPayload)
	if err != nil {
		setJSONError(w, log.Errorf("Unable to scrub gohai metadata payload: %s", err), 500)
		return
	}
	w.Write(scrubbed)
}

func metadataPayloadInvChecks(w http.ResponseWriter, _ *http.Request, invChecks inventorychecks.Component) {
	// GetAsJSON already return scrubbed data
	scrubbed, err := invChecks.GetAsJSON()
	if err != nil {
		setJSONError(w, err, 500)
		return
	}
	w.Write(scrubbed)
}

func metadataPayloadInvAgent(w http.ResponseWriter, _ *http.Request, invAgent inventoryagent.Component) {
	// GetAsJSON already return scrubbed data
	scrubbed, err := invAgent.GetAsJSON()
	if err != nil {
		setJSONError(w, err, 500)
		return
	}
	w.Write(scrubbed)
}

func metadataPayloadInvHost(w http.ResponseWriter, _ *http.Request, invHost inventoryhost.Component) {
	// GetAsJSON already return scrubbed data
	scrubbed, err := invHost.GetAsJSON()
	if err != nil {
		setJSONError(w, err, 500)
		return
	}
	w.Write(scrubbed)
}

func metadataPayloadPkgSigning(w http.ResponseWriter, _ *http.Request, pkgSigning packagesigning.Component) {
	// GetAsJSON already return scrubbed data
	scrubbed, err := pkgSigning.GetAsJSON()
	if err != nil {
		setJSONError(w, err, 500)
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
	conn := GetConnection(r)
	_ = conn.SetDeadline(time.Time{})

	// Indicate that we are already running in Agent process (and flip RunLocal)
	diagCfg.RunningInAgentProcess = true
	diagCfg.RunLocal = true

	// Get diagnoses via API
	diagnoses, err := diagnose.Run(diagCfg, diagnoseDeps)
	if err != nil {
		setJSONError(w, log.Errorf("Running diagnose in Agent process failed: %s", err), 500)
		return
	}

	// Serizalize diagnoses (and implicitly write result to the response)
	w.Header().Set("Content-Type", "application/json")
	err = json.NewEncoder(w).Encode(diagnoses)
	if err != nil {
		setJSONError(w, log.Errorf("Unable to marshal config check response: %s", err), 500)
	}
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

func dumpDogstatsdContexts(w http.ResponseWriter, _ *http.Request, demux demultiplexer.Component) {
	if demux == nil {
		setJSONError(w, log.Errorf("Unable to stream dogstatsd contexts, demultiplexer is not initialized"), 404)
		return
	}

	path, err := dumpDogstatsdContextsImpl(demux)
	if err != nil {
		setJSONError(w, log.Errorf("Failed to create dogstatsd contexts dump: %v", err), 500)
		return
	}

	resp, err := json.Marshal(path)
	if err != nil {
		setJSONError(w, log.Errorf("Failed to serialize response: %v", err), 500)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.Write(resp)
}

func dumpDogstatsdContextsImpl(demux demultiplexer.Component) (string, error) {
	path := path.Join(config.Datadog.GetString("run_path"), "dogstatsd_contexts.json.zstd")

	f, err := os.Create(path)
	if err != nil {
		return "", err
	}

	c := zstd.NewWriter(f)

	w := bufio.NewWriter(c)

	for _, err := range []error{demux.DumpDogstatsdContexts(w), w.Flush(), c.Close(), f.Close()} {
		if err != nil {
			return "", err
		}
	}

	return path, nil
}
