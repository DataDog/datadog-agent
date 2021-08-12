// +build !windows

package main

import (
	"fmt"
	"github.com/DataDog/datadog-agent/cmd/agent/common/commands"
	"github.com/DataDog/datadog-agent/cmd/process-agent/flags"
	"github.com/DataDog/datadog-agent/cmd/process-agent/rtcfg"
	"github.com/DataDog/datadog-agent/pkg/config/settings"
	"github.com/DataDog/datadog-agent/pkg/process/config"
	"github.com/spf13/cobra"
	_ "net/http/pprof"
)

var (
	rootCmd = &cobra.Command{
		Run: func(_ *cobra.Command, _ []string) {
			startAgent()
		},
		Long: "The Datadog Agent faithfully collects information about running processes " +
			"and sends them to Datadog on your behalf",
	}

	configCmd = commands.Config(setupConfigClient)
	checkCmd  = &cobra.Command{
		Use:       "check {process|connections|realtime}",
		Short:     "Run a specific check and print the results.",
		Args:      cobra.ExactValidArgs(1),
		ValidArgs: []string{"process", "connections", "realtime"},
		Run: func(_ *cobra.Command, args []string) {
			opts.check = args[1]
			startAgent()
		},
	}
	infoCmd = &cobra.Command{
		Use:   "info",
		Short: "Show info about running process agent and exit",
		Run: func(_ *cobra.Command, args []string) {
			opts.info = true
			startAgent()
		},
	}
)

func startAgent() {
	exit := make(chan struct{})
	runAgent(exit)
}

func setupConfigClient() (settings.Client, error) {
	cfg := config.NewDefaultAgentConfig(false)

	if err := cfg.LoadProcessYamlConfig(""); err != nil {
		return nil, err
	}
	return rtcfg.NewProcessAgentRuntimeConfigClient(cfg.RuntimeConfigPort())
}

func init() {
	ignore := ""

	if flags.DefaultSysProbeConfPath != "" {
		rootCmd.PersistentFlags().StringVar(&opts.sysProbeConfigPath, "sysprobe-config", flags.DefaultSysProbeConfPath, "Path to system-probe.yaml config")
	}

	rootCmd.LocalFlags().StringVar(&opts.pidfilePath, "pid", "", "Path to set pidfile for process")
	rootCmd.LocalFlags().BoolVar(&opts.version, "version", false, "Print the version and exit")
	rootCmd.PersistentFlags().StringVarP(&opts.configPath, "cfgPath", "c", flags.DefaultConfPath, "Path to datadog.yaml config")

	// Deprecated Flags
	rootCmd.LocalFlags().StringVar(&opts.check, "check", "", "[deprecated] Run a specific check and print the results. Choose from: process, connections, realtime")
	rootCmd.LocalFlags().StringVar(&opts.configPath, "config", flags.DefaultConfPath, "[deprecated] Path to datadog.yaml config")
	rootCmd.LocalFlags().StringVar(&ignore, "ddconfig", "", "[deprecated] Path to dd-agent config")
	rootCmd.LocalFlags().BoolVar(&opts.info, "info", false, "[deprecated] Show info about running process agent and exit")

	// Add commands
	rootCmd.AddCommand(configCmd, checkCmd, infoCmd)
}

func main() {
	err := rootCmd.Execute()
	if err != nil {
		fmt.Println(err)
	}
}
