// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

// Package statusimpl implements the status provider for the Private Action Runner
package statusimpl

import (
	"embed"
	"io"
	"strings"

	"go.uber.org/fx"

	"github.com/DataDog/datadog-agent/comp/core/config"
	"github.com/DataDog/datadog-agent/comp/core/status"
	par "github.com/DataDog/datadog-agent/comp/privateactionrunner/def"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

//go:embed status_templates
var templatesFS embed.FS

type dependencies struct {
	fx.In

	Config config.Component
}

type provides struct {
	fx.Out

	StatusProvider status.InformationProvider
}

// Module defines the fx options for the Private Action Runner status component.
func Module() fxutil.Module {
	return fxutil.Component(
		fx.Provide(newStatusProvider))
}

type statusProvider struct {
	config config.Component
}

func newStatusProvider(deps dependencies) provides {
	return provides{
		StatusProvider: status.NewInformationProvider(statusProvider{
			config: deps.Config,
		}),
	}
}

// Name returns the name
func (s statusProvider) Name() string {
	return "Private Action Runner"
}

// Section returns the section
func (s statusProvider) Section() string {
	return "Private Action Runner"
}

// JSON populates the status map
func (s statusProvider) JSON(_ bool, stats map[string]interface{}) error {
	s.populateStatus(stats)
	return nil
}

// Text renders the text output
func (s statusProvider) Text(_ bool, buffer io.Writer) error {
	return status.RenderText(templatesFS, "privateactionrunner.tmpl", buffer, s.getStatusInfo())
}

// HTML renders the html output
func (s statusProvider) HTML(_ bool, buffer io.Writer) error {
	return status.RenderHTML(templatesFS, "privateactionrunnerHTML.tmpl", buffer, s.getStatusInfo())
}

func (s statusProvider) getStatusInfo() map[string]interface{} {
	stats := make(map[string]interface{})
	s.populateStatus(stats)
	return stats
}

func (s statusProvider) populateStatus(stats map[string]interface{}) {
	parStatus := make(map[string]interface{})

	enabled := s.config.GetBool(par.PAREnabled)
	parStatus["Enabled"] = enabled

	if enabled {
		urn := s.config.GetString(par.PARUrn)
		if urn == "" {
			urn = "(not set)"
		}
		parStatus["URN"] = urn
		parStatus["SelfEnroll"] = s.config.GetBool(par.PARSelfEnroll)
		parStatus["ActionsAllowlist"] = strings.Join(s.config.GetStringSlice(par.PARActionsAllowlist), ", ")
	}

	stats["privateActionRunnerStatus"] = parStatus
}
