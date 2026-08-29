// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package rcstatusimpl implements the rcstatus component.
package rcstatusimpl

import (
	"embed"
	"encoding/json"
	"expvar"
	"io"

	"github.com/DataDog/datadog-agent/comp/core/config"
	"github.com/DataDog/datadog-agent/comp/core/status"
	compdef "github.com/DataDog/datadog-agent/comp/def"
	configutils "github.com/DataDog/datadog-agent/pkg/config/utils"
)

//go:embed status_templates
var templatesFS embed.FS

// defaultStatusInstance duplicates remoteconfig.DefaultStatusInstance rather
// than importing it: that package drags uptane and bbolt into every binary
// linking this one, which is far more expensive than repeating one string.
const defaultStatusInstance = "Remote Config"

// Requires holds the dependencies for the rcstatus component.
type Requires struct {
	compdef.In

	Config config.Component
}

// Provides holds the outputs of the rcstatus component.
type Provides struct {
	compdef.Out

	StatusProvider status.InformationProvider
}

type statusProvider struct {
	Config config.Component
}

// NewComponent creates a new rcstatus component.
func NewComponent(deps Requires) Provides {
	return Provides{
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

	if configutils.IsRemoteConfigEnabled(rc.Config) && expvar.Get("remoteConfigStatus") != nil {
		remoteConfigStatusJSON := expvar.Get("remoteConfigStatus").String()
		json.Unmarshal([]byte(remoteConfigStatusJSON), &status) //nolint:errcheck
	} else if !configutils.IsRemoteConfigEnabled(rc.Config) {
		if configutils.IsFed(rc.Config) && !rc.Config.IsConfigured("remote_configuration.enabled") {
			status["disabledReason"] = "it is not explicitly enabled in the agent configuration. (`remote_configuration.enabled is unset`)"
		} else {
			status["disabledReason"] = "it is explicitly disabled in the agent configuration. (`remote_configuration.enabled: false`)"
		}

	}

	// Split the extra clients out of the instances map so the template can render
	// them as a list, and so the heading only appears when there is one.
	if instances, ok := status["instances"].(map[string]interface{}); ok {
		additional := make(map[string]interface{}, len(instances))
		for name, instance := range instances {
			if name == defaultStatusInstance {
				continue
			}
			additional[name] = instance
		}
		if len(additional) > 0 {
			status["additionalInstances"] = additional
		}
	}

	stats["remoteConfiguration"] = status

	if expvar.Get("remoteConfigStartup") != nil {
		remoteConfigStartupJSON := expvar.Get("remoteConfigStartup").String()
		startupMap := make(map[string]interface{})
		json.Unmarshal([]byte(remoteConfigStartupJSON), &startupMap) //nolint:errcheck
		stats["remoteConfigStartup"] = startupMap
	}
}
