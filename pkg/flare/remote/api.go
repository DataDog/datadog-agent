// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

// Package agent implements the api endpoints for the `/agent` prefix.
// This group of endpoints is meant to provide high-level functionalities
// at the agent level.

package remote

import (
	"encoding/json"
	"io/ioutil"
	"net/http"
	"strconv"
	"time"

	"github.com/gorilla/mux"

	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/flare"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

var (
	defaultLogFile    string
	defaultJmxLogFile string
	distPath          string
	pyChecksPath      string
)

func InitAPI(logFile, jmxLogFile, distPath, pyPath string) {
	defaultLogFile = logFile
	defaultJmxLogFile = jmxLogFile
	distPath = distPath
	pyChecksPath = pyPath
}

func Handler(w http.ResponseWriter, r *http.Request) {

	vars := mux.Vars(r)
	flareType := vars["type"]
	opType, ok := vars["op"]

	if flareType == "trace" {
		tracerFlare(w, r, vars)
	} else if flareType == "core" {
		if !ok || opType == "gen" {
			vanillaFlare(w, r)
		} else {
			http.Error(w, log.Errorf("unsupported op type: %s", opType).Error(), 400)
		}
	} else {
		http.Error(w, log.Errorf("unsupported flare type: %s", flareType).Error(), 400)
	}
}

func LogHandler(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	flareId := vars["flare_id"]
	tracerId := vars["tracer_id"]
	log.Tracef("Logging entry for flare (%v) from tracer: %v", flareId, tracerId)

	// r.Body content is simple UTF-8 encoded string text/plain; charset=utf-8

	w.Header().Set("Content-Type", "application/json")
	if err := LogEntry(flareId, tracerId, r.Body); err != nil {
		body, _ := json.Marshal(map[string]string{"error": err.Error()})
		switch err.(type) {
		case *InvalidLogType:
		case *InvalidTracerId:
		case *InvalidFlareId:
			http.Error(w, string(body), 400)
		default:
			http.Error(w, string(body), 500)
		}
		return
	}

}

func tracerFlare(w http.ResponseWriter, r *http.Request, vars map[string]string) {
	w.Header().Set("Content-Type", "application/json")

	opType, ok := vars["op"]
	if !ok {
		http.Error(w, log.Error("no flare operation was specified: %v", vars).Error(), 400)
	}

	if opType == "status" {
		flareId := r.PostFormValue("flare_id")
		status := GetStatus(flareId)

		j, _ := json.Marshal(status)
		w.Write(j)

		return
	}

	if opType == "gen" {
		tracerId := r.PostFormValue("tracer_id")
		tracerEnv := r.PostFormValue("environment")
		tracerSvc := r.PostFormValue("service")
		tracerLang := r.PostFormValue("language")

		duration, err := strconv.Atoi(r.PostFormValue("duration"))
		if err != nil {
			http.Error(w, log.Errorf("error parsing flare duration interval: %s", err).Error(), 400)
			return
		}

		// TODO: use tracer lang for something
		if tracerId != "" || tracerEnv != "" || tracerSvc != "" || tracerLang != "" {
			flare, err := CreateRemoteFlareArchive(tracerId, tracerEnv, tracerSvc, time.Duration(duration)*time.Second)
			if err != nil {
				http.Error(w, log.Errorf("error creating flare: %s", err).Error(), 500)
				return
			}

			status := GetStatus(flare.GetId())
			j, _ := json.Marshal(status)
			w.Write(j)

			return
		}
	}

	http.Error(w, log.Errorf("unable to create flare, bad arguments").Error(), 400)
	return
}

func vanillaFlare(w http.ResponseWriter, r *http.Request) {
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
		logFile = defaultLogFile
	}

	jmxLogFile := config.Datadog.GetString("jmx_log_file")
	if jmxLogFile == "" {
		jmxLogFile = defaultJmxLogFile
	}
	logFiles := []string{logFile, jmxLogFile}

	log.Infof("Making a flare")
	filePath, err := flare.CreateArchive(false, distPath, pyChecksPath, logFiles, profile)
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
