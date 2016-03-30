package main

import (
	_ "expvar"
	"net/http"
	_ "net/http/pprof"

	"github.com/DataDog/datadog-agent/agentmain"
)

func main() {
	// go_expvar server
	go http.ListenAndServe(":8080", http.DefaultServeMux)

	ddagentmain.Start()
}
