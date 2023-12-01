// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package providers

import (
	"github.com/DataDog/datadog-agent/pkg/autodiscovery/common/types"
	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// getPrometheusConfigs reads and initializes the openmetrics checks from the configuration
// It defines a default openmetrics instances with default AD if the checks configuration is empty
func getPrometheusConfigs() ([]*types.PrometheusCheck, error) {
	checks, err := types.PrometheusScrapeChecksTransformer(config.Datadog.GetString("prometheus_scrape.checks"))
	if err != nil {
		return []*types.PrometheusCheck{}, err
	}

	if len(checks) == 0 {
		log.Info("The 'prometheus_scrape.checks' configuration is empty, a default openmetrics check configuration will be used")
		return []*types.PrometheusCheck{types.DefaultPrometheusCheck}, nil
	}

	validChecks := []*types.PrometheusCheck{}
	for i, check := range checks {
		if err := check.Init(config.Datadog.GetInt("prometheus_scrape.version")); err != nil {
			log.Errorf("Ignoring check configuration (# %d): %v", i+1, err)
			continue
		}
		validChecks = append(validChecks, check)
	}

	return validChecks, nil
}
