package api

import (
	"encoding/json"
	"net/http"

	"github.com/DataDog/datadog-agent/pkg/version"
)

func getVersion(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	av, _ := version.New(version.AgentVersion)
	j, _ := json.Marshal(av)
	w.Write(j)
}

func restart(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
}
