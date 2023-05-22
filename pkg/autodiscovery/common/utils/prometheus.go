// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubelet || clusterchecks || kubeapiserver

package utils

import (
	"encoding/json"

	"github.com/DataDog/datadog-agent/pkg/autodiscovery/common/types"
	"github.com/DataDog/datadog-agent/pkg/autodiscovery/integration"
	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

const (
	openmetricsCheckName  = "openmetrics"
	openmetricsInitConfig = "{}"
)

// buildInstances generates check config instances based on the Prometheus config and the object annotations
// The second returned value is true if more than one instance is found
func buildInstances(pc *types.PrometheusCheck, annotations map[string]string, namespacedName string) ([]integration.Data, bool) {
	openmetricsVersion := config.Datadog.GetInt("prometheus_scrape.version")

	instances := []integration.Data{}
	for k, v := range pc.AD.KubeAnnotations.Incl {
		if annotations[k] == v {
			log.Debugf("'%s' matched the annotation '%s=%s' to schedule an openmetrics check", namespacedName, k, v)
			for _, instance := range pc.Instances {
				instanceValues := *instance
				if instanceValues.PrometheusURL == "" && instanceValues.OpenMetricsEndpoint == "" {
					switch openmetricsVersion {
					case 1:
						instanceValues.PrometheusURL = types.BuildURL(annotations)
						if len(instanceValues.Metrics) == len(types.DefaultPrometheusCheck.Instances[0].Metrics) &&
							&instanceValues.Metrics[0] == &types.DefaultPrometheusCheck.Instances[0].Metrics[0] {
							instanceValues.Metrics = types.OpenmetricsDefaultMetricsV1
						}
					case 2:
						instanceValues.OpenMetricsEndpoint = types.BuildURL(annotations)
						if len(instanceValues.Metrics) == len(types.DefaultPrometheusCheck.Instances[0].Metrics) &&
							&instanceValues.Metrics[0] == &types.DefaultPrometheusCheck.Instances[0].Metrics[0] {
							instanceValues.Metrics = types.OpenmetricsDefaultMetricsV2
						}
					}
				}
				instanceJSON, err := json.Marshal(instanceValues)
				if err != nil {
					log.Warnf("Error processing prometheus configuration: %v", err)
					continue
				}
				instances = append(instances, instanceJSON)
			}
			return instances, len(instances) > 0
		}
	}

	return instances, false
}
