// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package config

import (
	"net/http"
	"net/url"
)

// RuntimeConfig is used by PAR lifecycle, enrollment, and control-plane code.
// Action handlers receive Config, which does not expose enrollment credentials.
type RuntimeConfig interface {
	Config
	Enabled() bool
	SplitEnabled() bool
	FIPSEnabled() bool
	ProcessManagerEnabled() bool
	SelfEnroll() bool
	APIKeyOnlyEnrollment() bool
	SkipConnectionCreation() bool
	IdentityUseK8sSecret() bool
	IdentitySecretName() string
	URN() string
	APIKey() string
	AppKey() string
	ExecutorSocketPath() string
	IdleTimeoutSeconds() int
	LogLevel() string
	SkipSSLValidation() bool
	MinTLSVersion() string
	RemoteConfigAPIKey() string
	RemoteConfigKey() string
	DefaultActionsEnabled() bool
	ActionPatterns() []string
	CredentialValues() any
	APIBaseURLForSite(string) (string, error)
	Site() string
	IdentityFilePath() string
	Identity() (*Identity, error)
	SetIdentity(privateKey, urn string) error
	SetEnrolledIdentity(identity *Identity) error
	Validate() error
	AllowedActions() ActionsAllowlist
	HTTPTransport() *http.Transport
	ProxyURL(*http.Request) (*url.URL, error)
	RemoteConfigEnabled() bool
	RemoteConfigURL() string
	MetricsAddress() (string, int)
	TelemetryAddress() string
}
