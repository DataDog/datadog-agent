package main

import (
	_ "expvar"
	"fmt"
	_ "net/http/pprof"
	"os"

	"github.com/DataDog/datadog-agent/cmd/manage/app"
)

func main() {
	// Invoke the Agent
	if err := app.ManageCmd.Execute(); err != nil {
		fmt.Println(err)
		os.Exit(-1)
	}
}
