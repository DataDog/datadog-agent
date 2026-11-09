// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package config

import (
	"time"

	"github.com/DataDog/datadog-agent/pkg/config/model"
	par "github.com/DataDog/datadog-agent/pkg/privateactionrunner"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/util"
)

type agentConfig struct {
	source model.Reader
}

// New restricts access to the shared Agent configuration without copying values.
func New(source model.Reader) Config {
	return &agentConfig{source: source}
}

func (c *agentConfig) RunnerID() (string, error) {
	identity, err := util.ParseRunnerURN(c.source.GetString(par.URN))
	if err != nil {
		return "", err
	}
	return identity.RunnerID, nil
}

func (c *agentConfig) HTTPTimeout() time.Duration {
	if seconds := c.source.GetInt32(par.HTTPTimeoutSeconds); seconds != 0 {
		return time.Duration(seconds) * time.Second
	}
	return 30 * time.Second
}

func (c *agentConfig) HTTPAllowlist() []string {
	return c.source.GetStringSlice(par.HTTPAllowlist)
}

func (c *agentConfig) HTTPAllowIMDSEndpoint() bool {
	return c.source.GetBool(par.HTTPAllowIMDSEndpoint)
}

func (c *agentConfig) KubernetesAllowedCustomResources() ([]string, bool) {
	return c.source.GetStringSlice(par.KubernetesAllowedCustomResources), c.source.IsConfigured(par.KubernetesAllowedCustomResources)
}

func (c *agentConfig) RestrictedShellAllowedPaths() ([]string, bool) {
	return c.source.GetStringSlice(par.RestrictedShellAllowedPaths), c.source.IsConfigured(par.RestrictedShellAllowedPaths)
}

func (c *agentConfig) RestrictedShellAllowedCommands() ([]string, bool) {
	return c.source.GetStringSlice(par.RestrictedShellAllowedCommands), c.source.IsConfigured(par.RestrictedShellAllowedCommands)
}

// Nil means unset; an explicitly empty map denies all services.
func (c *agentConfig) RestrictedShellAllowedSystemServices() map[string][]string {
	if !c.source.IsConfigured(par.RestrictedShellAllowedSystemServices) {
		return nil
	}
	services := c.source.GetStringMapStringSlice(par.RestrictedShellAllowedSystemServices)
	if services == nil {
		return map[string][]string{}
	}
	return services
}

// Nil means unset; an explicitly empty list disables elevation.
func (c *agentConfig) RestrictedShellElevatableCommands() []string {
	if !c.source.IsConfigured(par.RestrictedShellPrivilegedElevatableCommands) {
		return nil
	}
	return append([]string{}, c.source.GetStringSlice(par.RestrictedShellPrivilegedElevatableCommands)...)
}

func (c *agentConfig) RestrictedShellDisableDetailedTelemetry() bool {
	return c.source.GetBool(par.RestrictedShellDisableDetailedTelemetry)
}

func (c *agentConfig) RestrictedShellPrivilegedEnabled() bool {
	return c.source.GetBool(par.RestrictedShellPrivilegedEnabled)
}

func (c *agentConfig) RestrictedShellPrivilegedSocket() string {
	return c.source.GetString(par.RestrictedShellPrivilegedSocket)
}

func (c *agentConfig) TaskTimeoutSeconds() int32 {
	return c.source.GetInt32(par.TaskTimeoutSeconds)
}

func (c *agentConfig) TaskConcurrency() int32 {
	return c.source.GetInt32(par.TaskConcurrency)
}

func (c *agentConfig) IsActionAllowed(bundle, action string) bool {
	return MakeActionsAllowlist(c.source).IsActionAllowed(bundle, action)
}

func (c *agentConfig) APIBaseURL() (string, error) {
	return par.APIBaseURL(c.source, par.Site(c.source))
}

func (c *agentConfig) OPMSExtraHeaders() map[string]string {
	return c.source.GetStringMapString(par.OPMSExtraHeaders)
}
