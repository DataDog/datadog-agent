// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package bootstrapper provides the installer bootstrapper commands.
package bootstrapper

import (
	"net/url"
	"os"

	"github.com/DataDog/datadog-agent/pkg/fleet/bootstrapper"
	installer "github.com/DataDog/datadog-agent/pkg/fleet/installer/commands"
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
	*installer.Cmd
}

func newBootstrapperCmd(operation string) *bootstrapperCmd {
	cmd := installer.NewCmd(operation)
	cmd.Span.SetTag("env.DD_UPGRADE", os.Getenv(envUpgrade))
	cmd.Span.SetTag("env.DD_APM_INSTRUMENTATION_NO_CONFIG_CHANGE", os.Getenv(envAPMInstrumentationNoConfigChange))
	cmd.Span.SetTag("env.DD_SYSTEM_PROBE_ENSURE_CONFIG", os.Getenv(envSystemProbeEnsureConfig))
	cmd.Span.SetTag("env.DD_RUNTIME_SECURITY_CONFIG_ENABLED", os.Getenv(envRuntimeSecurityConfigEnabled))
	cmd.Span.SetTag("env.DD_COMPLIANCE_CONFIG_ENABLED", os.Getenv(envComplianceConfigEnabled))
	cmd.Span.SetTag("env.DD_INSTALL_ONLY", os.Getenv(envInstallOnly))
	cmd.Span.SetTag("env.DD_NO_AGENT_INSTALL", os.Getenv(envNoAgentInstall))
	cmd.Span.SetTag("env.DD_APM_INSTRUMENTATION_LIBRARIES", os.Getenv(envAPMInstrumentationLibraries))
	cmd.Span.SetTag("env.DD_APM_INSTRUMENTATION_LANGUAGES", os.Getenv(envAPMInstrumentationLanguages))
	cmd.Span.SetTag("env.DD_APPSEC_ENABLED", os.Getenv(envAppSecEnabled))
	cmd.Span.SetTag("env.DD_IAST_ENABLED", os.Getenv(envIASTEnabled))
	cmd.Span.SetTag("env.DD_APM_INSTRUMENTATION_ENABLED", os.Getenv(envAPMInstrumentationEnabled))
	cmd.Span.SetTag("env.DD_REPO_URL", os.Getenv(envRepoURL))
	cmd.Span.SetTag("env.REPO_URL", os.Getenv(envRepoURLDeprecated))
	cmd.Span.SetTag("env.DD_RPM_REPO_GPGCHECK", os.Getenv(envRPMRepoGPGCheck))
	cmd.Span.SetTag("env.DD_AGENT_MAJOR_VERSION", os.Getenv(envAgentMajorVersion))
	cmd.Span.SetTag("env.DD_AGENT_MINOR_VERSION", os.Getenv(envAgentMinorVersion))
	cmd.Span.SetTag("env.DD_AGENT_DIST_CHANNEL", os.Getenv(envAgentDistChannel))
	cmd.Span.SetTag("env.DD_REMOTE_UPDATES", os.Getenv(envRemoteUpdates))
	cmd.Span.SetTag("env.HTTP_PROXY", redactURL(os.Getenv(envHTTPProxy)))
	cmd.Span.SetTag("env.http_proxy", redactURL(os.Getenv(envhttpProxy)))
	cmd.Span.SetTag("env.HTTPS_PROXY", redactURL(os.Getenv(envHTTPSProxy)))
	cmd.Span.SetTag("env.https_proxy", redactURL(os.Getenv(envhttpsProxy)))
	cmd.Span.SetTag("env.NO_PROXY", os.Getenv(envNoProxy))
	cmd.Span.SetTag("env.no_proxy", os.Getenv(envnoProxy))
	return &bootstrapperCmd{
		Cmd: cmd,
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

// BootstrapCommand is the command to bootstrap the package with the first version
func BootstrapCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "bootstrap",
		Short:   "Bootstraps the package with the first version.",
		GroupID: "bootstrap",
		RunE: func(_ *cobra.Command, _ []string) (err error) {
			b := newBootstrapperCmd("bootstrap")
			defer func() { b.Stop(err) }()
			return bootstrapper.Bootstrap(b.Ctx, b.Env)
		},
	}
	return cmd
}
