// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

// Package status implements comp/core/status provider interface
package status

import (
	"embed"
	"fmt"
	"io"

	"k8s.io/client-go/kubernetes"

	"github.com/DataDog/datadog-agent/comp/core/status"
	"github.com/DataDog/datadog-agent/pkg/clusteragent/custommetrics"
	"github.com/DataDog/datadog-agent/pkg/clusteragent/externalmetrics"
	"github.com/DataDog/datadog-agent/pkg/config"

	"github.com/DataDog/datadog-agent/pkg/util/kubernetes/apiserver"
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
		return status
	}

	configMapName := custommetrics.GetConfigmapName()
	configMapNamespace := common.GetResourcesNamespace()
	status["Cmname"] = fmt.Sprintf("%s/%s", configMapNamespace, configMapName)

	store, err := custommetrics.NewConfigMapStore(apiCl, configMapNamespace, configMapName)
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

// Provider provides the functionality to populate the status output
type Provider struct{}

//go:embed status_templates
var templatesFS embed.FS

// Name returns the name
func (Provider) Name() string {
	return "Custom Metrics Server"
}

// Section return the section
func (Provider) Section() string {
	return "Custom Metrics Server"
}

// JSON populates the status map
func (Provider) JSON(_ bool, stats map[string]interface{}) error {
	populateStatus(stats)

	return nil
}

// Text renders the text output
func (Provider) Text(_ bool, buffer io.Writer) error {
	return status.RenderText(templatesFS, "custommetrics.tmpl", buffer, getStatusInfo())
}

// HTML renders the html output
func (Provider) HTML(_ bool, _ io.Writer) error {
	return nil
}

func populateStatus(stats map[string]interface{}) {
	apiCl, apiErr := apiserver.GetAPIClient()
	if apiErr != nil {
		stats["custommetrics"] = map[string]string{"Error": apiErr.Error()}
	} else {
		stats["custommetrics"] = GetStatus(apiCl.Cl)
	}

	if config.Datadog.GetBool("external_metrics_provider.use_datadogmetric_crd") {
		stats["externalmetrics"] = externalmetrics.GetStatus()
	} else {
		stats["externalmetrics"] = apiserver.GetStatus()
	}
}

func getStatusInfo() map[string]interface{} {
	stats := make(map[string]interface{})

	populateStatus(stats)

	return stats
}
