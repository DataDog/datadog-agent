// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build windows
// +build windows

package flags

import (
	"flag"
	"path/filepath"

	"github.com/DataDog/datadog-agent/pkg/util/winutil"
)
import "github.com/DataDog/datadog-agent/pkg/traceinit"

// DefaultConfigPath specifies the default configuration path.
var DefaultConfigPath = "c:\\programdata\\datadog\\datadog.yaml"

func init() {
	traceinit.TraceFunction(`\DataDog\datadog-agent\cmd\trace-agent\internal\flags\flags_windows.go 21`)
	pd, err := winutil.GetProgramDataDir()
	traceinit.TraceFunction(`\DataDog\datadog-agent\cmd\trace-agent\internal\flags\flags_windows.go 22`)
	if err == nil {
		DefaultConfigPath = filepath.Join(pd, "datadog.yaml")
	}
	traceinit.TraceFunction(`\DataDog\datadog-agent\cmd\trace-agent\internal\flags\flags_windows.go 25`)
}

func registerOSSpecificFlags() {
	flag.BoolVar(&Win.InstallService, "install-service", false, "Install the trace agent to the Service Control Manager")
	flag.BoolVar(&Win.UninstallService, "uninstall-service", false, "Remove the trace agent from the Service Control Manager")
	flag.BoolVar(&Win.StartService, "start-service", false, "Starts the trace agent service")
	flag.BoolVar(&Win.StopService, "stop-service", false, "Stops the trace agent service")
	flag.BoolVar(&Win.Foreground, "foreground", false, "Always run foreground instead whether session is interactive or not")
}
