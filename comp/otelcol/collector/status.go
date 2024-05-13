// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build otlp

package collector

import (
	"embed"
	"io"

	"github.com/DataDog/datadog-agent/comp/core/status"
	"github.com/DataDog/datadog-agent/comp/otelcol/otlp"
)

//go:embed status_templates
var templatesFS embed.FS

func (c *collector) getStatusInfo() map[string]interface{} {
	stats := make(map[string]interface{})

	c.populateStatus(stats)

	return stats
}

func (c *collector) populateStatus(stats map[string]interface{}) {
	otlpStatus := make(map[string]interface{})
	otlpIsEnabled := otlp.IsEnabled(c.deps.Config)

	var otlpCollectorStatus otlp.CollectorStatus

	if otlpIsEnabled {
		otlpCollectorStatus = c.Status()
	} else {
		otlpCollectorStatus = otlp.CollectorStatus{Status: "Not running", ErrorMessage: ""}
	}
	otlpStatus["otlpStatus"] = otlpIsEnabled
	otlpStatus["otlpCollectorStatus"] = otlpCollectorStatus.Status
	otlpStatus["otlpCollectorStatusErr"] = otlpCollectorStatus.ErrorMessage

	stats["otlp"] = otlpStatus
}

// Name returns the name
func (c *collector) Name() string {
	return "OTLP"
}

// Name returns the section
func (c *collector) Section() string {
	return "OTLP"
}

// JSON populates the status map
func (c *collector) JSON(_ bool, stats map[string]interface{}) error {
	c.populateStatus(stats)

	return nil
}

// Text renders the text output
func (c *collector) Text(_ bool, buffer io.Writer) error {
	return status.RenderText(templatesFS, "otlp.tmpl", buffer, c.getStatusInfo())
}

// HTML renders the html output
func (c *collector) HTML(_ bool, _ io.Writer) error {
	return nil
}
