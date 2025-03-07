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
	"io"
	"net/http"
	"strconv"
	"time"

	"github.com/gorilla/mux"
	"gopkg.in/yaml.v2"

	"github.com/DataDog/datadog-agent/cmd/agent/common/signals"
	"github.com/DataDog/datadog-agent/comp/aggregator/demultiplexer"
	"github.com/DataDog/datadog-agent/comp/api/api/types"
	"github.com/DataDog/datadog-agent/comp/core/autodiscovery"
	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/autodiscoveryimpl"
	"github.com/DataDog/datadog-agent/comp/core/settings"
	"github.com/DataDog/datadog-agent/comp/core/status"
	tagger "github.com/DataDog/datadog-agent/comp/core/tagger/def"
	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	pkgcollector "github.com/DataDog/datadog-agent/pkg/collector"
	"github.com/DataDog/datadog-agent/pkg/collector/check"
	"github.com/DataDog/datadog-agent/pkg/collector/check/stats"
	pkgconfigsetup "github.com/DataDog/datadog-agent/pkg/config/setup"
	clusterAgentFlare "github.com/DataDog/datadog-agent/pkg/flare/clusteragent"
	"github.com/DataDog/datadog-agent/pkg/logs/message"
	"github.com/DataDog/datadog-agent/pkg/metrics"
	"github.com/DataDog/datadog-agent/pkg/status/health"
	"github.com/DataDog/datadog-agent/pkg/util/defaultpaths"
	"github.com/DataDog/datadog-agent/pkg/util/hostname"
	httputils "github.com/DataDog/datadog-agent/pkg/util/http"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/datadog-agent/pkg/version"
)

