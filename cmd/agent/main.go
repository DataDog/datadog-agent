package main

import (
	_ "expvar"
	"fmt"
	"net/http"
	_ "net/http/pprof"
	"os"

	"github.com/DataDog/datadog-agent/cmd/agent/app"
	"github.com/gorilla/mux"

	// register core checks
	_ "github.com/DataDog/datadog-agent/pkg/collector/check/core/system"
)

func main() {
	// root HTTP router
	r := mux.NewRouter()

	// IPC REST API server

	// go_expvar server
	r.Handle("/debug/vars", http.DefaultServeMux)
	go http.ListenAndServe("localhost:5000", r)

	// Invoke the Agent
	if err := app.AgentCmd.Execute(); err != nil {
		fmt.Println(err)
		os.Exit(-1)
	}
}
