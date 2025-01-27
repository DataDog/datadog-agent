// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

// Package statusimpl implements the status component interface
package statusimpl

import (
	"embed"
	"encoding/json"
	"fmt"
	"gopkg.in/yaml.v3"
	"io"
	"net/http"

	"github.com/DataDog/datadog-agent/comp/core/config"
	"github.com/DataDog/datadog-agent/comp/core/status"
	ddflareextension "github.com/DataDog/datadog-agent/comp/otelcol/ddflareextension/def"
	apiutil "github.com/DataDog/datadog-agent/pkg/api/util"
	"github.com/DataDog/datadog-agent/pkg/util/prometheus"
)

//go:embed status_templates
var templatesFS embed.FS

// Dependencies defines the dependencies of the status component.
type Dependencies struct {
	Config config.Component
}

// Provider contains components provided by status constructor.
type Provides struct {
	StatusProvider status.InformationProvider
}

type statusProvider struct {
	Config         config.Component
	client         *http.Client
	receiverStatus map[string]interface{}
	exporterStatus map[string]interface{}
}

type prometheusRuntimeConfig struct {
	Service struct {
		Telemetry struct {
			Metrics struct {
				Readers []struct {
					Pull struct {
						Exporter struct {
							Prometheus struct {
								Host string
								Port int
							}
						}
					}
				}
			}
		}
	}
}

// NewComponent creates a new status component.
func NewComponent(deps Dependencies) Provides {
	return Provides{
		StatusProvider: status.NewInformationProvider(statusProvider{
			Config: deps.Config,
			client: apiutil.GetClient(false),
			receiverStatus: map[string]interface{}{
				"spans":           0.0,
				"metrics":         0.0,
				"logs":            0.0,
				"refused_spans":   0.0,
				"refused_metrics": 0.0,
				"refused_logs":    0.0,
			},
			exporterStatus: map[string]interface{}{
				"spans":          0.0,
				"metrics":        0.0,
				"logs":           0.0,
				"failed_spans":   0.0,
				"failed_metrics": 0.0,
				"failed_logs":    0.0,
			},
		}),
	}
}

// Name returns the name
func (s statusProvider) Name() string {
	return "OTel Agent"
}

// Section return the section
func (s statusProvider) Section() string {
	return "OTel Agent"
}

func (s statusProvider) getStatusInfo() map[string]interface{} {
	statusInfo := make(map[string]interface{})

	values := s.populateStatus()

	statusInfo["otelAgent"] = values

	return statusInfo
}

func getPrometheusURL(extensionResp ddflareextension.Response) (string, error) {
	var runtimeConfig prometheusRuntimeConfig
	if err := yaml.Unmarshal([]byte(extensionResp.RuntimeConfig), &runtimeConfig); err != nil {
		return "", err
	}
	prometheusHost := "localhost"
	prometheusPort := 8888
	for _, reader := range runtimeConfig.Service.Telemetry.Metrics.Readers {
		prometheusEndpoint := reader.Pull.Exporter.Prometheus
		if prometheusEndpoint.Host != "" && prometheusEndpoint.Port != 0 {
			prometheusHost = prometheusEndpoint.Host
			prometheusPort = prometheusEndpoint.Port
		}
	}
	return fmt.Sprintf("http://%v:%d/metrics", prometheusHost, prometheusPort), nil
}

func (s statusProvider) populatePrometheusStatus(prometheusURL string) error {
	resp, err := apiutil.DoGet(s.client, prometheusURL, apiutil.CloseConnection)
	if err != nil {
		return err
	}
	metrics, err := prometheus.ParseMetrics(resp)
	if err != nil {
		return err
	}

	for _, m := range metrics {
		value := m.Samples[0].Value
		switch m.Name {
		case "otelcol_receiver_accepted_spans":
			s.receiverStatus["spans"] = value
		case "otelcol_receiver_accepted_metric_points":
			s.receiverStatus["metrics"] = value
		case "otelcol_receiver_accepted_log_records":
			s.receiverStatus["logs"] = value
		case "otelcol_receiver_refused_spans":
			s.receiverStatus["refused_spans"] = value
		case "otelcol_receiver_refused_metric_points":
			s.receiverStatus["refused_metrics"] = value
		case "otelcol_receiver_refused_log_records":
			s.receiverStatus["refused_logs"] = value
		case "otelcol_exporter_sent_spans":
			s.exporterStatus["spans"] = value
		case "otelcol_exporter_sent_metric_points":
			s.exporterStatus["metrics"] = value
		case "otelcol_exporter_sent_log_records":
			s.exporterStatus["logs"] = value
		case "otelcol_exporter_send_failed_spans":
			s.exporterStatus["failed_spans"] = value
		case "otelcol_exporter_send_failed_metric_points":
			s.exporterStatus["failed_metrics"] = value
		case "otelcol_exporter_send_failed_log_records":
			s.exporterStatus["failed_logs"] = value
		}
	}
	return nil
}

func (s statusProvider) populateStatus() map[string]interface{} {
	extensionURL := s.Config.GetString("otelcollector.extension_url")
	resp, err := apiutil.DoGet(s.client, extensionURL, apiutil.CloseConnection)
	if err != nil {
		return map[string]interface{}{
			"url":   extensionURL,
			"error": err.Error(),
		}
	}
	var extensionResp ddflareextension.Response
	if err = json.Unmarshal(resp, &extensionResp); err != nil {
		return map[string]interface{}{
			"url":   extensionURL,
			"error": err.Error(),
		}
	}
	prometheusURL, err := getPrometheusURL(extensionResp)
	if err != nil {
		return map[string]interface{}{
			"url":   extensionURL,
			"error": err.Error(),
		}
	}
	err = s.populatePrometheusStatus(prometheusURL)
	if err != nil {
		return map[string]interface{}{
			"url":   prometheusURL,
			"error": err.Error(),
		}
	}
	return map[string]interface{}{
		"agentVersion":     extensionResp.AgentVersion,
		"collectorVersion": extensionResp.ExtensionVersion,
		"receiver":         s.receiverStatus,
		"exporter":         s.exporterStatus,
	}
}

// JSON populates the status map
func (s statusProvider) JSON(_ bool, stats map[string]interface{}) error {
	values := s.populateStatus()

	stats["otelAgent"] = values

	return nil
}

// Text renders the text output
func (s statusProvider) Text(_ bool, buffer io.Writer) error {
	return status.RenderText(templatesFS, "otelagent.tmpl", buffer, s.getStatusInfo())
}

// HTML renders the html output
func (s statusProvider) HTML(_ bool, buffer io.Writer) error {
	return status.RenderHTML(templatesFS, "otelagentHTML.tmpl", buffer, s.getStatusInfo())
}
