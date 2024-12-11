// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package rcstatusimpl //nolint:revive // TODO(RC) Fix revive linter

import (
	"embed"
	"encoding/json"
	"expvar"
	"io"

	"github.com/DataDog/datadog-agent/comp/core/config"
	"github.com/DataDog/datadog-agent/comp/core/status"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
	"go.uber.org/fx"
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

// Module defines the fx options for the status component.
func Module() fxutil.Module {
	return fxutil.Component(
		fx.Provide(newStatus))
}

type statusProvider struct {
	Config config.Component
}

func newStatus(deps dependencies) provides {
	return provides{
		StatusProvider: status.NewInformationProvider(statusProvider{
			Config: deps.Config,
		}),
	}
}

func (rc statusProvider) getStatusInfo() map[string]interface{} {
	stats := make(map[string]interface{})

	rc.populateStatus(stats)

	return stats
}

// Name returns the name
func (rc statusProvider) Name() string {
	return "Remote Configuration"
}

// Section return the section
func (rc statusProvider) Section() string {
	return "Remote Configuration"
}

// JSON populates the status map
func (rc statusProvider) JSON(_ bool, stats map[string]interface{}) error {
	rc.populateStatus(stats)

	return nil
}

// Text renders the text output
func (rc statusProvider) Text(_ bool, buffer io.Writer) error {
	return status.RenderText(templatesFS, "remoteconfiguration.tmpl", buffer, rc.getStatusInfo())
}

// HTML renders the html output
func (rc statusProvider) HTML(_ bool, buffer io.Writer) error {
	return status.RenderHTML(templatesFS, "remoteconfigurationHTML.tmpl", buffer, rc.getStatusInfo())
}

func (rc statusProvider) populateStatus(stats map[string]interface{}) {
	status := make(map[string]interface{})

	if isRemoteConfigEnabled(rc.Config) && expvar.Get("remoteConfigStatus") != nil {
		remoteConfigStatusJSON := expvar.Get("remoteConfigStatus").String()
		json.Unmarshal([]byte(remoteConfigStatusJSON), &status) //nolint:errcheck
	} else {
		if !rc.Config.GetBool("remote_configuration.enabled") {
			status["disabledReason"] = "it is explicitly disabled in the agent configuration. (`remote_configuration.enabled: false`)"
		} else if rc.Config.GetBool("fips.enabled") {
			status["disabledReason"] = "it is not supported when FIPS is enabled. (`fips.enabled: true`)"
		} else if rc.Config.GetString("site") == "ddog-gov.com" {
			status["disabledReason"] = "it is not supported on GovCloud. (`site: \"ddog-gov.com\"`)"
		}
	}

	stats["remoteConfiguration"] = status
}

func isRemoteConfigEnabled(conf config.Component) bool {
	// Disable Remote Config for GovCloud
	if conf.GetBool("fips.enabled") || conf.GetString("site") == "ddog-gov.com" {
		return false
	}
	return conf.GetBool("remote_configuration.enabled")
}
