// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

package providers

import (
	"github.com/DataDog/datadog-agent/pkg/autodiscovery/common"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// PrometheusConfigProvider can be shared between Prometheus Pods and Prometheus Services
type PrometheusConfigProvider struct {
	checks []*common.PrometheusCheck
}

// setupConfigs reads and initializes the checks from the configuration
// It defines a default openmetrics instances with default AD if the checks configuration is empty
func (p *PrometheusConfigProvider) setupConfigs() error {
	checks, err := common.ReadPrometheusChecksConfig()
	if err != nil {
		return err
	}

	if len(checks) == 0 {
		log.Info("The 'prometheus_scrape.checks' configuration is empty, a default openmetrics check configuration will be used")
		p.checks = []*common.PrometheusCheck{common.DefaultPrometheusCheck}
		return nil
	}

	for i, check := range checks {
		if err := check.Init(); err != nil {
			log.Errorf("Ignoring check configuration (# %d): %v", i+1, err)
			continue
		}
		p.checks = append(p.checks, check)
	}

	return nil
}
