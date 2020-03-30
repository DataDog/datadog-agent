// +build !windows

package main

import (
	"flag"
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/DataDog/datadog-agent/pkg/process/config"

	_ "net/http/pprof"
)

func main() {
	// Parse flags
	flag.StringVar(&opts.configPath, "config", "/etc/datadog-agent/system-probe.yaml", "Path to system-probe config formatted as YAML")
	flag.StringVar(&opts.pidFilePath, "pid", "", "Path to set pidfile for process")
	flag.BoolVar(&opts.version, "version", false, "Print the version and exit")

	allChecks := make([]string, 0, len(checkEndpoints))
	for check := range checkEndpoints {
		allChecks = append(allChecks, check)
	}
	sort.Strings(allChecks)
	opts.checkCmd = flag.NewFlagSet("check", flag.ExitOnError)
	opts.checkCmd.StringVar(&opts.checkType, "type", "", "The type of check to run. Choose from: "+strings.Join(allChecks, ", "))
	opts.checkCmd.StringVar(&opts.checkClient, "client", "", "The client ID that the check will use to run")
	flag.Parse()

	runAgent()
}

// run check command if the flag is specified
func runCheck(cfg *config.AgentConfig) {
	if len(flag.Args()) >= 1 && flag.Arg(0) == "check" {
		err := opts.checkCmd.Parse(flag.Args()[1:])
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		if opts.checkType == "" {
			opts.checkCmd.PrintDefaults()
			os.Exit(1)
		}
		err = querySocketEndpoint(cfg, opts.checkType, opts.checkClient)
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}

		os.Exit(0)
	}
}
