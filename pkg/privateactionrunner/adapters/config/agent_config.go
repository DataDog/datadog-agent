// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package config

import (
	"encoding/base64"
	"errors"
	"net"
	"net/http"
	"net/url"
	"path/filepath"
	"strconv"
	"time"

	logsconfig "github.com/DataDog/datadog-agent/comp/logs/agent/config"
	confighelper "github.com/DataDog/datadog-agent/pkg/config/helper"
	"github.com/DataDog/datadog-agent/pkg/config/model"
	configsetup "github.com/DataDog/datadog-agent/pkg/config/setup"
	configutils "github.com/DataDog/datadog-agent/pkg/config/utils"
	par "github.com/DataDog/datadog-agent/pkg/privateactionrunner"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/util"
	httputils "github.com/DataDog/datadog-agent/pkg/util/http"
)

const defaultIdentityFileName = "privateactionrunner_private_identity.json"

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
	return apiBaseURL(c.source, c.Site())
}

func (c *agentConfig) OPMSExtraHeaders() map[string]string {
	return c.source.GetStringMapString(par.OPMSExtraHeaders)
}

func (c *agentConfig) Enabled() bool {
	return c.source.GetBool(par.Enabled)
}

func (c *agentConfig) SplitEnabled() bool {
	return c.source.GetBool(par.SplitEnabled)
}

func (c *agentConfig) FIPSEnabled() bool {
	return c.source.GetBool(par.FIPSEnabled)
}

func (c *agentConfig) ProcessManagerEnabled() bool {
	return c.source.GetBool(par.ProcessManagerEnabled)
}

func (c *agentConfig) SelfEnroll() bool {
	return c.source.GetBool(par.SelfEnroll)
}

func (c *agentConfig) APIKeyOnlyEnrollment() bool {
	return c.source.GetBool(par.APIKeyOnlyEnrollment)
}

func (c *agentConfig) SkipConnectionCreation() bool {
	return c.source.GetBool(par.SkipConnectionCreation)
}

func (c *agentConfig) IdentityUseK8sSecret() bool {
	return c.source.GetBool(par.IdentityUseK8sSecret)
}

func (c *agentConfig) IdentitySecretName() string {
	return c.source.GetString(par.IdentitySecretName)
}

func (c *agentConfig) URN() string {
	return c.source.GetString(par.URN)
}

func (c *agentConfig) APIKey() string {
	return c.source.GetString(par.APIKey)
}

func (c *agentConfig) AppKey() string {
	return c.source.GetString(par.AppKey)
}

func (c *agentConfig) ExecutorSocketPath() string {
	return c.source.GetString(par.ExecutorSocketPath)
}

func (c *agentConfig) IdleTimeoutSeconds() int {
	return c.source.GetInt(par.IdleTimeoutSeconds)
}

func (c *agentConfig) LogLevel() string {
	return c.source.GetString(par.LogLevel)
}

func (c *agentConfig) SkipSSLValidation() bool {
	return c.source.GetBool(par.SkipSSLValidation)
}

func (c *agentConfig) MinTLSVersion() string {
	return c.source.GetString(par.MinTLSVersion)
}

func (c *agentConfig) RemoteConfigAPIKey() string {
	return c.source.GetString(par.RemoteConfigAPIKey)
}

func (c *agentConfig) RemoteConfigKey() string {
	return c.source.GetString(par.RemoteConfigKey)
}

func (c *agentConfig) DefaultActionsEnabled() bool {
	return c.source.GetBool(par.DefaultActionsEnabled)
}

func (c *agentConfig) ActionPatterns() []string {
	return c.source.GetStringSlice(par.ActionsAllowlist)
}

func (c *agentConfig) CredentialValues() any {
	return c.source.Get(par.CredentialsValues)
}

func (c *agentConfig) Site() string {
	return site(c.source)
}

func (c *agentConfig) IdentityFilePath() string {
	if path := c.source.GetString(par.IdentityFilePath); path != "" {
		return path
	}
	if path := c.source.GetString(par.AuthTokenFilePath); path != "" {
		return filepath.Join(filepath.Dir(path), defaultIdentityFileName)
	}
	return filepath.Join(filepath.Dir(c.source.ConfigFileUsed()), defaultIdentityFileName)
}

func (c *agentConfig) Identity() (*Identity, error) {
	return ReadIdentity(c.source)
}

func (c *agentConfig) SetIdentity(privateKey, urn string) error {
	writer, ok := c.source.(model.Writer)
	if !ok {
		return errors.New("runner identity requires writable Agent configuration")
	}
	writer.Set(par.PrivateKey, privateKey, model.SourceAgentRuntime)
	writer.Set(par.URN, urn, model.SourceAgentRuntime)
	return nil
}

func (c *agentConfig) SetEnrolledIdentity(identity *Identity) error {
	key, err := util.EcdsaToJWK(identity.PrivateKey)
	if err != nil {
		return err
	}
	data, err := key.MarshalJSON()
	if err != nil {
		return err
	}
	return c.SetIdentity(base64.RawURLEncoding.EncodeToString(data), identity.Urn)
}

func (c *agentConfig) Validate() error {
	return Validate(c.source)
}

func (c *agentConfig) AllowedActions() ActionsAllowlist {
	return MakeActionsAllowlist(c.source)
}

func (c *agentConfig) HTTPTransport() *http.Transport {
	return httputils.CreateHTTPTransport(c.source)
}

func (c *agentConfig) ProxyURL(req *http.Request) (*url.URL, error) {
	if proxies := c.source.GetProxies(); proxies != nil {
		return httputils.GetProxyTransportFunc(proxies, c.source)(req)
	}
	return nil, nil
}

func (c *agentConfig) RemoteConfigEnabled() bool {
	return configutils.IsRemoteConfigEnabled(c.source)
}

func (c *agentConfig) RemoteConfigURL() string {
	return configutils.GetMainEndpoint(c.source, "https://config.", par.RemoteConfigURL)
}

func (c *agentConfig) MetricsAddress() (string, int) {
	return configutils.GetBindHost(c.source), c.source.GetInt(par.DogstatsdPort)
}

func (c *agentConfig) TelemetryAddress() string {
	return net.JoinHostPort(configutils.GetBindHost(c.source), strconv.Itoa(c.source.GetInt(par.MetricsPort)))
}

func (c *agentConfig) APIBaseURLForSite(site string) (string, error) {
	return apiBaseURL(c.source, site)
}

func (c *agentConfig) AgentAPIURL() (string, error) {
	address, err := confighelper.GetIPCAddress(c.source)
	if err != nil {
		return "", err
	}
	return "https://" + net.JoinHostPort(address, strconv.Itoa(c.source.GetInt(par.CommandPort))), nil
}

func (c *agentConfig) HasAPIKey() bool {
	return c.source.IsConfigured(par.APIKey)
}

func (c *agentConfig) LogProcessingRules() ([]*logsconfig.ProcessingRule, error) {
	return logsconfig.GlobalProcessingRules(c.source)
}

func (c *agentConfig) CaptureSocketPaths() (string, string) {
	source := configsetup.SystemProbe()
	return source.GetString(par.PacketCaptureSocket), source.GetString(par.SystemProbeSocket)
}
