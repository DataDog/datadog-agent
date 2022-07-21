// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package app

import (
	"fmt"
	"os"

	ddconfig "github.com/DataDog/datadog-agent/pkg/config"
	"github.com/spf13/cobra"
)
import "github.com/DataDog/datadog-agent/pkg/traceinit"

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
	traceinit.TraceFunction(`\DataDog\datadog-agent\cmd\system-probe\app\app.go 33`)
	SysprobeCmd.PersistentFlags().StringVarP(&configPath, "config", "c", "", "Path to system-probe config formatted as YAML")
	traceinit.TraceFunction(`\DataDog\datadog-agent\cmd\system-probe\app\app.go 34`)
	SysprobeCmd.PersistentFlags().BoolVarP(&flagNoColor, "no-color", "n", false, "disable color output")
	traceinit.TraceFunction(`\DataDog\datadog-agent\cmd\system-probe\app\app.go 35`)
}
