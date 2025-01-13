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

// SetupMwaa sets up the DJM environment on Dataproc
func SetupMwaa(s *common.Setup) error {

	err := os.Setenv("OPENLINEAGE_API_KEY", s.Config.DatadogYAML.APIKey)
	if err != nil {
		return err
	}

	err = os.Setenv("OPENLINEAGE_URL", "https://data-obs-intake."+s.Config.DatadogYAML.Site)
	if err != nil {
		return err
	}

	err = os.Setenv("AIRFLOW__OPENLINEAGE__CONFIG_PATH", "")
	if err != nil {
		return err
	}

	err = os.Setenv("AIRFLOW__OPENLINEAGE__DISABLED_FOR_OPERATORS", "")
	if err != nil {
		return err
	}

	namespace, ok := os.LookupEnv("AIRFLOW_ENV_NAME")
	if !ok {
		log.Error("environment variable AIRFLOW_ENV_NAME is not set")
		namespace = "default"
	}
	err = os.Setenv("AIRFLOW__OPENLINEAGE__NAMESPACE", namespace)
	if err != nil {
		return err
	}

	return nil
}
