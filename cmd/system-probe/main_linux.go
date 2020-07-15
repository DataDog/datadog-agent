// +build linux

package main

import (
	"flag"

	"github.com/DataDog/datadog-agent/pkg/process/util"
)

func main() {
	// Parse flags
	flag.StringVar(&opts.configPath, "config", "/etc/datadog-agent/system-probe.yaml", "Path to system-probe config formatted as YAML")
	flag.StringVar(&opts.pidFilePath, "pid", "", "Path to set pidfile for process")
	flag.BoolVar(&opts.version, "version", false, "Print the version and exit")
	flag.Parse()

	// Handles signals, which tells us whether we should exit.
	exit := make(chan struct{})
	go util.HandleSignals(exit)
	runAgent(exit)
}
