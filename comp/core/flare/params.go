// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package flare

import flaredef "github.com/DataDog/datadog-agent/comp/core/flare/def"

// Params defines the parameters for the flare component.
type Params = flaredef.Params

// NewLocalParams returns parameters to initialize a local flare component. Local flares are meant to be created by
// the CLI process instead of the main Agent one.
func NewLocalParams(distPath string, pythonChecksPath string, defaultLogFile string, defaultJMXLogFile string, defaultDogstatsdLogFile string, defaultStreamlogsLogFile string) Params {
	return flaredef.NewLocalParams(distPath, pythonChecksPath, defaultLogFile, defaultJMXLogFile, defaultDogstatsdLogFile, defaultStreamlogsLogFile)
}

// NewParams returns parameters to initialize a non local flare component.
func NewParams(distPath string, pythonChecksPath string, defaultLogFile string, defaultJMXLogFile string, defaultDogstatsdLogFile string, defaultStreamlogsLogFile string) Params {
	return flaredef.NewParams(distPath, pythonChecksPath, defaultLogFile, defaultJMXLogFile, defaultDogstatsdLogFile, defaultStreamlogsLogFile)
}
