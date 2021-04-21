// +build !windows

package main

import (
	"flag"
	_ "net/http/pprof"

	"github.com/DataDog/datadog-agent/cmd/process-agent/flags"
)

func main() {
	ignore := ""
	flag.StringVar(&opts.configPath, "config", flags.DefaultConfPath, "Path to datadog.yaml config")
	flag.StringVar(&ignore, "ddconfig", "", "[deprecated] Path to dd-agent config")

	if flags.DefaultSysProbeConfPath != "" {
		flag.StringVar(&opts.sysProbeConfigPath, "sysprobe-config", flags.DefaultSysProbeConfPath, "Path to system-probe.yaml config")
	}

	flag.StringVar(&opts.pidfilePath, "pid", "", "Path to set pidfile for process")
	flag.BoolVar(&opts.info, "info", false, "Show info about running process agent and exit")
	flag.BoolVar(&opts.version, "version", false, "Print the version and exit")
	flag.StringVar(&opts.check, "check", "", "Run a specific check and print the results. Choose from: process, connections, realtime")
	flag.Parse()

	exit := make(chan struct{})

	// Invoke the Agent
	runAgent(exit)
}
