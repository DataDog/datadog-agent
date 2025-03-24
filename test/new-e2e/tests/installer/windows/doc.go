// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package installer contains code for the E2E tests for the Datadog installer/Fleet Automation/Remote Upgrades on Windows.
//
// This package provides utilities and test suites to validate the installation, uninstallation,
// and upgrade processes of the Datadog Agent and related components on Windows systems.
//
// # Running Tests with Pipeline Artifacts
//
// To run the tests using artifacts from a specific pipeline, set the following environment variables:
//
//  E2E_PIPELINE_ID=<pipeline_id>
//  CURRENT_AGENT_VERSION=<agent_version>
//  STABLE_AGENT_VERSION_PACKAGE=<stable_agent_version_package>
//
// Example:
//
//  E2E_PIPELINE_ID=40537701
//  CURRENT_AGENT_VERSION=7.66.0-devel
//  STABLE_AGENT_VERSION_PACKAGE=7.65.0-1
//
// # Running Tests with Local Artifacts
//
// To run the tests using local artifacts, set one or more the following environment variables:
//
//  CURRENT_AGENT_MSI_URL=file:///path/to/agent.msi
//  STABLE_AGENT_OCI_URL=file:///path/to/oci/package.tar
//
// See `WithDevEnvOverrides()` and `WithMSIDevEnvOverrides()` for more information.
//
// # Contents Overview
//
// Files Overview:
//
// - install_script.go: Contains the `DatadogInstallScript` struct and methods to run the Datadog Install script on a remote Windows host.
// - installer.go: Contains the `DatadogInstaller` struct and methods to manage the Datadog Installer executable on a remote Windows host, including installation, uninstallation, and package management.
//
// Test Suites Overview:
//
// @team/windows-agent
// - suites/installer-package: (deprecated) Old test suite for the Installer MSI. Contents should be eventually be moved to the Agent MSI tests.
// - suites/agent-package Tests remote upgrade and MSI operations of the Datadog Agent package using the Datadog installer.
// APM and @team/windows-agent
// - suites/apm-library-dotnet-package: Tests the .NET APM Library for IIS package through remote upgrades and the Agent MSI
package installer
