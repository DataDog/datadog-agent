// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package mode

import (
	serverlessLog "github.com/DataDog/datadog-agent/cmd/serverless-init/log"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"os"
)

const (
	loggerNameInit    = "SERVERLESS_INIT"
	loggerNameSidecar = "SERVERLESS_SIDECAR"
)

func DetectMode() (string, func(logConfig *serverlessLog.Config)) {

	envToSet := map[string]string{
		"DD_REMOTE_CONFIGURATION_ENABLED": "false",
		"DD_HOSTNAME":                     "none",
		"DD_APM_ENABLED":                  "true",
		"DD_TRACE_ENABLED":                "true",
		"DD_LOGS_ENABLED":                 "true",
	}

	defaultModeRunner := RunInit
	defaultLoggerName := loggerNameInit

	if len(os.Args) == 1 {
		log.Infof("No arguments provided, launching in Sidecar mode")
		defaultModeRunner = RunSidecar
		defaultLoggerName = loggerNameSidecar
		envToSet["DD_APM_NON_LOCAL_TRAFFIC"] = "true"
		envToSet["DD_DOGSTATSD_NON_LOCAL_TRAFFIC"] = "true"
	} else {
		log.Infof("Arguments provided, launching in Init mode")
	}

	setupEnv(envToSet)

	return defaultLoggerName, defaultModeRunner
}

func setupEnv(envToSet map[string]string) {
	for envName, envVal := range envToSet {
		if val, set := os.LookupEnv(envName); !set {
			os.Setenv(envName, envVal)
		} else {
			log.Debugf("%s already set with %s, skipping setting it", envName, val)
		}
	}
}
