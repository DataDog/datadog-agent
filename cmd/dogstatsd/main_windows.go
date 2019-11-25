// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2019 Datadog, Inc.

package main

import (
	"path/filepath"

	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/util/winutil"
)

var defaultLogFile = "c:\\programdata\\datadog\\logs\\dogstatsd.log"

func init() {
	pd, err := winutil.GetProgramDataDir()
	if err == nil {
		defaultLogFile = filepath.Join(pd, "logs", "dogstatsd.log")
	} else {
		winutil.LogEventViewer(config.ServiceName, 0x8000000F, defaultLogFile)
	}
}
