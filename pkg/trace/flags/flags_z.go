// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

package flags

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

var (
	// ConfigPath specifies the path to the configuration file.
	ConfigPath string

	// PIDFilePath specifies the path to the PID file.
	PIDFilePath string

	// LogLevel specifies the log output level.
	LogLevel string

	// Version will cause the agent to show version information.
	Version bool

	// Info will display information about a running agent.
	Info bool

	// CPUProfile specifies the path to output CPU profiling information to.
	// When empty, CPU profiling is disabled.
	CPUProfile string

	// MemProfile specifies the path to output memory profiling information to.
	// When empty, memory profiling is disabled.
	MemProfile string

	// TraceCmd is the root command
	TraceCmd = &cobra.Command{
		Use:   fmt.Sprintf("%s [command]", os.Args[0]),
		Short: "Datadog Agent at your service.",
		Long: `
The Datadog Agent faithfully collects events and metrics and brings them
to Datadog on your behalf so that you can do something useful with your
monitoring and performance data.`,
		SilenceUsage: true,
	}
)

// Win holds a set of flags which will be populated only during the Windows build.
var Win = struct {
	InstallService   bool
	UninstallService bool
	StartService     bool
	StopService      bool
	Foreground       bool
}{}

func init() {
	TraceCmd.PersistentFlags().StringVar(&ConfigPath, "config", DefaultConfigPath, "Datadog Agent config file location")
	TraceCmd.PersistentFlags().StringVar(&PIDFilePath, "pid", "", "Path to set pidfile for process")
	TraceCmd.PersistentFlags().BoolVar(&Version, "version", false, "Show version information and exit")
	TraceCmd.PersistentFlags().BoolVar(&Info, "info", false, "Show info about running trace agent process and exit")

	// profiling
	TraceCmd.PersistentFlags().StringVar(&CPUProfile, "cpuprofile", "", "Write cpu profile to file")
	TraceCmd.PersistentFlags().StringVar(&MemProfile, "memprofile", "", "Write memory profile to `file`")

	registerOSSpecificFlags()
}
