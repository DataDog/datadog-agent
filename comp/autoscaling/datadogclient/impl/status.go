// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2021-present Datadog, Inc.

package datadogclientimpl

// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

import (
	"embed"
	"io"

	"gopkg.in/zorkian/go-datadog-api.v2"

	datadogclient "github.com/DataDog/datadog-agent/comp/autoscaling/datadogclient/def"
	"github.com/DataDog/datadog-agent/comp/core/status"
)

// statusProvider provides the functionality to populate the status output
type statusProvider struct {
	dc datadogclient.Component
}

//go:embed status_templates
var templatesFS embed.FS

// Name returns the name
func (p statusProvider) Name() string {
	return "External Metrics Endpoints"
}

// Section return the section
func (p statusProvider) Section() string {
	return "External Metrics Endpoints"
}

// JSON populates the status map
func (p statusProvider) JSON(_ bool, stats map[string]interface{}) error {
	populateStatus(p.dc, stats)

	return nil
}

// Text renders the text output
func (p statusProvider) Text(_ bool, buffer io.Writer) error {
	return status.RenderText(templatesFS, "externalmetrics.tmpl", buffer, getStatusInfo(p.dc))
}

// HTML renders the html output
func (p statusProvider) HTML(_ bool, _ io.Writer) error {
	return nil
}

func populateStatus(dc datadogclient.Component, stats map[string]interface{}) {
	stats["externalmetrics"] = getStatus(dc)
}

func getStatusInfo(dc datadogclient.Component) map[string]interface{} {
	stats := make(map[string]interface{})

	populateStatus(dc, stats)

	return stats
}

func getStatus(dcComp datadogclient.Component) map[string]interface{} {
	dc := (datadogclient.Component)(nil)
	status := make(map[string]interface{})
	if client, ok := dcComp.(*datadogClientWrapper); ok {
		dc = client.client
	} else {
		dc = dcComp
	}
	switch ddCl := dc.(type) {
	case *datadog.Client:
		// Can be nil if there's an error in NewDatadogClient()
		if ddCl == nil {
			return status
		}

		clientStatus := make(map[string]interface{})
		clientStatus["url"] = ddCl.GetBaseUrl()
		status["client"] = clientStatus
	case *datadogFallbackClient:
		if ddCl == nil {
			return status
		}

		status["lastUsedClient"] = ddCl.lastUsedClient
		clientsStatus := make([]map[string]interface{}, len(ddCl.clients))
		for i, individualClient := range ddCl.clients {
			clientsStatus[i] = make(map[string]interface{})
			clientsStatus[i]["url"] = individualClient.client.GetBaseUrl()
			clientsStatus[i]["lastQuerySucceeded"] = individualClient.lastQuerySucceeded
			if individualClient.lastFailure.IsZero() {
				clientsStatus[i]["lastFailure"] = "Never"
			} else {
				clientsStatus[i]["lastFailure"] = individualClient.lastFailure
			}
			if individualClient.lastSuccess.IsZero() {
				clientsStatus[i]["lastSuccess"] = "Never"
			} else {
				clientsStatus[i]["lastSuccess"] = individualClient.lastSuccess
			}
			if individualClient.lastFailure.IsZero() &&
				individualClient.lastSuccess.IsZero() {
				clientsStatus[i]["status"] = "Unknown"
			} else if individualClient.lastQuerySucceeded {
				clientsStatus[i]["status"] = "OK"
			} else {
				clientsStatus[i]["status"] = "Failed"
			}
			clientsStatus[i]["retryInterval"] = individualClient.retryInterval
		}
		status["clients"] = clientsStatus
	}

	return status
}
