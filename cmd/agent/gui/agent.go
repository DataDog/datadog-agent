// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package gui

import (
	"encoding/json"
	"fmt"
	"html"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/gorilla/mux"
	yaml "gopkg.in/yaml.v2"

	"github.com/DataDog/datadog-agent/cmd/agent/common/path"
	"github.com/DataDog/datadog-agent/comp/core/flare"
	"github.com/DataDog/datadog-agent/comp/core/flare/helpers"
	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/status"
	"github.com/DataDog/datadog-agent/pkg/util/hostname"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/datadog-agent/pkg/version"
)

// Adds the specific handlers for /agent/ endpoints
func agentHandler(r *mux.Router, flare flare.Component) {
	r.HandleFunc("/ping", http.HandlerFunc(ping)).Methods("POST")
	r.HandleFunc("/status/{type}", http.HandlerFunc(getStatus)).Methods("POST")
	r.HandleFunc("/version", http.HandlerFunc(getVersion)).Methods("POST")
	r.HandleFunc("/hostname", http.HandlerFunc(getHostname)).Methods("POST")
	r.HandleFunc("/log/{flip}", http.HandlerFunc(getLog)).Methods("POST")
	r.HandleFunc("/flare", func(w http.ResponseWriter, r *http.Request) { makeFlare(w, r, flare) }).Methods("POST")
	r.HandleFunc("/restart", http.HandlerFunc(restartAgent)).Methods("POST")
	r.HandleFunc("/getConfig", http.HandlerFunc(getConfigFile)).Methods("POST")
	r.HandleFunc("/getConfig/{setting}", http.HandlerFunc(getConfigSetting)).Methods("GET")
	r.HandleFunc("/setConfig", http.HandlerFunc(setConfigFile)).Methods("POST")
}

// Sends a simple reply (for checking connection to server)
func ping(w http.ResponseWriter, r *http.Request) {
	elapsed := time.Now().Unix() - startTimestamp
	w.Write([]byte(strconv.FormatInt(elapsed, 10)))
}

// Sends the current agent status
func getStatus(w http.ResponseWriter, r *http.Request) {
	statusType := mux.Vars(r)["type"]

	verbose := r.URL.Query().Get("verbose") == "true"
	status, e := status.GetStatus(verbose)
	if e != nil {
		log.Errorf("Error getting status: " + e.Error())
		w.Write([]byte("Error getting status: " + e.Error()))
		return
	}
	json, _ := json.Marshal(status)
	html, e := renderStatus(json, statusType)
	if e != nil {
		w.Write([]byte("Error generating status html: " + e.Error()))
		return
	}

	w.Header().Set("Content-Type", "text/html")
	w.Write([]byte(html))
}

// Sends the current agent version
func getVersion(w http.ResponseWriter, r *http.Request) {
	version, e := version.Agent()
	if e != nil {
		log.Errorf("Error getting version: " + e.Error())
		w.Write([]byte("Error: " + e.Error()))
		return
	}

	res, _ := json.Marshal(version)
	w.Header().Set("Content-Type", "application/json")
	w.Write(res)
}

// Sends the agent's hostname
func getHostname(w http.ResponseWriter, r *http.Request) {
	hname, e := hostname.Get(r.Context())
	if e != nil {
		log.Errorf("Error getting hostname: " + e.Error())
		w.Write([]byte("Error: " + e.Error()))
		return
	}

	res, _ := json.Marshal(hname)
	w.Header().Set("Content-Type", "application/json")
	w.Write(res)
}

