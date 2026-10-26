// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package commands

// defaultAgentConfigDir is where the macOS Agent keeps datadog.yaml. The installer reads the API
// key and site from it for its own telemetry, and the installer.registry settings, as it does on
// Linux. Kept to this package: paths.AgentConfigDir also backs the config experiment commands,
// which aren't wired on macOS.
const defaultAgentConfigDir = "/opt/datadog-agent/etc"
