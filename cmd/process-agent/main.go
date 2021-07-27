// +build !windows

package main

import (
	"flag"
	"github.com/DataDog/datadog-agent/cmd/agent/common"
	"github.com/DataDog/datadog-agent/pkg/api/util"
	"log"
	_ "net/http/pprof"
	"strings"

	"github.com/DataDog/datadog-agent/cmd/process-agent/flags"
)

func setupConfig() {
	verb, key, value :=
		strings.ToLower(flag.Arg(1)),
		strings.ToLower(flag.Arg(2)),
		flag.Arg(3)

	// Configure a session token so nothing gets rejected
	err := util.SetAuthToken()
	if err != nil {
		log.Fatal(err)
		return
	}
	settingsClient, err := common.NewSettingsClient()

	if err != nil {
		log.Fatal(err)
		return
	}

	if verb == "get" {
		get, err := settingsClient.Get(key)
		if err == nil {
			log.Println(get)
		}
	} else if verb == "set" {
		_, err = settingsClient.Set(key, value)
	} else {
		log.Println("Usage: process-agent config {get setting, set setting value}")
	}
	if err != nil {
		log.Fatal(err)
	}
}

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

	opts.isConfig = flag.Arg(0) == "config"

	exit := make(chan struct{})

	// Invoke the Agent
	runAgent(exit)
}
