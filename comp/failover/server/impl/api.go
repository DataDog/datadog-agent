package serverimpl

import (
	"fmt"
	"net/http"
	"strconv"

	pkgconfigsetup "github.com/DataDog/datadog-agent/pkg/config/setup"
)

func startNewApiServer() {
	cmdMux := http.NewServeMux()
	cmdMux.HandleFunc("/health", HealthHandler)

	port := pkgconfigsetup.Datadog().GetInt("ha_agent.port")
	// TODO: Declare settings in config.go
	// TODO: MOVE DEFAULT TO config.go
	if port == 0 {
		port = 6001
	}

	http.ListenAndServe(":"+strconv.Itoa(port), cmdMux)
}

func HealthHandler(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
	fmt.Fprintf(w, "OK\n")
}
