// Package check implements the api endpoints for the `/check` prefix.
// This group of endpoints is meant to provide specific functionalities
// to interact with agent checks.
package check

import (
	"encoding/json"
	"net/http"

	"github.com/DataDog/datadog-agent/cmd/agent/common"
	"github.com/gorilla/mux"
)

type checkDetail struct {
	Name    string
	ID      string
	Running bool
}

// SetupHandlers adds the specific handlers for /check endpoints
func SetupHandlers(r *mux.Router) {
	r.HandleFunc("/", listChecks).Methods("GET")
	r.HandleFunc("/{name}", listCheck).Methods("GET", "DELETE")
	r.HandleFunc("/{name}/reload", reloadCheck).Methods("POST")
}

func reloadCheck(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	vars := mux.Vars(r)
	checkName := vars["name"]
	_, found := common.AgentScheduler.ScheduledChecks()[checkName]
	if found {
		common.ReloadCheck(checkName)
	} else {
		http.NotFound(w, r)
	}
}

func listChecks(w http.ResponseWriter, r *http.Request) {
	detailsList := []checkDetail{}
	if common.AgentScheduler == nil {
		// Service Unavailable
		w.WriteHeader(503)
		return
	}

	w.Header().Set("Content-Type", "application/json")

	for _, cPtr := range common.AgentScheduler.ScheduledChecks() {
		check := *cPtr
		detailsList = append(detailsList, checkDetail{check.String(), check.ID(), common.AgentRunner.IsCheckRunning(check)})
	}

	j, _ := json.Marshal(detailsList)
	w.Write(j)
}

func listCheck(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	vars := mux.Vars(r)
	checkName := vars["name"]
	cPtr, found := common.AgentScheduler.ScheduledChecks()[checkName]
	if found {
		check := *cPtr
		j, _ := json.Marshal(checkDetail{check.String(), check.ID(), common.AgentRunner.IsCheckRunning(check)})
		w.Write(j)
	} else {
		http.NotFound(w, r)
	}
}
