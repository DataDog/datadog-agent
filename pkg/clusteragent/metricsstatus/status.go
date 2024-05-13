// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

// Package metricsstatus implements the core status provider interface
// Provides support for JSON and Text status information for cluster agent metrics
package metricsstatus

import (
	"embed"
	"io"

	"github.com/DataDog/datadog-agent/comp/core/status"
	"github.com/DataDog/datadog-agent/pkg/clusteragent/autoscaling/custommetrics"
	"github.com/DataDog/datadog-agent/pkg/clusteragent/autoscaling/externalmetrics"
	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes/apiserver"
)

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
		stats["custommetrics"] = custommetrics.GetStatus(apiCl.Cl)
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
