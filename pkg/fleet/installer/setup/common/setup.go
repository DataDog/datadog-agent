// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package common defines the Setup structure that allows setup scripts to define packages and configurations to install.
package common

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"os/user"
	"strings"
	"time"

	"github.com/DataDog/datadog-agent/pkg/fleet/installer"
	"github.com/DataDog/datadog-agent/pkg/fleet/installer/env"
	"github.com/DataDog/datadog-agent/pkg/fleet/installer/installinfo"
	"github.com/DataDog/datadog-agent/pkg/fleet/installer/oci"
	"github.com/DataDog/datadog-agent/pkg/fleet/installer/telemetry"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/datadog-agent/pkg/version"
)

const (
	commandTimeoutDuration = 10 * time.Second
)

var (
	// ErrNoAPIKey is returned when no API key is provided.
	ErrNoAPIKey = errors.New("no API key provided")
)

// Setup allows setup scripts to define packages and configurations to install.
type Setup struct {
	configDir string
	installer installer.Installer
	start     time.Time
	flavor    string

	Out                     *Output
	Env                     *env.Env
	Ctx                     context.Context
	Span                    *telemetry.Span
	Packages                Packages
	Config                  Config
	DdAgentAdditionalGroups []string
}

// NewSetup creates a new Setup structure with some default values.
func NewSetup(ctx context.Context, env *env.Env, flavor string, flavorPath string, logOutput io.Writer) (*Setup, error) {
	header := `Datadog Installer %s - https://www.datadoghq.com
Running the %s installation script (https://github.com/DataDog/datadog-agent/tree/%s/pkg/fleet/installer/setup/%s) - %s
`
	start := time.Now()
	output := &Output{tty: logOutput}
	output.WriteString(fmt.Sprintf(header, version.AgentVersion, flavor, version.Commit, flavorPath, start.Format(time.RFC3339)))
	if env.APIKey == "" {
		return nil, ErrNoAPIKey
	}
	installer, err := installer.NewInstaller(env)
	if err != nil {
		return nil, fmt.Errorf("failed to create installer: %w", err)
	}
	var proxyNoProxy []string
	if os.Getenv("DD_PROXY_NO_PROXY") != "" {
		proxyNoProxy = strings.Split(os.Getenv("DD_PROXY_NO_PROXY"), ",")
	}
	span, ctx := telemetry.StartSpanFromContext(ctx, fmt.Sprintf("setup.%s", flavor))
	s := &Setup{
		configDir: configDir,
		installer: installer,
		start:     start,
		flavor:    flavor,
		Out:       output,
		Env:       env,
		Ctx:       ctx,
		Span:      span,
		Config: Config{
			DatadogYAML: DatadogConfig{
				APIKey:   env.APIKey,
				Hostname: os.Getenv("DD_HOSTNAME"),
				Site:     env.Site,
				Proxy: DatadogConfigProxy{
					HTTP:    os.Getenv("DD_PROXY_HTTP"),
					HTTPS:   os.Getenv("DD_PROXY_HTTPS"),
					NoProxy: proxyNoProxy,
				},
				Env: os.Getenv("DD_ENV"),
			},
			IntegrationConfigs: make(map[string]IntegrationConfig),
		},
		Packages: Packages{
			install: make(map[string]packageWithVersion),
		},
	}
	return s, nil
}

// Run installs the packages and writes the configurations
func (s *Setup) Run() (err error) {
	defer func() { s.Span.Finish(err) }()
	s.Out.WriteString("Applying configurations...\n")
	err = writeConfigs(s.Config, s.configDir)
	if err != nil {
		return fmt.Errorf("failed to write configuration: %w", err)
	}
	packages := resolvePackages(s.Packages)
	s.Out.WriteString("The following packages will be installed:\n")
	installerVersion := "latest"
	if s.Env.DefaultPackagesVersionOverride["datadog-installer"] != "" {
		installerVersion = s.Env.DefaultPackagesVersionOverride["datadog-installer"]
	}
	s.Out.WriteString(fmt.Sprintf("  - %s / %s\n", "datadog-installer", installerVersion))
	for _, p := range packages {
		s.Out.WriteString(fmt.Sprintf("  - %s / %s\n", p.name, p.version))
	}
	// HACK: even if the setup and installer are currently merged we can't self install since we
	// don't have the full OCI and only the installer CLI.
	// To solve this issue we assume our parent OCI is available with our version as tag.
	url := oci.PackageURL(s.Env, "datadog-installer", installerVersion)
	err = s.installPackage("datadog-installer", url)
	if err != nil {
		return fmt.Errorf("failed to install installer: %w", err)
	}
	err = installinfo.WriteInstallInfo(fmt.Sprintf("install-%s.sh", s.flavor))
	if err != nil {
		return fmt.Errorf("failed to write install info: %w", err)
	}
	for _, group := range s.DdAgentAdditionalGroups {
		// Add dd-agent user to additional group for permission reason, in particular to enable reading log files not world readable
		if _, err := user.LookupGroup(group); err != nil {
			log.Infof("Skipping group %s as it does not exist", group)
			continue
		}
		_, err = ExecuteCommandWithTimeout(s, "usermod", "-aG", group, "dd-agent")
		if err != nil {
			s.Out.WriteString("Failed to add dd-agent to group" + group + ": " + err.Error())
			log.Warnf("failed to add dd-agent to group %s:  %v", group, err)
		}
	}

	for _, p := range packages {
		url := oci.PackageURL(s.Env, p.name, p.version)
		err = s.installPackage(p.name, url)
		if err != nil {
			return fmt.Errorf("failed to install package %s: %w", url, err)
		}
	}
	err = s.restartServices(packages)
	if err != nil {
		return fmt.Errorf("failed to restart services: %w", err)
	}
	s.Out.WriteString(fmt.Sprintf("Successfully ran the %s install script in %s!\n", s.flavor, time.Since(s.start).Round(time.Second)))
	return nil
}

// installPackage mimicks the telemetry of calling the install package command
func (s *Setup) installPackage(name string, url string) (err error) {
	span, ctx := telemetry.StartSpanFromContext(s.Ctx, "install")
	defer func() { span.Finish(err) }()
	span.SetTag("url", url)
	span.SetTopLevel()

	s.Out.WriteString(fmt.Sprintf("Installing %s...\n", name))
	err = s.installer.Install(ctx, url, nil)
	if err != nil {
		return err
	}
	s.Out.WriteString(fmt.Sprintf("Successfully installed %s\n", name))
	return nil
}

// ExecuteCommandWithTimeout executes a bash command with args and times out if the command has not finished
var ExecuteCommandWithTimeout = func(s *Setup, command string, args ...string) (output []byte, err error) {
	span, _ := telemetry.StartSpanFromContext(s.Ctx, "setup.command")
	span.SetResourceName(command)
	defer func() { span.Finish(err) }()

	ctx, cancel := context.WithTimeout(context.Background(), commandTimeoutDuration)
	defer cancel()

	cmd := exec.CommandContext(ctx, command, args...)
	output, err = cmd.Output()
	if output != nil {
		span.SetTag("command_output", string(output))
	}

	if err != nil {
		span.SetTag("command_error", err.Error())
		span.Finish(err)
		return nil, err
	}
	return output, nil
}
