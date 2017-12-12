// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2017 Datadog, Inc.

package main

import (
	"flag"
	"log"
	"net/http"
	_ "net/http/pprof"
	"os"

	"github.com/DataDog/datadog-agent/pkg/pidfile"

	"github.com/DataDog/datadog-agent/pkg/logs/config"
	"github.com/DataDog/datadog-agent/pkg/logs/utils"
)

var ddconfigPath = flag.String("ddconfig", "", "Path to the datadog.yaml configuration file")
var ddconfdPath = flag.String("ddconfd", "", "Path to the conf.d directory that contains all integration config files")
var pidfilePath = flag.String("pidfile", "", "Path to set pidfile for process")

// main starts the logs agent
func main() {
	flag.Parse()

	utils.SetupLogger()

	err := config.BuildLogsAgentConfig(*ddconfigPath, *ddconfdPath)
	if err != nil {
		log.Println(err)
		log.Println("Not starting logs-agent")
	} else if config.LogsAgent.GetBool("log_enabled") {
		log.Println("Starting logs-agent")
		if *pidfilePath != "" {
			err := pidfile.WritePID(*pidfilePath)
			if err != nil {
				log.Printf("Error while writing PID file, exiting: %v", err)
				os.Exit(1)
			}

			log.Printf("pid '%d' written to pid file '%s'\n", os.Getpid(), *pidfilePath)
			defer func() {
				// remove pidfile if set
				os.Remove(*pidfilePath)
			}()
		}
		Start()

		if config.LogsAgent.GetBool("log_profiling_enabled") {
			log.Println("starting logs-agent profiling")
			go func() {
				log.Println(http.ListenAndServe("localhost:6060", nil))
			}()
		}
	} else {
		log.Println("logs-agent disabled")
	}

	done := make(chan bool)
	for range done {

	}
}
