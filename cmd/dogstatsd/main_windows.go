// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.stackstatehq.com/).
// Copyright 2016-2019 Datadog, Inc.

package main

import (
	"github.com/StackVista/stackstate-agent/pkg/config"
	"github.com/StackVista/stackstate-agent/pkg/util/winutil"
	"path/filepath"
)

var defaultLogFile = "c:\\programdata\\stackstate\\logs\\dogstatsd.log"

func init() {
	pd, err := winutil.GetProgramDataDir()
	if err == nil {
		defaultLogFile = filepath.Join(pd, "Datadog", "logs", "dogstatsd.log")
	} else {
		winutil.LogEventViewer(config.ServiceName, 0x8000000F, defaultLogFile)
	}
}
