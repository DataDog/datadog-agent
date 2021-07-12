// +build !windows

package main

import (
	"flag"
	_ "net/http/pprof"
)

func main() {
	ignore := ""
	flag.StringVar(&opts.configPath, "config", "/etc/datadog-agent/datadog.yaml", "Path to datadog.yaml config")
	flag.StringVar(&ignore, "ddconfig", "", "[deprecated] Path to dd-agent config")
	flag.StringVar(&opts.sysProbeConfigPath, "sysprobe-config", "/etc/datadog-agent/system-probe.yaml", "Path to system-probe.yaml config")
	flag.StringVar(&opts.pidfilePath, "pid", "", "Path to set pidfile for process")
	flag.BoolVar(&opts.info, "info", false, "Show info about running process agent and exit")
	flag.BoolVar(&opts.version, "version", false, "Print the version and exit")
	flag.StringVar(&opts.check, "check", "", "Run a specific check and print the results. Choose from: process, connections, realtime")
	flag.Parse()

	exit := make(chan struct{})

	// Invoke the Agent
	runAgent(exit)
}
