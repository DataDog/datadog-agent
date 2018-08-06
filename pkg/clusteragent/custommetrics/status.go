// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018 Datadog, Inc.

// +build kubeapiserver

package custommetrics

import "github.com/DataDog/datadog-agent/pkg/util/kubernetes/apiserver"

// GetStatus returns status info for the Custom Metrics Provider.
func GetStatus() map[string]interface{} {
	status := make(map[string]interface{})
	apiCl, err := apiserver.GetAPIClient()
	if err != nil {
		status["Error"] = err.Error()
		return status
	}

	datadogHPAConfigMap := GetHPAConfigmapName()
	status["Cmname"] = datadogHPAConfigMap

	store, err := NewConfigMapStore(apiCl.Cl, apiserver.GetResourcesNamespace(), datadogHPAConfigMap)
	if err != nil {
		status["StoreError"] = err.Error()
		return status
	}

	externalStatus := make(map[string]interface{})
	status["External"] = externalStatus

	externalMetrics, err := store.ListAllExternalMetricValues()
	if err != nil {
		externalStatus["ListError"] = err.Error()
		return status
	}

	externalStatus["Metrics"] = externalMetrics

	externalStatus["Total"] = len(externalMetrics)

	valid := 0
	for _, metric := range externalMetrics {
		if metric.Valid {
			valid += 1
		}
	}

	externalStatus["Valid"] = valid

	return status
}
