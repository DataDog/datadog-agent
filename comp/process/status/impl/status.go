// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

// Package statusimpl implements the status component interface
package statusimpl

import (
	"bytes"
	"context"
	"embed"
	"encoding/json"
	"io"

	"github.com/DataDog/datadog-agent/comp/core/config"
	"github.com/DataDog/datadog-agent/comp/core/hostname/hostnameinterface/def"
	corestatus "github.com/DataDog/datadog-agent/comp/core/status"
	compdef "github.com/DataDog/datadog-agent/comp/def"
	processstatus "github.com/DataDog/datadog-agent/comp/process/status/def"
	processStatus "github.com/DataDog/datadog-agent/pkg/process/util/status"
	pbcore "github.com/DataDog/datadog-agent/pkg/proto/pbgo/core"
)

type dependencies struct {
	compdef.In

	Config   config.Component
	Hostname hostnameinterface.Component
}

// Provides defines the output dependencies of the status component.
type Provides struct {
	compdef.Out

	Comp           processstatus.Component
	StatusProvider corestatus.InformationProvider
}

// NewComponent creates the status component.
func NewComponent(deps dependencies) Provides {
	provider := &statusProvider{
		config:   deps.Config,
		hostname: deps.Hostname,
	}

	return Provides{
		Comp:           provider,
		StatusProvider: corestatus.NewInformationProvider(provider),
	}
}

type statusProvider struct {
	pbcore.UnimplementedStatusProviderServer

	config   config.Component
	hostname hostnameinterface.Component
}

//go:embed status_templates
var templatesFS embed.FS

// Name returns the name
func (s statusProvider) Name() string {
	return "Process Agent"
}

// Section return the section
func (s statusProvider) Section() string {
	return "Process Agent"
}

func (s statusProvider) getStatusInfo() map[string]interface{} {
	return map[string]interface{}{"processAgentStatus": s.populateStatus()}
}

func (s statusProvider) populateStatus() map[string]interface{} {
	status := make(map[string]interface{})
	agentStatus, err := processStatus.GetLocalStatus(s.config, s.hostname)
	if err != nil {
		status["error"] = err.Error()
		return status
	}

	bytes, err := json.Marshal(agentStatus)
	if err != nil {
		return map[string]interface{}{
			"error": err.Error(),
		}
	}

	err = json.Unmarshal(bytes, &status)
	if err != nil {
		return map[string]interface{}{
			"error": err.Error(),
		}
	}

	return status
}

// JSON populates the status map
func (s statusProvider) JSON(_ bool, stats map[string]interface{}) error {
	stats["processAgentStatus"] = s.populateStatus()

	return nil
}

// Text renders the text output
func (s statusProvider) Text(_ bool, buffer io.Writer) error {
	return s.renderTextFromStatus(s.getStatusInfo(), buffer)
}

func (s statusProvider) renderTextFromStatus(stats map[string]interface{}, buffer io.Writer) error {
	return corestatus.RenderText(templatesFS, "processagent.tmpl", buffer, stats)
}

// HTML renders the html output
func (s statusProvider) HTML(_ bool, _ io.Writer) error {
	return nil
}

// GetStatusDetails returns the Process Agent status rendered as text.
func (s statusProvider) GetStatusDetails(_ context.Context, _ *pbcore.GetStatusDetailsRequest) (*pbcore.GetStatusDetailsResponse, error) {
	stats := s.getStatusInfo()

	var details bytes.Buffer
	if err := s.renderTextFromStatus(stats, &details); err != nil {
		return nil, err
	}
	payload, err := json.Marshal(stats)
	if err != nil {
		return nil, err
	}

	return &pbcore.GetStatusDetailsResponse{
		NamedSections: map[string]*pbcore.StatusSection{
			"Details": {
				Fields: map[string]string{
					"": details.String(),
				},
			},
		},
		JsonPayload: payload,
	}, nil
}
