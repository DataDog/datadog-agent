// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018 Datadog, Inc.

// Package agent implements the api endpoints for the `/agent` prefix.
// This group of endpoints is meant to provide high-level functionalities
// at the agent level.
package agent

import (
	"encoding/json"
	"fmt"
	"net/http"

	as "github.com/DataDog/datadog-agent/pkg/util/kubernetes/apiserver"

	"github.com/DataDog/datadog-agent/cmd/agent/common"
	"github.com/DataDog/datadog-agent/cmd/agent/common/signals"
	apiutil "github.com/DataDog/datadog-agent/pkg/api/util"
	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/flare"
	"github.com/DataDog/datadog-agent/pkg/status"
	"github.com/DataDog/datadog-agent/pkg/util"
	"github.com/DataDog/datadog-agent/pkg/version"
	log "github.com/cihub/seelog"
	"github.com/gorilla/mux"
	//	"github.com/DataDog/datadog-agent/pkg/util/kubernetes/apiserver/leaderelection"
)

// EventChecks are checks that send events and are supported by the DCA
var EventChecks = []string{
	"kubernetes",
}

// SetupHandlers adds the specific handlers for cluster agent endpoints
func SetupHandlers(r *mux.Router) {
	r.HandleFunc("/version", getVersion).Methods("GET")
	r.HandleFunc("/hostname", getHostname).Methods("GET")
	r.HandleFunc("/flare", makeFlare).Methods("POST")
	r.HandleFunc("/stop", stopAgent).Methods("POST")
	r.HandleFunc("/status", getStatus).Methods("GET")
	// r.HandleFunc("/status/formatted", getFormattedStatus).Methods("GET")
	r.HandleFunc("/api/v1/metadata/{nodeName}/{podName}", getPodMetadata).Methods("GET")
	r.HandleFunc("/api/v1/{check}/events", getCheckLatestEvents).Methods("GET")
}

func getStatus(w http.ResponseWriter, r *http.Request) {
	if err := apiutil.Validate(w, r); err != nil {
		return
	}

	log.Info("Got a request for the status. Making status.")
	s, err := status.GetDCAStatus()
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

// TODO: make sure it works for DCA
func stopAgent(w http.ResponseWriter, r *http.Request) {
	if err := apiutil.Validate(w, r); err != nil {
		return
	}
	signals.Stopper <- true
	w.Header().Set("Content-Type", "application/json")
	j, _ := json.Marshal("")
	w.Write(j)
}

func getVersion(w http.ResponseWriter, r *http.Request) {
	if err := apiutil.Validate(w, r); err != nil {
		return
	}
	w.Header().Set("Content-Type", "application/json")
	av, _ := version.New(version.AgentVersion, version.Commit)
	j, _ := json.Marshal(av)
	w.Write(j)
}

func getHostname(w http.ResponseWriter, r *http.Request) {
	if err := apiutil.Validate(w, r); err != nil {
		return
	}
	w.Header().Set("Content-Type", "application/json")
	hname, err := util.GetHostname()
	if err != nil {
		log.Warnf("Error getting hostname: %s\n", err) // or something like this
		hname = ""
	}
	j, _ := json.Marshal(hname)
	w.Write(j)
}

func makeFlare(w http.ResponseWriter, r *http.Request) {
	if err := apiutil.Validate(w, r); err != nil {
		return
	}

	w.Header().Set("Content-Type", "application/json")
	log.Infof("Making a flare for the DCA")
	logFile := config.Datadog.GetString("log_file")
	if logFile == "" {
		logFile = common.DefaultLogFile
	}
	filePath, err := flare.CreateDCAArchive(false, common.GetDistPath(), logFile)
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

// TODO: complete it
func getCheckLatestEvents(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	check := vars["check"]
	supportedCheck := false
	for _, c := range EventChecks {
		if c == check {
			supportedCheck = true
			break
		}
	}
	if supportedCheck {
		// TODO
		w.Write([]byte("[OK] TODO"))
	} else {
		err := fmt.Errorf("[FAIL] TODO")
		log.Errorf("%s", err.Error())
		http.Error(w, err.Error(), 404)
	}
}

func getPodMetadata(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	var err error
	var slcB []byte
	nodeName := vars["nodeName"]
	podName := vars["podName"]

	if podName == "*" {
		log.Info("Computing service map on all nodes ...")
		svcList, errNodes := as.GetServiceMapBundleOnNode(nodeName)
		if errNodes != nil {
			log.Errorf("could not collect the service map for %s: %s", nodeName, errNodes.Error())
		}
		slcB, err = json.Marshal(svcList)
	} else {
		svcList := as.GetPodServiceNames(nodeName, podName)
		slcB, err = json.Marshal(svcList)
	}
	if err != nil {
		log.Errorf("Could not process the list of services of: %s", podName)
	}
	if len(slcB) != 0 {
		w.WriteHeader(200)
		w.Write(slcB)
		return
	}
	w.WriteHeader(404)
	w.Write([]byte(fmt.Sprintf("Could not find associated services mapped to the pod: %s on node: %s", podName, nodeName)))

}
