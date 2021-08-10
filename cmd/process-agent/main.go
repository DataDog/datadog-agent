// +build !windows

package main

import (
	"flag"
	"fmt"
	"github.com/DataDog/datadog-agent/cmd/agent/common"
	"github.com/DataDog/datadog-agent/cmd/agent/common/commands"
	"github.com/DataDog/datadog-agent/cmd/process-agent/flags"
	"github.com/DataDog/datadog-agent/cmd/process-agent/runtime_config"
	"github.com/DataDog/datadog-agent/pkg/api/util"
	"github.com/DataDog/datadog-agent/pkg/config/settings"
	"github.com/spf13/cobra"
	_ "net/http/pprof"
)

var (
	RootCmd = &cobra.Command{
		Run: func(_ *cobra.Command, _ []string) {
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
		},
	}
)

func setupConfig() (cli settings.Client, err error) {
	// Configure a session token so nothing gets rejected
	err = common.SetupConfigWithoutSecrets("", "")
	if err != nil {
		return
	}

	err = util.SetAuthToken()
	if err != nil {
		return
	}

	return common.NewSettingsClient()
}

func init() {
	RootCmd.AddCommand(commands.Config(runtime_config.SetupConfig))
}

func main() {
	err := RootCmd.Execute()
	if err != nil {
		fmt.Println(err)
	}
}
