// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package common defines the Setup structure that allows setup scripts to define packages and configurations to install.
package common

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"time"

	"github.com/DataDog/datadog-agent/pkg/fleet/installer"
	"github.com/DataDog/datadog-agent/pkg/fleet/installer/env"
	"github.com/DataDog/datadog-agent/pkg/fleet/installer/installinfo"
	"github.com/DataDog/datadog-agent/pkg/fleet/installer/oci"
	"github.com/DataDog/datadog-agent/pkg/fleet/installer/paths"
	"github.com/DataDog/datadog-agent/pkg/fleet/installer/setup/config"
	"github.com/DataDog/datadog-agent/pkg/fleet/installer/telemetry"
	"github.com/DataDog/datadog-agent/pkg/version"
)

const (
	commandTimeoutDuration = 10 * time.Second
	configDir              = "/etc/datadog-agent"
)

// Setup allows setup scripts to define packages and configurations to install.
type Setup struct {
	configDir string
	installer installer.Installer
	start     time.Time
	flavor    string

	Out                       *Output
	Env                       *env.Env
	Ctx                       context.Context
	Span                      *telemetry.Span
	Packages                  Packages
	Config                    config.Config
	DdAgentAdditionalGroups   []string
	DelayedAgentRestartConfig config.DelayedAgentRestartConfig
	NoConfig                  bool
}

// NewSetup creates a new Setup structure with some default values.
func NewSetup(ctx context.Context, env *env.Env, flavor string, flavorPath string, logOutput io.Writer) (*Setup, error) {
	header := `Datadog Installer %s - https://www.datadoghq.com
Running the %s installation script (https://github.com/DataDog/datadog-agent/tree/%s/pkg/fleet/installer/setup/%s) - %s
`
	start := time.Now()
	output := &Output{tty: logOutput}
	output.WriteString(fmt.Sprintf(header, version.AgentVersion, flavor, version.Commit, flavorPath, start.Format(time.RFC3339)))
	installer, err := installer.NewInstaller(env)
	if err != nil {
		return nil, fmt.Errorf("failed to create installer: %w", err)
	}
	var proxyNoProxy []string
	if os.Getenv("DD_PROXY_NO_PROXY") != "" {
		proxyNoProxy = strings.FieldsFunc(os.Getenv("DD_PROXY_NO_PROXY"), func(r rune) bool {
			return r == ',' || r == ' '
		}) // comma and space-separated list, consistent with viper and documentation
	}
	span, ctx := telemetry.StartSpanFromContext(ctx, fmt.Sprintf("setup.%s", flavor))
	s := &Setup{
		configDir: paths.DatadogDataDir,
		installer: installer,
		start:     start,
		flavor:    flavor,
		Out:       output,
		Env:       env,
		Ctx:       ctx,
		Span:      span,
		Config: config.Config{
			DatadogYAML: config.DatadogConfig{
				APIKey:   env.APIKey,
				Hostname: os.Getenv("DD_HOSTNAME"),
				Site:     env.Site,
				Proxy: config.DatadogConfigProxy{
					HTTP:    os.Getenv("DD_PROXY_HTTP"),
					HTTPS:   os.Getenv("DD_PROXY_HTTPS"),
					NoProxy: proxyNoProxy,
				},
				Env:                os.Getenv("DD_ENV"),
				InfrastructureMode: os.Getenv("DD_INFRASTRUCTURE_MODE"),
			},
			IntegrationConfigs: make(map[string]config.IntegrationConfig),
		},
		Packages: Packages{
			install: make(map[string]packageWithVersion),
		},
	}
	return s, nil
}

// Run installs the packages and writes the configurations
func (s *Setup) Run() (err error) {
	// TODO: go idiom is to get ctx from parameter not a struct
	//       s.Ctx is tied to s.Span, many files would need to be refactored
	ctx := s.Ctx

	defer func() { s.Span.Finish(err) }()

	packages := resolvePackages(s.Env, s.Packages)
	s.Out.WriteString("The following packages will be installed:\n")
	for _, p := range packages {
		s.Out.WriteString(fmt.Sprintf("  - %s / %s\n", p.name, p.version))
	}
	s.Out.WriteString("Stopping Datadog Agent services...\n")
	err = s.stopServices(ctx, packages)
	if err != nil {
		return fmt.Errorf("failed to stop services: %w", err)
	}
	s.Out.WriteString("Applying configurations...\n")
	// ensure config root is created with correct permissions
	err = paths.EnsureInstallerDataDir()
	if err != nil {
		return fmt.Errorf("could not create config directory: %w", err)
	}
	if !s.NoConfig {
		err = config.WriteConfigs(s.Config, s.configDir)
		if err != nil {
			return fmt.Errorf("failed to write configuration: %w", err)
		}
	}
	err = installinfo.WriteInstallInfo(ctx, fmt.Sprintf("install-script-%s", s.flavor))
	if err != nil {
		return fmt.Errorf("failed to write install info: %w", err)
	}
	for _, p := range packages {
		url := oci.PackageURL(s.Env, p.name, p.version)
		err = s.installPackage(p.name, url)
		if err != nil {
			return fmt.Errorf("failed to install package %s: %w", url, err)
		}
	}
	if err = s.postInstallPackages(); err != nil {
		return fmt.Errorf("failed during post-package installation: %w", err)
	}
	if s.Packages.copyInstallerSSI {
		if err := copyInstallerSSI(); err != nil {
			return err
		}
	}
	err = s.restartServices(ctx, packages)
	if err != nil {
		return fmt.Errorf("failed to restart services: %w", err)
	}
	if s.DelayedAgentRestartConfig.Scheduled {
		ScheduleDelayedAgentRestart(s, s.DelayedAgentRestartConfig.Delay, s.DelayedAgentRestartConfig.LogFile)
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
	if runtime.GOOS == "windows" && name == DatadogAgentPackage {
		// TODO(WINA-2018): Add support for skipping the installation of the core Agent if it is already installed
		err = s.installer.ForceInstall(ctx, url, nil)
	} else {
		err = s.installer.Install(ctx, url, nil)
	}
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

// ScheduleDelayedAgentRestart schedules an agent restart after the specified delay
func ScheduleDelayedAgentRestart(s *Setup, delay time.Duration, logFile string) {
	s.Out.WriteString(fmt.Sprintf("Scheduling agent restart in %v for GPU monitoring\n", delay))
	cmd := exec.Command("nohup", "bash", "-c", fmt.Sprintf("echo \"[$(date -u +%%Y-%%m-%%dT%%H:%%M:%%SZ)] Waiting %v...\" >> %[2]s.log && sleep %d && echo \"[$(date -u +%%Y-%%m-%%dT%%H:%%M:%%SZ)] Restarting agent...\" >> %[2]s.log && systemctl restart datadog-agent >> %[2]s.log 2>&1", delay, logFile, int(delay.Seconds())))
	if err := cmd.Start(); err != nil {
		s.Out.WriteString(fmt.Sprintf("Failed to schedule restart: %v\n", err))
	}
}
