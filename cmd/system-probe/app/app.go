package app

import (
	"fmt"
	"os"

	ddconfig "github.com/DataDog/datadog-agent/pkg/config"
	"github.com/spf13/cobra"
)

var (
	// SysprobeCmd is the root command
	SysprobeCmd = &cobra.Command{
		Use:   fmt.Sprintf("%s [command]", os.Args[0]),
		Short: "Datadog Agent System Probe",
		Long: `
The Datadog Agent System Probe runs as superuser in order to instrument 
your machine at a deeper level. It is required for features such as Network Performance Monitoring,
Runtime Security Monitoring, and others.`,
		SilenceUsage: true,
	}
	configPath  string
	flagNoColor bool
)

const loggerName = ddconfig.LoggerName("SYS-PROBE")

func init() {
	SysprobeCmd.PersistentFlags().StringVarP(&configPath, "config", "c", "", "Path to system-probe config formatted as YAML")
	SysprobeCmd.PersistentFlags().BoolVarP(&flagNoColor, "no-color", "n", false, "disable color output")
}
