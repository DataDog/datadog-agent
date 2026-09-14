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
	"fmt"
	"io"
	"time"

	compdef "github.com/DataDog/datadog-agent/comp/def"

	"github.com/DataDog/datadog-agent/comp/core/config"
	ipc "github.com/DataDog/datadog-agent/comp/core/ipc/def"
	ipchttp "github.com/DataDog/datadog-agent/comp/core/ipc/httphelpers"
	corestatus "github.com/DataDog/datadog-agent/comp/core/status"
	tracestatus "github.com/DataDog/datadog-agent/comp/trace/status/def"
	pbcore "github.com/DataDog/datadog-agent/pkg/proto/pbgo/core"
)

// Requires defines the dependencies of the status component.
type Requires struct {
	Config config.Component
	Client ipc.HTTPClient
}

// Provides defines the output of the status component.
type Provides struct {
	compdef.Out

	Comp           tracestatus.Component
	StatusProvider corestatus.InformationProvider
}

type statusProvider struct {
	pbcore.UnimplementedStatusProviderServer

	Config config.Component
	Client ipc.HTTPClient
}

// NewComponent creates a new trace agent status component.
func NewComponent(reqs Requires) Provides {
	provider := &statusProvider{
		Config: reqs.Config,
		Client: reqs.Client,
	}

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

func (s statusProvider) getStatusInfo(ctx context.Context) map[string]interface{} {
	stats := make(map[string]interface{})

	values := s.populateStatus(ctx)

	stats["apmStats"] = values

	return stats
}

func (s statusProvider) populateStatus(ctx context.Context) map[string]interface{} {
	port := s.Config.GetInt("apm_config.debug.port")
	timeout := s.Config.GetDuration("server_timeout") * time.Second

	url := fmt.Sprintf("https://localhost:%d/debug/vars", port)
	resp, err := s.Client.Get(url, ipchttp.WithContext(ctx), ipchttp.WithCloseConnection, ipchttp.WithTimeout(timeout), ipchttp.WithoutAuthToken)
	if err != nil {
		return map[string]interface{}{
			"port":  port,
			"error": err.Error(),
		}
	}

	statusMap := make(map[string]interface{})
	if err := json.Unmarshal(resp, &statusMap); err != nil {
		return map[string]interface{}{
			"port":  port,
			"error": err.Error(),
		}
	}
	return statusMap
}

// JSON populates the status map
func (s statusProvider) JSON(_ bool, stats map[string]interface{}) error {
	values := s.populateStatus(context.Background())

	stats["apmStats"] = values

	return nil
}

// Text renders the text output
func (s statusProvider) Text(_ bool, buffer io.Writer) error {
	return s.renderText(context.Background(), buffer)
}

func (s statusProvider) renderText(ctx context.Context, buffer io.Writer) error {
	return corestatus.RenderText(templatesFS, "traceagent.tmpl", buffer, s.getStatusInfo(ctx))
}

// HTML renders the html output
func (s statusProvider) HTML(_ bool, buffer io.Writer) error {
	return corestatus.RenderHTML(templatesFS, "traceagentHTML.tmpl", buffer, s.getStatusInfo(context.Background()))
}

// GetStatusDetails returns the Trace Agent status rendered as text.
func (s statusProvider) GetStatusDetails(ctx context.Context, _ *pbcore.GetStatusDetailsRequest) (*pbcore.GetStatusDetailsResponse, error) {
	var details bytes.Buffer
	if err := s.renderText(ctx, &details); err != nil {
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
	}, nil
}
