package main

import (
	_ "expvar"
	"fmt"
	"net/http"
	_ "net/http/pprof"
	"os"

	"github.com/DataDog/datadog-agent/cmd/agent/app"
)

func main() {
	// go_expvar server
	go http.ListenAndServe("localhost:8080", http.DefaultServeMux)

	if err := app.AgentCmd.Execute(); err != nil {
		fmt.Println(err)
		os.Exit(-1)
	}
}
