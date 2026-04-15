// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package extension implements 'agent extension'.
package extension

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
	pkgversion "github.com/DataDog/datadog-agent/pkg/version"
)

// Commands returns a slice of subcommands for the 'agent' command.
func Commands(_ *command.GlobalParams) []*cobra.Command {
	extensionCmd := &cobra.Command{
		Use:   "extension [command]",
		Short: "Manage agent package extensions",
	}
	extensionCmd.AddCommand(installCommand(), removeCommand())
	return []*cobra.Command{extensionCmd}
}

func installCommand() *cobra.Command {
	var explicitURL string
	var registry string

	cmd := &cobra.Command{
		Use:   "install <extension> [extensions...]",
		Short: "Install one or more extensions from an OCI package",
		Long: `Install one or more extensions from an OCI package.

The package URL is determined as follows (in order of precedence):
  1. --url flag: use the provided full OCI URL
  2. --registry flag: construct oci://<registry>/agent-package:<current-version>
  3. No flags: use the default Datadog registry with the current agent version

The agent must have been installed via the Datadog installer (OCI or deb/rpm).
Registry authentication is controlled via the DD_INSTALLER_REGISTRY_AUTH
environment variable (docker, gcr, or password).

Examples:
  # Standard Datadog registry, version inferred from installed agent
  datadog-agent extension install ddot

  # Custom BYOC registry, version inferred from installed agent
  DD_INSTALLER_REGISTRY_AUTH=gcr datadog-agent extension install --registry registry.example.com ddot

  # Explicit full URL
  datadog-agent extension install --url oci://registry.example.com/agent-package:7.78.0-1 ddot`,
		Args: cobra.MinimumNArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			url, err := resolvePackageURL(explicitURL, registry)
			if err != nil {
				return err
			}
			i, err := newInstallerExec()
			if err != nil {
				return err
			}
			return i.InstallExtensions(context.Background(), url, args)
		},
	}
	cmd.Flags().StringVar(&explicitURL, "url", "", "full OCI package URL (e.g. oci://registry.example.com/agent-package:7.78.0-1)")
	cmd.Flags().StringVar(&registry, "registry", "", "OCI registry base for a custom package; the current agent version is appended automatically")
	cmd.MarkFlagsMutuallyExclusive("url", "registry")
	return cmd
}

func removeCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "remove <extension> [extensions...]",
		Short: "Remove one or more installed extensions",
		Long: `Remove one or more installed extensions from the agent package.

Example:
  datadog-agent extension remove ddot`,
		Args: cobra.MinimumNArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			i, err := newInstallerExec()
			if err != nil {
				return err
			}
			return i.RemoveExtensions(context.Background(), "datadog-agent", args)
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
//   - Windows:     <root>/datadog-installer.exe
//
// This layout holds for both OCI packages (/opt/datadog-packages/datadog-agent/<version>/)
// and deb/rpm installs (/opt/datadog-agent/).
func installerBinFromAgentExe(agentExe string) (string, error) {
	root := filepath.Clean(filepath.Join(filepath.Dir(agentExe), "..", ".."))

	var installerBin string
	if runtime.GOOS == "windows" {
		installerBin = filepath.Join(root, "datadog-installer.exe")
	} else {
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
