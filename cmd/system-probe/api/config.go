package api

import (
	"github.com/DataDog/datadog-agent/cmd/system-probe/config"
	settingshttp "github.com/DataDog/datadog-agent/pkg/config/settings/http"
	"github.com/gorilla/mux"
)

// setupConfigHandlers adds the specific handlers for /config endpoints
func setupConfigHandlers(r *mux.Router) {
	r.HandleFunc("/config", settingshttp.Server.GetFull(config.Namespace)).Methods("GET")
	r.HandleFunc("/config/list-runtime", settingshttp.Server.ListConfigurable).Methods("GET")
	r.HandleFunc("/config/{setting}", settingshttp.Server.GetValue).Methods("GET")
	r.HandleFunc("/config/{setting}", settingshttp.Server.SetValue).Methods("POST")
}
