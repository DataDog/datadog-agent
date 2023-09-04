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
				// The `PrometheusCheck` config may come from two sources:
				// Either it comes from the `DD_PROMETHEUS_SCRAPE_CHECKS` environment variable.
				//   In this case, it has been parsed by JSON decoder
				//     In this case, dictionaries have been decoded as `map[string]interface{}`
				//     and everything is fine
				// Or it comes from the `prometheus_scrape.checks` property inside the `datadog.yaml` file.
				//   In this case, it has been parsed by YAML decoder
				//     In this case, dictionaries have been decoded as `map[interface{}]interface{}`
				//     and this will be problematic as this type cannot be encoded by a JSON encoder
				//
				// So, let’s convert all `map[interface{}]interface{}` to `map[string]interface{}`
				instanceValues.Metrics = convertMap(instanceValues.Metrics).([]interface{})
				instanceValues.IgnoreMetricsByLabels = convertMap(instanceValues.IgnoreMetricsByLabels).(map[string]interface{})
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

func convertMap(x interface{}) interface{} {
	switch elem := x.(type) {

	case []interface{}:
		for i, v := range elem {
			elem[i] = convertMap(v)
		}
		return elem

	case map[string]interface{}:
		for k, v := range elem {
			elem[k] = convertMap(v)
		}
		return elem

	case map[interface{}]interface{}:
		out := make(map[string]interface{})
		for k, v := range elem {
			if s, ok := k.(string); ok {
				out[s] = convertMap(v)
			} else {
				log.Errorf("Error processing prometheus configuration: map keys should be strings in %#v", elem)
			}
		}
		return out

	default:
		return elem
	}
}
