// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package agent

import (
	"fmt"
	"os"

	e2eos "github.com/DataDog/test-infra-definitions/components/os"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/runner"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/runner/parameters"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/utils/e2e/client"
)

const (
	linuxInstallScriptURL = "https://install.datadoghq.com/scripts/install_script_agent7.sh"
)

// InstallOption is an optional function parameter type for InstallParams options
type InstallOption func(*installParams)

type installParams struct {
	remoteUpdates  bool
	stablePackages bool
}

var defaultInstallParams = &installParams{
	remoteUpdates:  false,
	stablePackages: false,
}

// WithRemoteUpdates enables remote updates.
func WithRemoteUpdates() InstallOption {
	return func(p *installParams) {
		p.remoteUpdates = true
	}
}

// WithStablePackages uses the stable packages.
func WithStablePackages() InstallOption {
	return func(p *installParams) {
		p.stablePackages = true
	}
}

// Install installs the agent.
func (a *Agent) Install(options ...InstallOption) error {
	params := defaultInstallParams
	for _, option := range options {
		option(params)
	}
	switch a.remote.OSFamily {
	case e2eos.LinuxFamily:
		return a.installLinuxInstallScript(params)
	default:
		return fmt.Errorf("unsupported OS family: %v", a.remote.OSFamily)
	}
}

// MustInstall installs the agent and panics if it fails.
func (a *Agent) MustInstall(options ...InstallOption) {
	err := a.Install(options...)
	require.NoError(a.t(), err)
}

func (a *Agent) installLinuxInstallScript(params *installParams) error {
	env := map[string]string{
		"DD_API_KEY": apiKey(),
		"DD_SITE":    "datadoghq.com",
	}
	if params.remoteUpdates {
		env["DD_REMOTE_UPDATES"] = "true"
	}
	if !params.stablePackages {
		env["TESTING_KEYS_URL"] = "apttesting.datad0g.com/test-keys"
		env["TESTING_APT_URL"] = fmt.Sprintf("s3.amazonaws.com/apttesting.datad0g.com/datadog-agent/pipeline-%s-a7", os.Getenv("E2E_PIPELINE_ID"))
		env["TESTING_APT_REPO_VERSION"] = fmt.Sprintf("stable-%s 7", a.remote.Architecture)
		env["TESTING_YUM_URL"] = "s3.amazonaws.com/yumtesting.datad0g.com"
		env["TESTING_YUM_VERSION_PATH"] = fmt.Sprintf("testing/pipeline-%s-a7/7", os.Getenv("E2E_PIPELINE_ID"))
		env["DD_APM_INSTRUMENTATION_PIPELINE_ID"] = os.Getenv("E2E_PIPELINE_ID")
	}
	_, err := a.remote.Execute(fmt.Sprintf(`bash -c "$(curl -L %s)"`, linuxInstallScriptURL), client.WithEnvVariables(env))
	return err
}

// Uninstall uninstalls the agent.
func (a *Agent) Uninstall() error {
	switch a.remote.OSFamily {
	case e2eos.LinuxFamily:
		return a.uninstallLinux()
	default:
		return fmt.Errorf("unsupported OS family: %v", a.remote.OSFamily)
	}
}

// MustUninstall uninstalls the agent and panics if it fails.
func (a *Agent) MustUninstall() {
	err := a.Uninstall()
	require.NoError(a.t(), err)
}

func (a *Agent) uninstallLinux() error {
	_, err := a.remote.Execute("sudo apt-get remove -y --purge datadog-installer datadog-agent|| sudo yum remove -y datadog-installer datadog-agent || sudo zypper remove -y datadog-installer datadog-agent")
	if err != nil {
		return err
	}
	_, err = a.remote.Execute("sudo rm -rf /etc/datadog-agent")
	if err != nil {
		return err
	}
	return nil
}

func apiKey() string {
	apiKey, err := runner.GetProfile().SecretStore().Get(parameters.APIKey)
	if apiKey == "" || err != nil {
		apiKey = "deadbeefdeadbeefdeadbeefdeadbeef"
	}
	return apiKey
}
