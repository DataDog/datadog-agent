// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package ddot implements 'agent ddot'.
package ddot

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/spf13/cobra"

	"github.com/DataDog/datadog-agent/cmd/agent/command"
	"github.com/DataDog/datadog-agent/pkg/fleet/installer/env"
	"github.com/DataDog/datadog-agent/pkg/fleet/installer/exec"
	"github.com/DataDog/datadog-agent/pkg/fleet/installer/oci"
	"github.com/DataDog/datadog-agent/pkg/util/defaultpaths"
	pkgversion "github.com/DataDog/datadog-agent/pkg/version"
)

const extensionName = "ddot"

// Commands returns a slice of subcommands for the 'agent' command.
func Commands(_ *command.GlobalParams) []*cobra.Command {
	ddotCmd := &cobra.Command{
		Use:   "ddot [command]",
		Short: "Manage the DDOT installation",
	}
	ddotCmd.AddCommand(installCommand(), removeCommand())
	return []*cobra.Command{ddotCmd}
}

func installCommand() *cobra.Command {
	var explicitURL string
	var registry string

	cmd := &cobra.Command{
		Use:   "install",
		Short: "Install DDOT from an OCI package",
		Long: `Install DDOT from an OCI package.

The package URL is determined as follows (in order of precedence):
  1. --registry flag: construct oci://<registry>/agent-package:<current-version>
  2. No flags: use the default Datadog registry with the current agent version

Registry authentication is controlled via the DD_INSTALLER_REGISTRY_AUTH
environment variable (docker, gcr, or password).

Examples:
  # Standard Datadog registry, version inferred from installed agent
  datadog-agent ddot install

  # Custom BYOC registry, version inferred from installed agent
  DD_INSTALLER_REGISTRY_AUTH=gcr datadog-agent ddot install --registry registry.example.com`,
		Args: cobra.NoArgs,
		RunE: func(_ *cobra.Command, _ []string) error {
			url, err := resolvePackageURL(explicitURL, registry)
			if err != nil {
				return err
			}
			i, err := newInstallerExec()
			if err != nil {
				return err
			}
			return i.InstallExtensions(context.Background(), url, []string{extensionName})
		},
	}
	cmd.Flags().StringVar(&explicitURL, "url", "", "full OCI package URL (e.g. oci://registry.example.com/agent-package:7.78.0-1)")
	cmd.Flags().StringVar(&registry, "registry", "", "OCI registry base for a custom package; the current agent version is appended automatically")
	cmd.MarkFlagsMutuallyExclusive("url", "registry")
	return cmd
}

func removeCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "remove",
		Short: "Remove the DDOT installation",
		Long: `Remove the DDOT installation.

Example:
  datadog-agent ddot remove`,
		Args: cobra.NoArgs,
		RunE: func(_ *cobra.Command, _ []string) error {
			i, err := newInstallerExec()
			if err != nil {
				return err
			}
			return i.RemoveExtensions(context.Background(), "datadog-agent", []string{extensionName})
		},
	}
}

// resolvePackageURL determines the OCI URL for the agent package:
//  1. explicit --url flag takes precedence
//  2. --registry flag: construct oci://<registry>/agent-package:<version>
//  3. neither: use default Datadog registry via oci.PackageURL
func resolvePackageURL(explicitURL, registry string) (string, error) {
	if explicitURL != "" {
		return explicitURL, nil
	}

	version := currentAgentVersion()

	if registry != "" {
		base := strings.TrimSuffix(strings.TrimPrefix(registry, "oci://"), "/")
		return fmt.Sprintf("oci://%s/agent-package:%s", base, version), nil
	}

	return oci.PackageURL(env.FromEnv(), "datadog-agent", version), nil
}

// currentAgentVersion returns the agent version in the format expected by OCI
// package URLs (URL-safe, with a -1 release suffix).
// NOTE: duplicated from pkg/fleet/installer/packages/datadog_agent_extensions.go
// (getCurrentAgentVersion). Keep both in sync until a shared helper is extracted.
func currentAgentVersion() string {
	v := pkgversion.AgentVersionURLSafe
	if strings.HasSuffix(v, "-1") {
		return v
	}
	return v + "-1"
}

// newInstallerExec resolves the installer binary path and returns an InstallerExec.
func newInstallerExec() (*exec.InstallerExec, error) {
	binPath, err := resolveInstallerBinPath()
	if err != nil {
		return nil, err
	}
	return exec.NewInstallerExec(env.FromEnv(), binPath), nil
}

// resolveInstallerBinPath finds the installer binary co-located with the agent.
func resolveInstallerBinPath() (string, error) {
	agentExe, err := exec.GetExecutable()
	if err != nil {
		return "", fmt.Errorf("could not get agent executable path: %w", err)
	}
	agentExe, err = filepath.EvalSymlinks(agentExe)
	if err != nil {
		return "", fmt.Errorf("could not resolve agent executable path: %w", err)
	}
	return installerBinFromAgentExe(agentExe)
}

// installerBinFromAgentExe computes and verifies the installer binary path from
// the resolved agent executable path.
//
// The agent binary sits at <root>/bin/agent/agent[.exe], where <root> is two
// directories up. Installer locations differ by OS:
//
//   - Linux/macOS: <root>/embedded/bin/installer
//   - Windows:     <InstallPath>/bin/datadog-installer.exe
//
// This layout holds for both OCI packages (/opt/datadog-packages/datadog-agent/<version>/)
// and deb/rpm installs (/opt/datadog-agent/).
func installerBinFromAgentExe(agentExe string) (string, error) {
	var installerBin string
	if runtime.GOOS == "windows" {
		installerBin = filepath.Join(defaultpaths.GetInstallPath(), "bin", "datadog-installer.exe")
	} else {
		root := filepath.Clean(filepath.Join(filepath.Dir(agentExe), "..", ".."))
		installerBin = filepath.Join(root, "embedded", "bin", "installer")
	}

	if _, err := os.Stat(installerBin); err != nil {
		return "", fmt.Errorf(
			"installer binary not found at %s: ensure the agent was installed via the Datadog installer",
			installerBin,
		)
	}
	return installerBin, nil
}
