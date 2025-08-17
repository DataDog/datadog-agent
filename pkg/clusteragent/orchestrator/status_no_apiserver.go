// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !kubeapiserver

//nolint:revive // TODO(CAPP) Fix revive linter
package orchestrator

import (
	"context"
	"embed"
	"io"

	clusterchecks "github.com/DataDog/datadog-agent/comp/core/clusterchecks/def"
	"github.com/DataDog/datadog-agent/comp/core/status"
	"github.com/DataDog/datadog-agent/pkg/util/option"
)

// GetStatus returns status info for the orchestrator explorer.
//
//nolint:revive // TODO(CAPP) Fix revive linter
func GetStatus(_ context.Context, _ interface{}, _ option.Option[clusterchecks.Component]) map[string]interface{} {
	status := make(map[string]interface{})
	status["Disabled"] = "The orchestrator explorer is not compiled-in"
	return status
}

// Provider provides the functionality to populate the status output
type Provider struct {
	ClusterChecksHandler option.Option[clusterchecks.Component]
}

// NewProvider creates a new Provider with optional cluster checks handler
func NewProvider(clusterChecksHandler option.Option[clusterchecks.Component]) Provider {
	return Provider{
		ClusterChecksHandler: clusterChecksHandler,
	}
}

//go:embed status_templates
var templatesFS embed.FS

// Name returns the name
func (p Provider) Name() string {
	return "Orchestrator Explorer"
}

// Section return the section
func (p Provider) Section() string {
	return "Orchestrator Explorer"
}

// JSON populates the status map
func (p Provider) JSON(_ bool, stats map[string]interface{}) error {
	stats["orchestrator"] = map[string]interface{}{
		"Disabled": "The orchestrator explorer is not compiled-in",
	}
	return nil
}

// Text renders the text output
func (p Provider) Text(_ bool, buffer io.Writer) error {
	return status.RenderText(templatesFS, "orchestrator.tmpl", buffer, p.getStatusInfo())
}

// HTML renders the html output
func (p Provider) HTML(_ bool, _ io.Writer) error {
	return nil
}

func (p Provider) getStatusInfo() map[string]interface{} {
	return map[string]interface{}{
		"orchestrator": map[string]interface{}{
			"Disabled": "The orchestrator explorer is not compiled-in",
		},
	}
}
