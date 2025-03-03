// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package check implements the api endpoints for the `/check` prefix.
// This group of endpoints is meant to provide specific functionalities
// to interact with agent checks.
package check

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/gorilla/mux"
	"gopkg.in/yaml.v2"

	"github.com/DataDog/datadog-agent/comp/aggregator/demultiplexer"
	"github.com/DataDog/datadog-agent/comp/api/api/types"
	"github.com/DataDog/datadog-agent/comp/collector/collector"
	"github.com/DataDog/datadog-agent/comp/core/autodiscovery"
	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/autodiscoveryimpl"
	pkgcollector "github.com/DataDog/datadog-agent/pkg/collector"
	"github.com/DataDog/datadog-agent/pkg/collector/check"
	"github.com/DataDog/datadog-agent/pkg/collector/check/stats"
	"github.com/DataDog/datadog-agent/pkg/util/option"
)

// SetupHandlers adds the specific handlers for /check endpoints
func SetupHandlers(
	r *mux.Router,
	collector option.Option[collector.Component],
	autodiscovery autodiscovery.Component,
	demultiplexer demultiplexer.Component,
) *mux.Router {
	r.HandleFunc("/", listChecks).Methods("GET")
	r.HandleFunc("/{name}", listCheck).Methods("GET", "DELETE")
	r.HandleFunc("/{name}/reload", reloadCheck).Methods("POST")
	r.HandleFunc("/run", func(w http.ResponseWriter, r *http.Request) {
		runChecks(collector, autodiscovery, demultiplexer, w, r)
	}).Methods("POST")

	return r
}

func reloadCheck(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "text/plain")
	w.Write([]byte("Not yet implemented."))
}

func listChecks(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "text/plain")
	w.Write([]byte("Not yet implemented."))
}

func listCheck(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "text/plain")
	w.Write([]byte("Not yet implemented."))
}

type memoryProfileConfig struct {
	dir     string
	frames  string
	gc      string
	combine string
	sort    string
	limit   string
	diff    string
	filters string
	unit    string
	verbose string
}

// TODO (component): Use collector once it implement GetLoaderErrors
func runChecks(_ option.Option[collector.Component], autodiscovery autodiscovery.Component, _ demultiplexer.Component, w http.ResponseWriter, r *http.Request) {
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

	allConfigs := autodiscovery.GetAllConfigs()

	for _, c := range allConfigs {
		if c.Name != checkName {
			continue
		}

		if check.IsJMXConfig(c) {
			fmt.Println("Please consider using the 'jmx' command instead of 'check jmx'")
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

	for _, c := range cs {
		s := runCheck(c, times, pause)

		time.Sleep(time.Duration(delay) * time.Millisecond)

		instancesData = append(instancesData, s)
	}
	instancesJSON, _ := json.Marshal(instancesData)
	w.Write(instancesJSON)
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
	for check, warnings := range autodiscoveryimpl.GetResolveWarnings() {
		if checkName == check {
			for _, warning := range warnings {
				warnings = append(warnings, warning)
			}
		}
	}
	setErrorsAndWarnings(w, errors, warnings)
}

func setErrorsAndWarnings(w http.ResponseWriter, errors []string, warnings []string) {
	body, _ := json.Marshal(map[string][]string{"errors": errors, "warnings": warnings})
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
