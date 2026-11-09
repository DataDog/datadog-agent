// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package config

import (
	"net/http"
	"net/url"
	"time"

	logsconfig "github.com/DataDog/datadog-agent/comp/logs/agent/config"
)

// Config exposes the named settings and identity updates PAR consumers may use.
// It does not expose generic key lookup or the underlying Agent component.
// Methods read current values; implementations must not cache configuration.
type Config interface {
	AgentAPIURL() (string, error)
	HasAPIKey() bool
	LogProcessingRules() ([]*logsconfig.ProcessingRule, error)
	CaptureSocketPaths() (capture string, systemProbe string)
	RunnerID() (string, error)
	HTTPTimeout() time.Duration
	HTTPAllowlist() []string
	HTTPAllowIMDSEndpoint() bool
	KubernetesAllowedCustomResources() (resources []string, configured bool)
	RestrictedShellAllowedPaths() (paths []string, configured bool)
	RestrictedShellAllowedCommands() (commands []string, configured bool)
	RestrictedShellAllowedSystemServices() map[string][]string
	RestrictedShellElevatableCommands() []string
	RestrictedShellDisableDetailedTelemetry() bool
	RestrictedShellPrivilegedEnabled() bool
	RestrictedShellPrivilegedSocket() string
	TaskTimeoutSeconds() int32
	TaskConcurrency() int32
	IsActionAllowed(bundle, action string) bool
	APIBaseURL() (string, error)
	OPMSExtraHeaders() map[string]string

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