// Sends the log file (agent.log)
func getLog(w http.ResponseWriter, r *http.Request) {
	flip, _ := strconv.ParseBool(mux.Vars(r)["flip"])

	logFile := config.Datadog.GetString("log_file")
	if logFile == "" {
		logFile = path.DefaultLogFile
	}

	logFileContents, e := os.ReadFile(logFile)
	if e != nil {
		w.Write([]byte("Error: " + e.Error()))
		return
	}
	escapedLogFileContents := html.EscapeString(string(logFileContents))

	html := strings.Replace(escapedLogFileContents, "\n", "<br>", -1)

	if flip {
		// Reverse the order so that the bottom of the file is read first
		arr := strings.Split(escapedLogFileContents, "\n")
		for i, j := 0, len(arr)-1; i < j; i, j = i+1, j-1 {
			arr[i], arr[j] = arr[j], arr[i]
		}
		html = strings.Join(arr, "<br>")
	}

	w.Header().Set("Content-Type", "text/html")
	w.Write([]byte(html))
}

// Makes a new flare
func makeFlare(w http.ResponseWriter, r *http.Request, flare flare.Component) {
	payload, e := parseBody(r)
	if e != nil {
		w.Write([]byte(e.Error()))
		return
	} else if payload.Email == "" || payload.CaseID == "" {
		w.Write([]byte("Error creating flare: missing information"))
		return
	} else if _, err := strconv.ParseInt(payload.CaseID, 10, 0); err != nil {
		w.Write([]byte("Invalid CaseID (must be a number)"))
		return
	}

	filePath, e := flare.Create(nil, nil)
	if e != nil {
		w.Write([]byte("Error creating flare zipfile: " + e.Error()))
		log.Errorf("Error creating flare zipfile: " + e.Error())
		return
	}

	res, e := flare.Send(filePath, payload.CaseID, payload.Email, helpers.NewLocalFlareSource())
	if e != nil {
		w.Write([]byte("Flare zipfile successfully created: " + filePath + "<br><br>" + e.Error()))
		log.Errorf("Flare zipfile successfully created: " + filePath + "\n" + e.Error())
		return
	}

	w.Write([]byte("Flare zipfile successfully created: " + filePath + "<br><br>" + res))
	log.Errorf("Flare zipfile successfully created: " + filePath + "\n" + res)
}

// Restarts the agent using the appropriate (platform-specific) restart function
func restartAgent(w http.ResponseWriter, r *http.Request) {
	log.Infof("got restart function")
	e := restart()
	if e != nil {
		log.Warnf("restart failed %v", e)
		w.Write([]byte(e.Error()))
		return
	}
	log.Infof("restart success")
	w.Write([]byte("Success"))
}

func getConfigSetting(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	setting := mux.Vars(r)["setting"]
	if _, ok := map[string]bool{
		// only allow whitelisted settings:
		"apm_config.receiver_port": true,
	}[setting]; !ok {
		w.WriteHeader(http.StatusForbidden)
		fmt.Fprintf(w, `"error": "requested setting is not whitelisted"`)
		return
	}
	if err := json.NewEncoder(w).Encode(map[string]interface{}{
		setting: config.Datadog.Get(setting),
	}); err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		fmt.Fprintf(w, `"error": "%v"`, err)
	}
}

// Sends the configuration (aka datadog.yaml) file
func getConfigFile(w http.ResponseWriter, r *http.Request) {
	path := config.Datadog.ConfigFileUsed()
	settings, e := os.ReadFile(path)
	if e != nil {
		w.Write([]byte("Error: " + e.Error()))
		return
	}

	w.Header().Set("Content-Type", "text")
	w.Write(settings)
}

// Overwrites the main config file (datadog.yaml) with new data
func setConfigFile(w http.ResponseWriter, r *http.Request) {
	payload, e := parseBody(r)
	if e != nil {
		w.Write([]byte(e.Error()))
		return
	}
	data := []byte(payload.Config)

	// Check that the data is actually a valid yaml file
	cf := make(map[string]interface{})
	e = yaml.Unmarshal(data, &cf)
	if e != nil {
		w.Write([]byte("Error: " + e.Error()))
		return
	}

	path := config.Datadog.ConfigFileUsed()
	e = os.WriteFile(path, data, 0644)
	if e != nil {
		w.Write([]byte("Error: " + e.Error()))
		return
	}

	log.Infof("Successfully wrote new config file.")
	w.Write([]byte("Success"))
}
