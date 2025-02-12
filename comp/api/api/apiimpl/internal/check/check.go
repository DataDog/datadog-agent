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
	"net/http"
	"strconv"
	"time"

	"github.com/gorilla/mux"

	"github.com/DataDog/datadog-agent/comp/aggregator/demultiplexer"
	"github.com/DataDog/datadog-agent/comp/collector/collector"
	"github.com/DataDog/datadog-agent/comp/core/autodiscovery"
	"github.com/DataDog/datadog-agent/pkg/aggregator"
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

func runChecks(collector option.Option[collector.Component], autodiscovery autodiscovery.Component, demultiplexer demultiplexer.Component, w http.ResponseWriter, r *http.Request) {
	printer := aggregator.AgentDemultiplexerPrinter{DemultiplexerWithAggregator: demultiplexer}
	r.ParseForm()

	checkName := r.FormValue("name")
	var times int
	var pause int
	var delay int
	var err error

	if times, err = strconv.Atoi(r.FormValue("times")); err != nil {
		times = 1
	}
	if pause, err = strconv.Atoi(r.FormValue("pause")); err != nil {
		pause = 0
	}
	if delay, err = strconv.Atoi(r.FormValue("delay")); err != nil {
		delay = 100
	}

	allConfigs := autodiscovery.GetAllConfigs()
	cs := pkgcollector.GetChecksByNameForConfigs(checkName, allConfigs)
	// something happened while getting the check(s), display some info.
	if len(cs) == 0 {
		fetchCheckNameError(w, checkName)
		return
	}

	var instancesData []interface{}

	for _, c := range cs {
		s := runCheck(c, times, pause)

		time.Sleep(time.Duration(delay) * time.Millisecond)

		instanceData := map[string]interface{}{
			"aggregator": printer.GetMetricsDataForPrint(),
			"runner":     s,
		}

		instancesData = append(instancesData, instanceData)

	}
	instancesJSON, _ := json.Marshal(instancesData)
	w.Write(instancesJSON)
}

func fetchCheckNameError(w http.ResponseWriter, checkName string) {
	// TODO
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
