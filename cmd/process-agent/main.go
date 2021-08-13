// +build !windows

package main

import (
	"fmt"
	"github.com/DataDog/datadog-agent/cmd/agent/common/commands"
	"github.com/DataDog/datadog-agent/cmd/process-agent/flags"
	"github.com/DataDog/datadog-agent/cmd/process-agent/runtimecfg"
	"github.com/DataDog/datadog-agent/pkg/config/settings"
	"github.com/DataDog/datadog-agent/pkg/process/config"
	"github.com/spf13/cobra"
	_ "net/http/pprof"
)

var (
	rootCmd = &cobra.Command{
		Run: func(_ *cobra.Command, _ []string) {

			exit := make(chan struct{})

			// Invoke the Agent
			runAgent(exit)
		},
	}
)

func setupConfigClient() (settings.Client, error) {
	cfg := config.NewDefaultAgentConfig(false)

	if err := cfg.LoadProcessYamlConfig(opts.configPath); err != nil {
		return nil, err
	}
	return runtimecfg.NewProcessAgentRuntimeConfigClient(cfg.RuntimeConfigPort())
}

func init() {
	ignore := ""
	rootCmd.PersistentFlags().StringVar(&opts.configPath, "config", flags.DefaultConfPath, "[deprecated] Path to datadog.yaml config")
	rootCmd.PersistentFlags().StringVar(&opts.configPath, "cfgPath", flags.DefaultConfPath, "Path to datadog.yaml config")
	rootCmd.PersistentFlags().StringVar(&ignore, "ddconfig", "", "[deprecated] Path to dd-agent config")

	if flags.DefaultSysProbeConfPath != "" {
		rootCmd.PersistentFlags().StringVar(&opts.sysProbeConfigPath, "sysprobe-config", flags.DefaultSysProbeConfPath, "Path to system-probe.yaml config")
	}

	rootCmd.PersistentFlags().StringVar(&opts.pidfilePath, "pid", "", "Path to set pidfile for process")
	rootCmd.PersistentFlags().BoolVar(&opts.info, "info", false, "Show info about running process agent and exit")
	rootCmd.PersistentFlags().BoolVar(&opts.version, "version", false, "Print the version and exit")
	rootCmd.PersistentFlags().StringVar(&opts.check, "check", "", "Run a specific check and print the results. Choose from: process, connections, realtime")

	rootCmd.AddCommand(commands.Config(setupConfigClient))

}

func main() {

	err := rootCmd.Execute()
	if err != nil {
		fmt.Println(err)
	}
}
