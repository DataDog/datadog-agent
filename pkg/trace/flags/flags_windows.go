// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

// +build windows

package flags

import (
	"flag"
	"path/filepath"

	"github.com/DataDog/datadog-agent/pkg/util/winutil"
)

// DefaultConfigPath specifies the default configuration path.
var DefaultConfigPath = "c:\\programdata\\datadog\\datadog.yaml"

func init() {
	pd, err := winutil.GetProgramDataDir()
	if err == nil {
		DefaultConfigPath = filepath.Join(pd, "datadog.yaml")
	}
}
func registerOSSpecificFlags() {
	TraceCmd.PersistentFlags().BoolVar(&Win.InstallService, "install-service", false, "Install the trace agent to the Service Control Manager")
	TraceCmd.PersistentFlags().BoolVar(&Win.UninstallService, "uninstall-service", false, "Remove the trace agent from the Service Control Manager")
	TraceCmd.PersistentFlags().BoolVar(&Win.StartService, "start-service", false, "Starts the trace agent service")
	TraceCmd.PersistentFlags().BoolVar(&Win.StopService, "stop-service", false, "Stops the trace agent service")
	TraceCmd.PersistentFlags().BoolVar(&Win.Foreground, "foreground", false, "Always run foreground instead whether session is interactive or not")
}
