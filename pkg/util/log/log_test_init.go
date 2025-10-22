// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build test

package log

import (
	"os"
)

func init() {
	lvl := DebugLvl
	if level, ok := os.LookupEnv("DD_LOG_LEVEL"); ok {
		logLevel, err := ValidateLogLevel(level)
		if err == nil {
			lvl = logLevel
		}
	}

	SetupLogger(Default(), lvl)
}
