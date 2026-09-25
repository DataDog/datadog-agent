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
	"expvar"
	"io"

	compdef "github.com/DataDog/datadog-agent/comp/def"

	corestatus "github.com/DataDog/datadog-agent/comp/core/status"
	tracestatus "github.com/DataDog/datadog-agent/comp/trace/status/def"
	pbcore "github.com/DataDog/datadog-agent/pkg/proto/pbgo/core"
)

// Provides defines the output of the status component.
type Provides struct {
	compdef.Out

	Comp           tracestatus.Component
	StatusProvider corestatus.InformationProvider
}

type statusProvider struct {
	pbcore.UnimplementedStatusProviderServer
}

// NewComponent creates a new trace agent status component.
func NewComponent() Provides {
	provider := &statusProvider{}

	return Provides{
		Comp:           provider,
		StatusProvider: corestatus.NewInformationProvider(provider),
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

func (s statusProvider) getStatusInfo() (map[string]interface{}, []byte, error) {
	values := make(map[string]json.RawMessage)
	// Read the same published values as /debug/vars, including the scrubbed config.
	expvar.Do(func(kv expvar.KeyValue) {
		values[kv.Key] = json.RawMessage(kv.Value.String())
	})
	// InitInfo publishes config last, after all fields used by the templates.
	if _, ready := values["config"]; !ready {
		values = map[string]json.RawMessage{
			"error": json.RawMessage(`"Trace Agent is not initialized yet"`),
		}
	}
	payload, err := json.Marshal(map[string]interface{}{"apmStats": values})
	if err != nil {
		return nil, nil, err
	}

	// Templates need decoded maps and float64 numbers; keep the original JSON
	// for the RAR payload so large counters don't lose precision.
	var stats map[string]interface{}
	if err := json.Unmarshal(payload, &stats); err != nil {
		return nil, nil, err
	}
	return stats, payload, nil
}

// JSON populates the status map
func (s statusProvider) JSON(_ bool, stats map[string]interface{}) error {
	values, _, err := s.getStatusInfo()
	if err != nil {
		return err
	}
	stats["apmStats"] = values["apmStats"]
	return nil
}

// Text renders the text output
func (s statusProvider) Text(_ bool, buffer io.Writer) error {
	stats, _, err := s.getStatusInfo()
	if err != nil {
		return err
	}
	return s.renderTextFromStatus(stats, buffer)
}

func (s statusProvider) renderTextFromStatus(stats map[string]interface{}, buffer io.Writer) error {
	return corestatus.RenderText(templatesFS, "traceagent.tmpl", buffer, stats)
}

// HTML renders the html output
func (s statusProvider) HTML(_ bool, buffer io.Writer) error {
	stats, _, err := s.getStatusInfo()
	if err != nil {
		return err
	}
	return corestatus.RenderHTML(templatesFS, "traceagentHTML.tmpl", buffer, stats)
}

// GetStatusDetails returns the Trace Agent status rendered as text.
func (s statusProvider) GetStatusDetails(_ context.Context, _ *pbcore.GetStatusDetailsRequest) (*pbcore.GetStatusDetailsResponse, error) {
	stats, payload, err := s.getStatusInfo()
	if err != nil {
		return nil, err
	}

	var details bytes.Buffer
	if err := s.renderTextFromStatus(stats, &details); err != nil {
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