// SetupHandlers adds the specific handlers for cluster agent endpoints
func SetupHandlers(r *mux.Router, wmeta workloadmeta.Component, ac autodiscovery.Component, statusComponent status.Component, settings settings.Component, taggerComp tagger.Component, demultiplexer demultiplexer.Component) {
	r.HandleFunc("/version", getVersion).Methods("GET")
	r.HandleFunc("/hostname", getHostname).Methods("GET")
	r.HandleFunc("/flare", func(w http.ResponseWriter, r *http.Request) {
		makeFlare(w, r, statusComponent)
	}).Methods("POST")
	r.HandleFunc("/stop", stopAgent).Methods("POST")
	r.HandleFunc("/status", func(w http.ResponseWriter, r *http.Request) { getStatus(w, r, statusComponent) }).Methods("GET")
	r.HandleFunc("/status/health", getHealth).Methods("GET")
	r.HandleFunc("/config-check", func(w http.ResponseWriter, r *http.Request) {
		getConfigCheck(w, r, ac)
	}).Methods("GET")
	r.HandleFunc("/config", settings.GetFullConfig("")).Methods("GET")
	r.HandleFunc("/config/list-runtime", settings.ListConfigurable).Methods("GET")
	r.HandleFunc("/config/{setting}", settings.GetValue).Methods("GET")
	r.HandleFunc("/config/{setting}", settings.SetValue).Methods("POST")
	r.HandleFunc("/tagger-list", func(w http.ResponseWriter, r *http.Request) { getTaggerList(w, r, taggerComp) }).Methods("GET")
	r.HandleFunc("/workload-list", func(w http.ResponseWriter, r *http.Request) {
		getWorkloadList(w, r, wmeta)
	}).Methods("GET")
	r.HandleFunc("check/run", func(w http.ResponseWriter, r *http.Request) {
		runChecks(w, r, ac, demultiplexer)
	}).Methods("POST")
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

func makeFlare(w http.ResponseWriter, r *http.Request, statusComponent status.Component) {
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
	filePath, err := clusterAgentFlare.CreateDCAArchive(false, defaultpaths.GetDistPath(), logFile, profile, statusComponent)
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

func runChecks(w http.ResponseWriter, r *http.Request, ac autodiscovery.Component, demultiplexer demultiplexer.Component) {
	w.Header().Set("Content-Type", "application/json")
	var checkRequest types.CheckRequest

	// Try to decode the request body into the struct. If there is an error,
	// respond to the client with the error message and a 400 status code.
	err := json.NewDecoder(r.Body).Decode(&checkRequest)
	if err != nil {
		setErrorsAndWarnings(w, []string{err.Error()}, []string{})
		return
	}

	checkName := checkRequest.Name
	times := checkRequest.Times
	pause := checkRequest.Pause
	delay := checkRequest.Delay

	allConfigs := ac.GetAllConfigs()

	for _, c := range allConfigs {
		if c.Name != checkName {
			continue
		}

		if check.IsJMXConfig(c) {
			setErrorsAndWarnings(w, []string{}, []string{"Please consider using the 'jmx' command instead of 'check jmx'"})
			return
		}
	}
	if checkRequest.ProfileConfig.Dir != "" {
		for idx := range allConfigs {
			conf := &allConfigs[idx]
			if conf.Name != checkName {
				continue
			}

			var data map[string]interface{}

			err = yaml.Unmarshal(conf.InitConfig, &data)
			if err != nil {
				setErrorsAndWarnings(w, []string{err.Error()}, []string{})
				return
			}

			if data == nil {
				data = make(map[string]interface{})
			}

			data["profile_memory"] = checkRequest.ProfileConfig.Dir
			err = populateMemoryProfileConfig(checkRequest.ProfileConfig, data)
			if err != nil {
				setErrorsAndWarnings(w, []string{err.Error()}, []string{})
				return
			}

			y, _ := yaml.Marshal(data)
			conf.InitConfig = y

			break
		}
	} else if checkRequest.Breakpoint != "" {
		breakPointLine, err := strconv.Atoi(checkRequest.Breakpoint)
		if err != nil {
			setErrorsAndWarnings(w, []string{err.Error()}, []string{})
			return
		}

		for idx := range allConfigs {
			conf := &allConfigs[idx]
			if conf.Name != checkName {
				continue
			}

			var data map[string]interface{}

			err = yaml.Unmarshal(conf.InitConfig, &data)
			if err != nil {
				setErrorsAndWarnings(w, []string{err.Error()}, []string{})
				return
			}

			if data == nil {
				data = make(map[string]interface{})
			}

			data["set_breakpoint"] = breakPointLine

			y, _ := yaml.Marshal(data)
			conf.InitConfig = y

			break
		}
	}

	cs := pkgcollector.GetChecksByNameForConfigs(checkName, allConfigs)
	// something happened while getting the check(s), display some info.
	if len(cs) == 0 {
		fetchCheckNameError(w, checkName)
		return
	}

	var instancesData []*stats.Stats
	result := types.CheckResponse{}
	metadata := make(map[string]map[string]interface{})

	for _, c := range cs {
		s := runCheck(c, times, pause)

		time.Sleep(time.Duration(delay) * time.Millisecond)

		instancesData = append(instancesData, s)
		metadata[string(c.ID())] = check.GetMetadata(c, false)
	}

	agg := demultiplexer.Aggregator()
	series, sketches := agg.GetSeriesAndSketches(time.Now())
	serviceChecks := agg.GetServiceChecks()
	events := agg.GetEvents()
	eventsPlatformEvents := agg.GetEventPlatformEvents()
	aggregatorData := types.AggregatorData{}

	if len(series) != 0 {
		aggregatorData.Series = series
	}

	if len(sketches) != 0 {
		s := make([]*metrics.SketchSeries, 0, len(sketches))
		for _, sketch := range sketches {
			s = append(s, sketch)
		}
		aggregatorData.SketchSeries = s
	}

	if len(serviceChecks) != 0 {
		aggregatorData.ServiceCheck = serviceChecks
	}

	if len(events) != 0 {
		aggregatorData.Events = events
	}

	if len(eventsPlatformEvents) != 0 {
		aggregatorData.EventPlatformEvents = toEpEvents(eventsPlatformEvents)
	}

	result.Results = instancesData
	result.Metadata = metadata
	result.AggregatorData = aggregatorData

	checkResult, _ := json.Marshal(result)

	w.Write(checkResult)
}

func fetchCheckNameError(w http.ResponseWriter, checkName string) {
	// TODO (components): move GetConfigErrors to autodicsovery component and collector
	errors := []string{}
	warnings := []string{}
	for check, error := range autodiscoveryimpl.GetConfigErrors() {
		if checkName == check {
			errors = append(errors, error)
		}
	}
	for check, CollectorErrors := range pkgcollector.GetLoaderErrors() {
		if checkName == check {
			for _, error := range CollectorErrors {
				errors = append(errors, error)
			}
		}
	}
	for check, autoDiscoveryWarnings := range autodiscoveryimpl.GetResolveWarnings() {
		if checkName == check {
			warnings = append(warnings, autoDiscoveryWarnings...)
		}
	}
	setErrorsAndWarnings(w, errors, warnings)
}

func setErrorsAndWarnings(w http.ResponseWriter, errors []string, warnings []string) {
	result := types.CheckResponse{
		Errors:   errors,
		Warnings: warnings,
	}
	body, _ := json.Marshal(result)
	http.Error(w, string(body), 500)
}

func populateMemoryProfileConfig(profileConfig types.MemoryProfileConfig, initConfig map[string]interface{}) error {
	if profileConfig.Frames != "" {
		profileMemoryFrames, err := strconv.Atoi(profileConfig.Frames)
		if err != nil {
			return fmt.Errorf("--m-frames must be an integer")
		}
		initConfig["profile_memory_frames"] = profileMemoryFrames
	}

	if profileConfig.GC != "" {
		profileMemoryGC, err := strconv.Atoi(profileConfig.GC)
		if err != nil {
			return fmt.Errorf("--m-gc must be an integer")
		}

		initConfig["profile_memory_gc"] = profileMemoryGC
	}

	if profileConfig.Combine != "" {
		profileMemoryCombine, err := strconv.Atoi(profileConfig.Combine)
		if err != nil {
			return fmt.Errorf("--m-combine must be an integer")
		}

		if profileMemoryCombine != 0 && profileConfig.Sort == "traceback" {
			return fmt.Errorf("--m-combine cannot be sorted (--m-sort) by traceback")
		}

		initConfig["profile_memory_combine"] = profileMemoryCombine
	}

	if profileConfig.Sort != "" {
		if profileConfig.Sort != "lineno" && profileConfig.Sort != "filename" && profileConfig.Sort != "traceback" {
			return fmt.Errorf("--m-sort must one of: lineno | filename | traceback")
		}
		initConfig["profile_memory_sort"] = profileConfig.Sort
	}

	if profileConfig.Limit != "" {
		profileMemoryLimit, err := strconv.Atoi(profileConfig.Limit)
		if err != nil {
			return fmt.Errorf("--m-limit must be an integer")
		}
		initConfig["profile_memory_limit"] = profileMemoryLimit
	}

	if profileConfig.Diff != "" {
		if profileConfig.Diff != "absolute" && profileConfig.Diff != "positive" {
			return fmt.Errorf("--m-diff must one of: absolute | positive")
		}
		initConfig["profile_memory_diff"] = profileConfig.Diff
	}

	if profileConfig.Filters != "" {
		initConfig["profile_memory_filters"] = profileConfig.Filters
	}

	if profileConfig.Unit != "" {
		initConfig["profile_memory_unit"] = profileConfig.Unit
	}

	if profileConfig.Verbose != "" {
		profileMemoryVerbose, err := strconv.Atoi(profileConfig.Verbose)
		if err != nil {
			return fmt.Errorf("--m-verbose must be an integer")
		}
		initConfig["profile_memory_verbose"] = profileMemoryVerbose
	}

	return nil
}

func runCheck(c check.Check, times int, pause int) *stats.Stats {
	s := stats.NewStats(c)
	for i := 0; i < times; i++ {
		t0 := time.Now()
		err := c.Run()
		warnings := c.GetWarnings()
		sStats, _ := c.GetSenderStats()
		s.Add(time.Since(t0), err, warnings, sStats, nil)
		if pause > 0 && i < times-1 {
			time.Sleep(time.Duration(pause) * time.Millisecond)
		}
	}

	return s
}

// toEpEvents transforms the raw event platform messages to EventPlatformEvent which are better for json formatting
func toEpEvents(events map[string][]*message.Message) map[string][]types.EventPlatformEvent {
	result := make(map[string][]types.EventPlatformEvent)
	for eventType, messages := range events {
		var events []types.EventPlatformEvent
		for _, m := range messages {
			e := types.EventPlatformEvent{EventType: eventType, RawEvent: string(m.GetContent())}
			err := json.Unmarshal([]byte(e.RawEvent), &e.UnmarshalledEvent)
			if err == nil {
				e.RawEvent = ""
			}
			events = append(events, e)
		}
		result[eventType] = events
	}
	return result
}
