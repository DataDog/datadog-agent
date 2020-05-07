// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

// +build kubeapiserver

package custommetrics

import (
	"fmt"

	"k8s.io/client-go/kubernetes"

	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes/apiserver/common"
)

// GetStatus returns status info for the Custom Metrics Server.
func GetStatus(apiCl kubernetes.Interface) map[string]interface{} {
	status := make(map[string]interface{})
	if !config.Datadog.GetBool("external_metrics_provider.enabled") {
		status["Disabled"] = "The external metrics provider is not enabled on the Cluster Agent"
		return status
	}

	if config.Datadog.GetBool("external_metrics_provider.use_datadogmetric_crd") {
		status["NoStatus"] = "External metrics provider uses DatadogMetric - Check status directly from Kubernetes with: `kubectl get datadogmetric`"
	}

	configMapName := GetConfigmapName()
	configMapNamespace := common.GetResourcesNamespace()
	status["Cmname"] = fmt.Sprintf("%s/%s", configMapNamespace, configMapName)

	store, err := NewConfigMapStore(apiCl, configMapNamespace, configMapName)
	if err != nil {
		status["StoreError"] = err.Error()
		return status
	}

	externalStatus := make(map[string]interface{})
	status["External"] = externalStatus

	bundle, err := store.GetMetrics()
	if err != nil {
		externalStatus["ListError"] = err.Error()
		return status
	}
	externalStatus["Metrics"] = bundle.External
	externalStatus["Total"] = len(bundle.External)
	valid := 0
	for _, metric := range bundle.External {
		if metric.Valid {
			valid++
		}
	}
	externalStatus["Valid"] = valid

	return status
}
