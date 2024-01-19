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

func (a *agent) Name() string {
	return "Logs Agent"
}

func (a *agent) Section() string {
	return "Logs Agent"
}

func (a *agent) getStatusInfo(verbose bool) map[string]interface{} {
	stats := make(map[string]interface{})

	a.populateStatus(verbose, stats)

	return stats
}

func (a *agent) populateStatus(verbose bool, stats map[string]interface{}) {
	stats["logsStats"] = logsStatus.Get(verbose)
}

func (a *agent) JSON(verbose bool, stats map[string]interface{}) error {
	a.populateStatus(verbose, stats)

	return nil
}

func (a *agent) Text(verbose bool, buffer io.Writer) error {
	return status.RenderText(templatesFS, "logsagent.tmpl", buffer, a.getStatusInfo(verbose))
}

func (a *agent) HTML(verbose bool, buffer io.Writer) error {
	return status.RenderHTML(templatesFS, "logsagentHTML.tmpl", buffer, a.getStatusInfo(verbose))
}
