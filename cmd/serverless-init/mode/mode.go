// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !windows

package mode

import (
	"os"

	serverlessLog "github.com/DataDog/datadog-agent/cmd/serverless-init/log"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// Conf contains the configuration for the mode in which the serverless-init agent should run
type Conf struct {
	LoggerName     string
	Runner         func(logConfig *serverlessLog.Config)
	TagVersionMode string
	EnvDefaults    map[string]string
}

const (
	loggerNameInit    = "SERVERLESS_INIT"
	loggerNameSidecar = "SERVERLESS_SIDECAR"
)

// DetectMode detects the mode in which the serverless agent should run
func DetectMode() Conf {

	envToSet := map[string]string{
		"DD_REMOTE_CONFIGURATION_ENABLED":      "false",
		"DD_HOSTNAME":                          "none",
		"DD_APM_ENABLED":                       "true",
		"DD_TRACE_ENABLED":                     "true",
		"DD_LOGS_ENABLED":                      "true",
		"DD_INSTRUMENTATION_TELEMETRY_ENABLED": "false",
	}

	if len(os.Args) == 1 {
		log.Infof("No arguments provided, launching in Sidecar mode")
		envToSet["DD_APM_NON_LOCAL_TRAFFIC"] = "true"
		envToSet["DD_DOGSTATSD_NON_LOCAL_TRAFFIC"] = "true"
		return Conf{
			LoggerName:     loggerNameSidecar,
			Runner:         RunSidecar,
			TagVersionMode: "sidecar",
			EnvDefaults:    envToSet,
		}
	}
	log.Infof("Arguments provided, launching in Init mode")
	return Conf{
		LoggerName:     loggerNameInit,
		Runner:         RunInit,
		TagVersionMode: "init",
		EnvDefaults:    envToSet,
	}
}
