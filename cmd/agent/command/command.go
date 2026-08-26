// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package command implements the top-level `agent` binary, including its subcommands.
package command

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/fatih/color"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"

	"github.com/DataDog/datadog-agent/comp/core"
	"github.com/DataDog/datadog-agent/comp/core/config"
	log "github.com/DataDog/datadog-agent/comp/core/log/def"
)

const (
	// ConfigName is the name of the config
	ConfigName = "datadog"
	// LoggerName is the name of the logger instance
	LoggerName = "CORE"
)

// GlobalParams contains the values of agent-global Cobra flags.
//
// A pointer to this type is passed to SubcommandFactory's, but its contents
// are not valid until Cobra calls the subcommand's Run or RunE function.
type GlobalParams struct {
	// ConfFilePath holds the path to the folder containing the configuration
	// file, to allow overrides from the command line
	ConfFilePath string

	// ExtraConfFilePath represents the paths to additional configuration files.
	ExtraConfFilePath []string

	// SysProbeConfFilePath holds the path to the folder containing the system-probe
	// configuration file, to allow overrides from the command line
	SysProbeConfFilePath string

	// LogStreamFilePath holds the path to the logstream log path
	LogStreamFilePath string

	// FleetPoliciesDirPath holds the path to the folder containing the remotely received agent
	// configuration files, to allow overrides from the command line
	FleetPoliciesDirPath string

	// NoColor is a flag to disable color output
	NoColor bool
}

// SubcommandFactory is a callable that will return a slice of subcommands.
type SubcommandFactory func(globalParams *GlobalParams) []*cobra.Command

// GetDefaultCoreBundleParams returns the default params for the Core Bundle (config loaded from the "datadog" file
// and logger disabled).
func GetDefaultCoreBundleParams(globalParams *GlobalParams) core.BundleParams {
	configOpts := []func(*config.Params){
		config.WithExtraConfFiles(globalParams.ExtraConfFilePath),
		config.WithFleetPoliciesDirPath(globalParams.FleetPoliciesDirPath),
	}
	return core.BundleParams{
		ConfigParams: config.NewAgentParams(globalParams.ConfFilePath, configOpts...),
		LogParams:    log.ForOneShot(LoggerName, "off", true)}
}

// MakeCommand makes the top-level Cobra command for this app.
func MakeCommand(subcommandFactories []SubcommandFactory) *cobra.Command {
	globalParams := GlobalParams{}

	// AgentCmd is the root command
	agentCmd := &cobra.Command{
		// cobra will tokenize the "Use" string by space, and take the first one so there's no need to pass anything
		// besides the filename of the executable.
		// Not even '[command]' is respected - try using their example "add [-F file | -D dir]... [-f format] profile"
		// and it will still come out as "add [command]" in the help output.
		// If the file name contains a space, this will break - but this is not the case for the Agent executable.
		Use:   filepath.Base(os.Args[0]),
		Short: "Datadog Agent at your service.",
		Long: `
The Datadog Agent faithfully collects events and metrics and brings them
to Datadog on your behalf so that you can do something useful with your
monitoring and performance data.`,
		SilenceUsage: true,
	}

	RegisterGlobalFlags(agentCmd.PersistentFlags(), &globalParams)
	_ = agentCmd.PersistentFlags().MarkHidden("fleetcfgpath")
	agentCmd.PersistentPreRun = func(*cobra.Command, []string) {
		if globalParams.NoColor {
			color.NoColor = true
		}
	}

	for _, sf := range subcommandFactories {
		subcommands := sf(&globalParams)
		for _, cmd := range subcommands {
			agentCmd.AddCommand(cmd)
		}
	}

	return agentCmd
}

// RegisterGlobalFlags registers the Agent flags shared by normal Cobra dispatch and remote command pre-dispatch.
func RegisterGlobalFlags(flags *pflag.FlagSet, params *GlobalParams) {
	flags.StringVarP(&params.ConfFilePath, "cfgpath", "c", "", "path to directory containing datadog.yaml")
	flags.StringArrayVarP(&params.ExtraConfFilePath, "extracfgpath", "E", []string{}, "specify additional configuration files to be loaded sequentially after the main datadog.yaml")
	flags.StringVarP(&params.SysProbeConfFilePath, "sysprobecfgpath", "", "", "path to directory containing system-probe.yaml")
	flags.StringVarP(&params.FleetPoliciesDirPath, "fleetcfgpath", "", "", "path to the directory containing fleet policies")
	flags.BoolVarP(&params.NoColor, "no-color", "n", false, "disable color output")
}

// ParseGlobalFlags parses recognized root flags without consuming provider-specific arguments.
func ParseGlobalFlags(args []string, params *GlobalParams) error {
	flags := pflag.NewFlagSet("agent", pflag.ContinueOnError)
	RegisterGlobalFlags(flags, params)

	for index := 0; index < len(args); index++ {
		arg := args[index]
		if arg == "--" {
			break
		}
		if !strings.HasPrefix(arg, "-") {
			continue
		}

		name, value, hasValue := strings.Cut(strings.TrimLeft(arg, "-"), "=")
		flag := flags.Lookup(name)
		if flag == nil {
			continue
		}
		if !hasValue {
			if flag.NoOptDefVal != "" {
				value = flag.NoOptDefVal
			} else if index+1 < len(args) {
				index++
				value = args[index]
			} else {
				return pflag.ErrHelp
			}
		}
		if err := flags.Set(flag.Name, value); err != nil {
			return err
		}
	}
	return nil
}

// LogLevelDefaultOff is used only for commands where logs are disabled by default.
// It allows to enabled logs for debugging purpose.
type LogLevelDefaultOff struct {
	value string
}

// Register adds the log_level flag to the command.
func (o *LogLevelDefaultOff) Register(cmd *cobra.Command) {
	cmd.PersistentFlags().StringVarP(&o.value, "log_level", "", "off", "Override the log level for this command for debugging purposes")
}

// Value returns the value of the log_level flag.
func (o *LogLevelDefaultOff) Value() string {
	return o.value
}
