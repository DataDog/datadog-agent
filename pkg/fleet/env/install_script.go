// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package env

const (
	envApmInstrumentationEnabled = "DD_APM_INSTRUMENTATION_ENABLED"
)

const (
	// APMInstrumentationEnabledAll enables APM instrumentation for all containers.
	APMInstrumentationEnabledAll = "all"
	// APMInstrumentationEnabledDocker enables APM instrumentation for Docker containers.
	APMInstrumentationEnabledDocker = "docker"
	// APMInstrumentationEnabledHost enables APM instrumentation for the host.
	APMInstrumentationEnabledHost = "host"
)

// InstallScriptEnv contains the environment variables for the install script.
type InstallScriptEnv struct {
	APMInstrumentationEnabled string
}

func installScriptEnvFromEnv() InstallScriptEnv {
	return InstallScriptEnv{
		APMInstrumentationEnabled: getEnvOrDefault(envApmInstrumentationEnabled, ""),
	}
}
