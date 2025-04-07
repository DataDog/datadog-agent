// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package commands

import (
	"net/url"
	"os"

	"github.com/DataDog/datadog-agent/pkg/fleet/installer/bootstrap"
	"github.com/spf13/cobra"
)

const (
	envUpgrade                          = "DD_UPGRADE"
	envAPMInstrumentationNoConfigChange = "DD_APM_INSTRUMENTATION_NO_CONFIG_CHANGE"
	envSystemProbeEnsureConfig          = "DD_SYSTEM_PROBE_ENSURE_CONFIG"
	envRuntimeSecurityConfigEnabled     = "DD_RUNTIME_SECURITY_CONFIG_ENABLED"
	envComplianceConfigEnabled          = "DD_COMPLIANCE_CONFIG_ENABLED"
	envInstallOnly                      = "DD_INSTALL_ONLY"
	envNoAgentInstall                   = "DD_NO_AGENT_INSTALL"
	envAPMInstrumentationLibraries      = "DD_APM_INSTRUMENTATION_LIBRARIES"
	// this env var is deprecated but still read by the install script
	envAPMInstrumentationLanguages = "DD_APM_INSTRUMENTATION_LANGUAGES"
	envAppSecEnabled               = "DD_APPSEC_ENABLED"
	envIASTEnabled                 = "DD_IAST_ENABLED"
	envAPMInstrumentationEnabled   = "DD_APM_INSTRUMENTATION_ENABLED"
	envRepoURL                     = "DD_REPO_URL"
	envRepoURLDeprecated           = "REPO_URL"
	envRPMRepoGPGCheck             = "DD_RPM_REPO_GPGCHECK"
	envAgentMajorVersion           = "DD_AGENT_MAJOR_VERSION"
	envAgentMinorVersion           = "DD_AGENT_MINOR_VERSION"
	envAgentDistChannel            = "DD_AGENT_DIST_CHANNEL"
	envRemoteUpdates               = "DD_REMOTE_UPDATES"
	envHTTPProxy                   = "HTTP_PROXY"
	envhttpProxy                   = "http_proxy"
	envHTTPSProxy                  = "HTTPS_PROXY"
	envhttpsProxy                  = "https_proxy"
	envNoProxy                     = "NO_PROXY"
	envnoProxy                     = "no_proxy"
	envIsFromDaemon                = "DD_INSTALLER_FROM_DAEMON"
)

type bootstrapperCmd struct {
	*cmd
}

func newBootstrapperCmd(operation string) *bootstrapperCmd {
	cmd := newCmd(operation)
	cmd.span.SetTag("env_var.DD_UPGRADE", os.Getenv(envUpgrade))
	cmd.span.SetTag("env_var.DD_APM_INSTRUMENTATION_NO_CONFIG_CHANGE", os.Getenv(envAPMInstrumentationNoConfigChange))
	cmd.span.SetTag("env_var.DD_SYSTEM_PROBE_ENSURE_CONFIG", os.Getenv(envSystemProbeEnsureConfig))
	cmd.span.SetTag("env_var.DD_RUNTIME_SECURITY_CONFIG_ENABLED", os.Getenv(envRuntimeSecurityConfigEnabled))
	cmd.span.SetTag("env_var.DD_COMPLIANCE_CONFIG_ENABLED", os.Getenv(envComplianceConfigEnabled))
	cmd.span.SetTag("env_var.DD_INSTALL_ONLY", os.Getenv(envInstallOnly))
	cmd.span.SetTag("env_var.DD_NO_AGENT_INSTALL", os.Getenv(envNoAgentInstall))
	cmd.span.SetTag("env_var.DD_APM_INSTRUMENTATION_LIBRARIES", os.Getenv(envAPMInstrumentationLibraries))
	cmd.span.SetTag("env_var.DD_APM_INSTRUMENTATION_LANGUAGES", os.Getenv(envAPMInstrumentationLanguages))
	cmd.span.SetTag("env_var.DD_APPSEC_ENABLED", os.Getenv(envAppSecEnabled))
	cmd.span.SetTag("env_var.DD_IAST_ENABLED", os.Getenv(envIASTEnabled))
	cmd.span.SetTag("env_var.DD_APM_INSTRUMENTATION_ENABLED", os.Getenv(envAPMInstrumentationEnabled))
	cmd.span.SetTag("env_var.DD_REPO_URL", os.Getenv(envRepoURL))
	cmd.span.SetTag("env_var.REPO_URL", os.Getenv(envRepoURLDeprecated))
	cmd.span.SetTag("env_var.DD_RPM_REPO_GPGCHECK", os.Getenv(envRPMRepoGPGCheck))
	cmd.span.SetTag("env_var.DD_AGENT_MAJOR_VERSION", os.Getenv(envAgentMajorVersion))
	cmd.span.SetTag("env_var.DD_AGENT_MINOR_VERSION", os.Getenv(envAgentMinorVersion))
	cmd.span.SetTag("env_var.DD_AGENT_DIST_CHANNEL", os.Getenv(envAgentDistChannel))
	cmd.span.SetTag("env_var.DD_REMOTE_UPDATES", os.Getenv(envRemoteUpdates))
	cmd.span.SetTag("env_var.HTTP_PROXY", redactURL(os.Getenv(envHTTPProxy)))
	cmd.span.SetTag("env_var.http_proxy", redactURL(os.Getenv(envhttpProxy)))
	cmd.span.SetTag("env_var.HTTPS_PROXY", redactURL(os.Getenv(envHTTPSProxy)))
	cmd.span.SetTag("env_var.https_proxy", redactURL(os.Getenv(envhttpsProxy)))
	cmd.span.SetTag("env_var.NO_PROXY", os.Getenv(envNoProxy))
	cmd.span.SetTag("env_var.no_proxy", os.Getenv(envnoProxy))
	return &bootstrapperCmd{
		cmd: cmd,
	}
}

func redactURL(u string) string {
	if u == "" {
		return ""
	}
	url, err := url.Parse(u)
	if err != nil {
		return "invalid"
	}
	return url.Redacted()
}

func bootstrapCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "bootstrap",
		Short:   "Bootstraps the package with the first version.",
		GroupID: "installer",
		RunE: func(_ *cobra.Command, _ []string) (err error) {
			b := newBootstrapperCmd("bootstrap")
			defer func() { b.stop(err) }()
			return bootstrap.Bootstrap(b.ctx, b.env)
		},
	}
	return cmd
}
