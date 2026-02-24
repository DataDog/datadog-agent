// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

// Package statusimpl implements the status component interface
package statusimpl

import (
	"embed"
	"encoding/json"
	"io"

	"go.uber.org/fx"

	"github.com/DataDog/datadog-agent/comp/core/config"
	ipc "github.com/DataDog/datadog-agent/comp/core/ipc/def"
	remoteagentregistry "github.com/DataDog/datadog-agent/comp/core/remoteagentregistry/def"
	"github.com/DataDog/datadog-agent/comp/core/status"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

type dependencies struct {
	fx.In

	Config config.Component
	Client ipc.HTTPClient
	RAR    remoteagentregistry.Component `optional:"true"`
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
	RAR    remoteagentregistry.Component
}

func newStatus(deps dependencies) provides {
	return provides{
		StatusProvider: status.NewInformationProvider(statusProvider{
			Config: deps.Config,
			Client: deps.Client,
			RAR:    deps.RAR,
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

	if s.RAR != nil {
		for _, agentStatus := range s.RAR.GetRegisteredAgentStatuses() {
			if agentStatus.Flavor != "trace_agent" {
				continue
			}
			if agentStatus.FailureReason != "" {
				return map[string]interface{}{
					"port":  port,
					"error": agentStatus.FailureReason,
				}
			}

			result := make(map[string]interface{}, len(agentStatus.MainSection))
			for k, v := range agentStatus.MainSection {
				var parsed interface{}
				if err := json.Unmarshal([]byte(v), &parsed); err == nil {
					result[k] = parsed
				} else {
					result[k] = v
				}
			}
			return result
		}
	}

	return map[string]interface{}{
		"port":  port,
		"error": "not running or unreachable",
	}
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
