// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package installer contains code for the E2E tests for the Datadog installer/Fleet Automation/Remote Upgrades on Windows.
//
// This package provides utilities and test suites to validate the installation, uninstallation,
// and upgrade processes of the Datadog Agent and related components on Windows systems.
//
// # Quick Start with setup-env Helper
//
//	Configure environment variables for local testing with the setup-env helper:
//
// PowerShell:
//
//	dda inv new-e2e-tests.setup-env --build local --fmt powershell | Invoke-Expression
//
// Bash/WSL:
//
//	eval "$(dda inv new-e2e-tests.setup-env --build local)"
//
// This automatically detects local MSI/OCI builds in omnibus/pkg/ and sets the appropriate environment variables.
//
// ## Using Pipeline Artifacts
//
// To use artifacts from a CI pipeline (requires GITLAB_TOKEN environment variable):
//
// PowerShell:
//
//	$env:GITLAB_TOKEN="your-token"
//	dda inv new-e2e-tests.setup-env --build pipeline --fmt powershell | Invoke-Expression
//
// Bash/WSL:
//
//	export GITLAB_TOKEN="your-token"
//	eval "$(dda inv new-e2e-tests.setup-env --build pipeline)"
//
// This auto-detects the most recent successful pipeline on your current branch and sets E2E_PIPELINE_ID
// along with the version variables.
//
// To use a specific pipeline:
//
//	dda inv new-e2e-tests.setup-env --build pipeline --pipeline-id 40537701 --fmt powershell | Invoke-Expression
//
// Options:
//   - --build local: Use local builds from omnibus/pkg/
//   - --build pipeline: Use CI pipeline artifacts (requires GITLAB_TOKEN)
//   - --fmt bash|powershell|json: Output format (default: bash)
//   - --pkg <name>: Specify a particular Local MSI package to use for --build local
//   - --pipeline-id <id>: Override pipeline ID for --build pipeline
//
// # Running Tests with Pipeline Artifacts
//
// To run the tests using artifacts from a specific pipeline, set the following environment variables:
//
//	E2E_PIPELINE_ID=<pipeline_id>
//	CURRENT_AGENT_VERSION=<agent_version>
//	STABLE_AGENT_VERSION=<stable_agent_version>
//
// Example:
//
//	E2E_PIPELINE_ID="40537701"
//	CURRENT_AGENT_VERSION="7.66.0-devel"
//	CURRENT_AGENT_VERSION_PACKAGE="7.66.0-devel.git.53.db3f37e.pipeline.1234-1"
//	STABLE_AGENT_VERSION="7.65.0"
//	STABLE_AGENT_VERSION_PACKAGE="7.65.0-1"
//
// VERSION is used for comparing with the output of the `version` subcommand.
//
// VERSION_PACKAGE is used for comparing with the Fleet package stable/experiment status
//
// This assertion is currently a "contains" check, so the package version can be
// shortened to 7.66.0-devel for convenience.
// With local testing the _VERSION_PACKAGE variables can be omitted, though they are required in the CI.
//
// # Building Local Artifacts
//
// To build the MSI installer locally:
//
//	dda inv msi.build
//
// This creates an MSI in omnibus/pkg/ (e.g., datadog-agent-7.77.0-devel.git.32.ce3a7fe-1-x86_64.msi).
//
// To build an OCI package from the MSI (required for Fleet/Remote Upgrade tests):
//
//	dda inv msi.package-oci
//
// This creates an OCI tar file in omnibus/pkg/ (e.g., datadog-agent-7.77.0-devel.git.32.ce3a7fe-1-windows-amd64.oci.tar).
//
// Note: The OCI packaging requires the `datadog-package` tool. Install it with:
//
//	go install github.com/DataDog/datadog-package@latest
//
// You can also specify a specific MSI to package:
//
//	dda inv msi.package-oci --msi-path=omnibus/pkg/datadog-agent-7.77.0-devel.git.32.ce3a7fe-1-x86_64.msi
//
// # Running Tests with Local Artifacts
//
// To run the tests using local artifacts, set one or more the following environment variables:
//
//	CURRENT_AGENT_MSI_URL="file:///path/to/agent.msi"
//	STABLE_AGENT_OCI_URL="file:///path/to/oci/package.tar"
//
// See `WithDevEnvOverrides()` here for more OCI options and and in `common/agent/` for more MSI options.
//
// # Contents Overview
//
// Files Overview:
//   - install_script.go: Contains the `DatadogInstallScript` struct and methods to run the Datadog Install script on a remote Windows host.
//   - installer.go: Contains the `DatadogInstaller` struct and methods to manage the Datadog Installer executable on a remote Windows host, including installation, uninstallation, and package management.
//
// Test Suites Overview:
//
// @team/windows-products
//   - suites/installer-package: (deprecated) Old test suite for the Installer MSI. Contents should be eventually be moved to the Agent MSI tests.
//   - suites/agent-package Tests remote upgrade and MSI operations of the Datadog Agent package using the Datadog installer.
//
// APM and @team/windows-products
//   - suites/apm-library-dotnet-package: Tests the .NET APM Library for IIS package through remote upgrades and the Agent MSI
package installer
