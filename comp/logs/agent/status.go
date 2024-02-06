// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package agent

import (
	"embed"
	"io"

	"github.com/DataDog/datadog-agent/comp/core/status"
	logsStatus "github.com/DataDog/datadog-agent/pkg/logs/status"
)

//go:embed status_templates
var templatesFS embed.FS

type statusProvider struct{}

func (p statusProvider) Name() string {
	return "Logs Agent"
}

func (p statusProvider) Section() string {
	return "Logs Agent"
}

func (p statusProvider) getStatusInfo(verbose bool) map[string]interface{} {
	stats := make(map[string]interface{})

	p.populateStatus(verbose, stats)

	return stats
}

func (p statusProvider) populateStatus(verbose bool, stats map[string]interface{}) {
	stats["logsStats"] = logsStatus.Get(verbose)
}

func (p statusProvider) JSON(verbose bool, stats map[string]interface{}) error {
	p.populateStatus(verbose, stats)

	return nil
}

func (p statusProvider) Text(verbose bool, buffer io.Writer) error {
	return status.RenderText(templatesFS, "logsagent.tmpl", buffer, p.getStatusInfo(verbose))
}

func (p statusProvider) HTML(verbose bool, buffer io.Writer) error {
	return status.RenderHTML(templatesFS, "logsagentHTML.tmpl", buffer, p.getStatusInfo(verbose))
}
