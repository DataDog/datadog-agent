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

// SetupHandlers adds the specific handlers for /check endpoints
func SetupHandlers(r *mux.Router) {
	r.HandleFunc("/", listChecks).Methods("GET")
	r.HandleFunc("/{name}", listChecks).Methods("GET", "DELETE")
	r.HandleFunc("/{name}/reload", reloadCheck).Methods("POST")
}

func reloadCheck(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/plain")
	w.Write([]byte("Not yet implemented."))
}

func listChecks(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/plain")

	type CheckDetail struct {
		Name string
		ID   string
	}
	detailsList := []CheckDetail{}

	if common.AgentScheduler == nil {
		// Service Unavailable
		w.WriteHeader(503)
	}

	for _, cPtr := range common.AgentScheduler.ScheduledChecks() {
		check := *cPtr
		detailsList = append(detailsList, CheckDetail{check.String(), check.ID()})
	}

	j, _ := json.Marshal(detailsList)
	w.Write(j)
}
