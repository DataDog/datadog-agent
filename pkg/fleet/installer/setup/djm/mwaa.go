// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package djm contains data-jobs-monitoring installation logic
package djm

import (
	"os"

	"github.com/DataDog/datadog-agent/pkg/fleet/installer/setup/common"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

const (
	mwaaInjectorVersion     = "0.26.0-1"
	mwaaPythonTracerVersion = "2.9.2-1"
)

// SetupMwaa sets up the DJM environment on Dataproc
func SetupMwaa(s *common.Setup) error {

	namespace, ok := os.LookupEnv("AIRFLOW_ENV_NAME")
	if !ok {
		log.Error("environment variable AIRFLOW_ENV_NAME is not set")
		namespace = "default"
	}
	tracerEnvConfigMwaa := []common.InjectTracerConfigEnvVar{
		{
			Key:   "OPENLINEAGE_API_KEY",
			Value: s.Config.DatadogYAML.APIKey,
		},
		{
			Key:   "OPENLINEAGE_URL",
			Value: "https://data-obs-intake." + s.Config.DatadogYAML.Site,
		},
		{
			Key:   "AIRFLOW__OPENLINEAGE__CONFIG_PATH",
			Value: "",
		},
		{
			Key:   "AIRFLOW__OPENLINEAGE__DISABLED_FOR_OPERATORS",
			Value: "",
		},
		{
			Key:   "AIRFLOW__OPENLINEAGE__NAMESPACE",
			Value: namespace,
		},
	}

	s.Packages.Install(common.DatadogAPMInjectPackage, mwaaInjectorVersion)
	s.Packages.Install(common.DatadogAPMLibraryPythonPackage, mwaaPythonTracerVersion)

	s.Config.InjectTracerYAML.AdditionalEnvironmentVariables = tracerEnvConfigMwaa
	return nil
}
