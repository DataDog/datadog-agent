// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package config

import "github.com/DataDog/datadog-agent/pkg/util/winutil"

// DefaultConfPath points to the folder containing datadog.yaml
var DefaultConfPath = "c:\\programdata\\datadog"

func init() {
	pd, err := winutil.GetProgramDataDir()
	if err == nil {
		DefaultConfPath = pd
	} else {
		// log a message to the Windows event viewer indicating this error
		// occurred.
		winutil.LogEventViewer(config.ServiceName, 0x8000000F, DefaultConfPath)
	}
}
