// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package app

import (
	"fmt"
	"os"

	"github.com/DataDog/datadog-agent/cmd/system-probe/api/module"
	spconfig "github.com/DataDog/datadog-agent/cmd/system-probe/config"
	ddconfig "github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/runtime"
	rc "github.com/DataDog/datadog-agent/pkg/runtimecompiler"
	"github.com/DataDog/datadog-agent/pkg/runtimecompiler/config"
	"github.com/DataDog/datadog-agent/pkg/util/log"

	"github.com/DataDog/datadog-go/v5/statsd"

	"github.com/spf13/cobra"
)

const (
	statsdPoolSize = 64
)

func init() {
	SysprobeCmd.AddCommand(compileCommand)
}

var compileCommand = &cobra.Command{
	Use:   "compile",
	Short: "Compile eBPF programs",
	Long:  "Perform runtime compilation of eBPF programs used by the system-probe",
	Run:   runtimeCompile,
}

func runtimeCompile(_ *cobra.Command, _ []string) {
	/*
		Because we expect the compile command to be used in a kubernetes init container prior to the
		system-probe container, this function should never return an error, else it could prevent
		the system-probe from being able to start.
		In the event that there is an error, we should just log the error and exit.
	*/

	defer func() {
		module.Close()
		log.Flush()
	}()

	// prepare go runtime
	runtime.SetMaxProcs()

	cfg, err := spconfig.New(configPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to create system probe config: %s", err)
		return
	}

	err = ddconfig.SetupLogger(
		loggerName,
		cfg.LogLevel,
		cfg.LogFile,
		ddconfig.GetSyslogURI(),
		ddconfig.Datadog.GetBool("syslog_rfc"),
		ddconfig.Datadog.GetBool("log_to_console"),
		ddconfig.Datadog.GetBool("log_format_json"),
	)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to setup configured logger: %s", err)
		return
	}

	rcConfig := config.NewConfig(cfg)

	statsdClient, err := getStatsdClient(rcConfig)
	if err != nil {
		log.Errorf("error creating statsd client for runtime compiler: %s", err)
		statsdClient = nil
	}

	log.Infof("Starting the runtime compiler")

	rc.RuntimeCompiler.Init(rcConfig, statsdClient)
	err = rc.RuntimeCompiler.Run()
	if err != nil {
		log.Errorf("Runtime compilation error: %w", err)
	}

	log.Infof("Runtime compilation complete")
}

func getStatsdClient(cfg *config.Config) (statsd.ClientInterface, error) {
	statsdAddr := os.Getenv("STATSD_URL")
	if statsdAddr == "" {
		statsdAddr = cfg.StatsdAddr
	}

	return statsd.New(statsdAddr, statsd.WithBufferPoolSize(statsdPoolSize))
}
