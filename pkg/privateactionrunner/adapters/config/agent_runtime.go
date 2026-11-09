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

	"github.com/DataDog/datadog-agent/pkg/config/model"
	configutils "github.com/DataDog/datadog-agent/pkg/config/utils"
	par "github.com/DataDog/datadog-agent/pkg/privateactionrunner"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/util"
	httputils "github.com/DataDog/datadog-agent/pkg/util/http"
)

func (c *agentConfig) Enabled() bool      { return c.source.GetBool(par.Enabled) }
func (c *agentConfig) SplitEnabled() bool { return c.source.GetBool(par.SplitEnabled) }
func (c *agentConfig) FIPSEnabled() bool  { return c.source.GetBool("fips.enabled") }
func (c *agentConfig) ProcessManagerEnabled() bool {
	return c.source.GetBool("process_manager.enabled")
}
func (c *agentConfig) SelfEnroll() bool           { return c.source.GetBool(par.SelfEnroll) }
func (c *agentConfig) APIKeyOnlyEnrollment() bool { return c.source.GetBool(par.APIKeyOnlyEnrollment) }
func (c *agentConfig) SkipConnectionCreation() bool {
	return c.source.GetBool(par.SkipConnectionCreation)
}
func (c *agentConfig) IdentityUseK8sSecret() bool { return c.source.GetBool(par.IdentityUseK8sSecret) }
func (c *agentConfig) IdentitySecretName() string { return c.source.GetString(par.IdentitySecretName) }
func (c *agentConfig) URN() string                { return c.source.GetString(par.URN) }
func (c *agentConfig) APIKey() string             { return c.source.GetString("api_key") }
func (c *agentConfig) AppKey() string             { return c.source.GetString("app_key") }
func (c *agentConfig) ExecutorSocketPath() string { return c.source.GetString(par.ExecutorSocketPath) }
func (c *agentConfig) IdleTimeoutSeconds() int    { return c.source.GetInt(par.IdleTimeoutSeconds) }
func (c *agentConfig) LogLevel() string           { return c.source.GetString("log_level") }
func (c *agentConfig) SkipSSLValidation() bool    { return c.source.GetBool("skip_ssl_validation") }
func (c *agentConfig) MinTLSVersion() string      { return c.source.GetString("min_tls_version") }
func (c *agentConfig) RemoteConfigAPIKey() string {
	return c.source.GetString("remote_configuration.api_key")
}
func (c *agentConfig) RemoteConfigKey() string { return c.source.GetString("remote_configuration.key") }
func (c *agentConfig) DefaultActionsEnabled() bool {
	return c.source.GetBool(par.DefaultActionsEnabled)
}
func (c *agentConfig) ActionPatterns() []string { return c.source.GetStringSlice(par.ActionsAllowlist) }
func (c *agentConfig) CredentialValues() any    { return c.source.Get(par.CredentialsValues) }
func (c *agentConfig) Site() string             { return site(c.source) }
func (c *agentConfig) IdentityFilePath() string {
	if path := c.source.GetString(par.IdentityFilePath); path != "" {
		return path
	}
	if path := c.source.GetString("auth_token_file_path"); path != "" {
		return filepath.Join(filepath.Dir(path), "privateactionrunner_private_identity.json")
	}
	return filepath.Join(filepath.Dir(c.source.ConfigFileUsed()), "privateactionrunner_private_identity.json")
}
func (c *agentConfig) Identity() (*Identity, error) { return ReadIdentity(c.source) }
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
func (c *agentConfig) Validate() error                  { return Validate(c.source) }
func (c *agentConfig) AllowedActions() ActionsAllowlist { return MakeActionsAllowlist(c.source) }
func (c *agentConfig) HTTPTransport() *http.Transport   { return httputils.CreateHTTPTransport(c.source) }
func (c *agentConfig) ProxyURL(req *http.Request) (*url.URL, error) {
	if proxies := c.source.GetProxies(); proxies != nil {
		return httputils.GetProxyTransportFunc(proxies, c.source)(req)
	}
	return nil, nil
}
func (c *agentConfig) RemoteConfigEnabled() bool { return configutils.IsRemoteConfigEnabled(c.source) }
func (c *agentConfig) RemoteConfigURL() string {
	return configutils.GetMainEndpoint(c.source, "https://config.", par.RemoteConfigURL)
}
func (c *agentConfig) MetricsAddress() (string, int) {
	return configutils.GetBindHost(c.source), c.source.GetInt("dogstatsd_port")
}
func (c *agentConfig) TelemetryAddress() string {
	return net.JoinHostPort(configutils.GetBindHost(c.source), strconv.Itoa(c.source.GetInt("metrics_port")))
}

func (c *agentConfig) APIBaseURLForSite(site string) (string, error) {
	return apiBaseURL(c.source, site)
}
