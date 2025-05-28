// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

// Package statusimpl implements the status component interface
package statusimpl

import (
	"embed"
	"encoding/json"
	"fmt"
	"io"

	"go.uber.org/fx"

	"github.com/DataDog/datadog-agent/comp/core/config"
	ipc "github.com/DataDog/datadog-agent/comp/core/ipc/def"
	ipchttp "github.com/DataDog/datadog-agent/comp/core/ipc/httphelpers"
	"github.com/DataDog/datadog-agent/comp/core/status"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

type dependencies struct {
	fx.In

	Config config.Component
	Client ipc.HTTPClient
}

type provides struct {
	fx.Out

	StatusProvider status.InformationProvider
}

// Module defines the fx options for the status component.
func Module() fxutil.Module {
	return fxutil.Component(
		fx.Provide(newStatus))
}

type statusProvider struct {
	Config config.Component
	Client ipc.HTTPClient
}

func newStatus(deps dependencies) provides {
	return provides{
		StatusProvider: status.NewInformationProvider(statusProvider{
			Config: deps.Config,
			Client: deps.Client,
		}),
	}
}

//go:embed status_templates
var templatesFS embed.FS

// Name returns the name
func (s statusProvider) Name() string {
	return "APM Agent"
}

// Section return the section
func (s statusProvider) Section() string {
	return "APM Agent"
}

func (s statusProvider) getStatusInfo() map[string]interface{} {
	stats := make(map[string]interface{})

	values := s.populateStatus()

	stats["apmStats"] = values

	return stats
}

func (s statusProvider) populateStatus() map[string]interface{} {
	port := s.Config.GetInt("apm_config.debug.port")

	url := fmt.Sprintf("https://localhost:%d/debug/vars", port)
	resp, err := s.Client.Get(url, ipchttp.WithCloseConnection)
	if err != nil {
		return map[string]interface{}{
			"port":  port,
			"error": err.Error(),
		}
	}

	status := make(map[string]interface{})
	if err := json.Unmarshal(resp, &status); err != nil {
		return map[string]interface{}{
			"port":  port,
			"error": err.Error(),
		}
	}
	return status
}

// JSON populates the status map
func (s statusProvider) JSON(_ bool, stats map[string]interface{}) error {
	values := s.populateStatus()

	stats["apmStats"] = values

	return nil
}

// Text renders the text output
func (s statusProvider) Text(_ bool, buffer io.Writer) error {
	return status.RenderText(templatesFS, "traceagent.tmpl", buffer, s.getStatusInfo())
}

// HTML renders the html output
func (s statusProvider) HTML(_ bool, buffer io.Writer) error {
	return status.RenderHTML(templatesFS, "traceagentHTML.tmpl", buffer, s.getStatusInfo())
}
