// +build !windows

package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/DataDog/datadog-agent/pkg/process/config"

	_ "net/http/pprof"
)

func main() {
	// Parse flags
	flag.StringVar(&opts.configPath, "config", "/etc/datadog-agent/system-probe.yaml", "Path to system-probe config formatted as YAML")
	flag.StringVar(&opts.pidFilePath, "pid", "", "Path to set pidfile for process")
	flag.BoolVar(&opts.version, "version", false, "Print the version and exit")

	opts.checkCmd = flag.NewFlagSet("check", flag.ExitOnError)
	flag.StringVar(&opts.checkType, "type", "", "The type of check to run. Choose from: conections, network_maps, network_state, stags")
	flag.StringVar(&opts.checkClient, "client", "", "The client ID that the check will use to run")
	flag.Parse()

	runAgent()
}

// run check command if the flag is specified
func runCheck(cfg *config.AgentConfig) {
	if len(os.Args) >= 2 && os.Args[1] == "check" {
		err := opts.checkCmd.Parse(os.Args[2:])
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			cleanupAndExit(1)
		}
		if opts.checkType == "" {
			opts.checkCmd.PrintDefaults()
			cleanupAndExit(1)
		}
		err = querySocketEndpoint(cfg, opts.checkType, opts.checkClient)
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			cleanupAndExit(1)
		}

		cleanupAndExit(0)
	}
}
