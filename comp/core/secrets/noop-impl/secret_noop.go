// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package secretsimpl implements for the secrets component interface
package secretsimpl

import (
	"io"
	"net/http"

	api "github.com/DataDog/datadog-agent/comp/api/api/def"
	flaretypes "github.com/DataDog/datadog-agent/comp/core/flare/types"
	secrets "github.com/DataDog/datadog-agent/comp/core/secrets/def"
	"github.com/DataDog/datadog-agent/comp/core/status"
)

// Provides list the provided interfaces from the secrets Component
type Provides struct {
	Comp            secrets.Component
	FlareProvider   flaretypes.Provider
	InfoEndpoint    api.AgentEndpointProvider
	RefreshEndpoint api.AgentEndpointProvider
	StatusProvider  status.InformationProvider
}

type secretNoop struct{}

var _ secrets.Component = (*secretNoop)(nil)

var secretDisabled = []byte("secret is disabled")

// NewComponent returns the implementation for the secrets component
func NewComponent() Provides {
	resolver := &secretNoop{}
	return Provides{
		Comp:            resolver,
		FlareProvider:   flaretypes.NewProvider(resolver.fillFlare),
		InfoEndpoint:    api.NewAgentEndpointProvider(resolver.writeDebugInfo, "/secrets", "GET"),
		RefreshEndpoint: api.NewAgentEndpointProvider(resolver.handleRefresh, "/secret/refresh", "GET"),
		StatusProvider:  status.NewInformationProvider(resolver),
	}
}

// Name returns the name of the component for status reporting
func (r *secretNoop) Name() string {
	return "Secrets"
}

// Section returns the section name for status reporting
func (r *secretNoop) Section() string {
	return "secrets"
}

// fillFlare fil a flare with secret information
func (r *secretNoop) fillFlare(fb flaretypes.FlareBuilder) error {
	fb.AddFile("secrets.log", secretDisabled)
	return nil
}

// JSON populates the status map
func (r *secretNoop) JSON(_ bool, stats map[string]interface{}) error {
	stats["enabled"] = false
	stats["message"] = "Agent secrets is disabled"
	return nil
}

// Text renders the text output
func (r *secretNoop) Text(_ bool, buffer io.Writer) error {
	buffer.Write(secretDisabled) //nolint:errcheck
	buffer.Write([]byte("\n"))   //nolint:errcheck
	return nil
}

// HTML renders the HTML output
func (r *secretNoop) HTML(_ bool, buffer io.Writer) error {
	buffer.Write([]byte("<div class=\"stat\"><span class=\"stat_title\">")) //nolint:errcheck
	buffer.Write(secretDisabled)                                            //nolint:errcheck
	buffer.Write([]byte("</span></div>"))                                   //nolint:errcheck
	return nil
}

func (r *secretNoop) writeDebugInfo(w http.ResponseWriter, _ *http.Request) {
	w.Write(secretDisabled)
}

func (r *secretNoop) handleRefresh(w http.ResponseWriter, _ *http.Request) {
	w.Write(secretDisabled)
}

// Configure does nothing
func (r *secretNoop) Configure(_ secrets.ConfigParams) {}

// SubscribeToChanges does nothing
func (r *secretNoop) SubscribeToChanges(_ secrets.SecretChangeCallback) {}

// Resolve does nothing
func (r *secretNoop) Resolve(data []byte, _ string, _ string, _ string) ([]byte, error) {
	return data, nil
}

// Refresh does nothing
func (r *secretNoop) Refresh() (string, error) {
	return "", nil
}
