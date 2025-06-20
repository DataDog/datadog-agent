// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package remote contains the code to run diagnose to be sent as payload
package remote

import (
	"github.com/DataDog/datadog-agent/comp/core/config"
	diagnose "github.com/DataDog/datadog-agent/comp/core/diagnose/def"
	"github.com/DataDog/datadog-agent/pkg/diagnose/connectivity"
)

// Run the remote diagnose suite.
func Run(
	diagnoseConfig diagnose.Config,
	diagnoseComponent diagnose.Component,
	config config.Component,
) (*diagnose.Result, error) {

	remoteCheckSuites := diagnose.Suites{
		"internal-connectivity": func(_ diagnose.Config) []diagnose.Diagnosis {
			return connectivity.DiagnoseDatadogURL(config)
		},
	}
	return diagnoseComponent.RunLocalSuite(remoteCheckSuites, diagnoseConfig)
}
