// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2017 Datadog, Inc.

// +build !windows

package main

import (
	"fmt"
	"os"

	"github.com/DataDog/datadog-agent/cmd/agent/app"
	"github.com/DataDog/datadog-agent/pkg/config"
	log "github.com/cihub/seelog"
	"github.com/mitchellh/panicwrap"
)

func panicHandler(output string) {
	// output contains the full output (including stack traces)
	err := config.SetupLogger(
		"error", config.Datadog.GetString("log_panic_file"),
		"", false, false, "")

	if err == nil {
		log.Errorf("Agent panicked (oh no!):\n\n%s\n", output)
		log.Flush()
	} else {
		fmt.Printf("Agent panicked (oh no!):\n\n%s\n", output)
	}

	os.Exit(1)
}

func main() {
	// Invoke the Agent
	exitStatus, err := panicwrap.BasicWrap(panicHandler)
	if err != nil {
		// Something went wrong setting up the panic wrapper. Unlikely,
		// but possible.
		panic(err)
	}

	// If exitStatus >= 0, then we're the parent process and the panicwrap
	// re-executed ourselves and completed. Just exit with the proper status.
	if exitStatus >= 0 {
		os.Exit(exitStatus)
	}
	if err := app.AgentCmd.Execute(); err != nil {
		os.Exit(-1)
	}
}
