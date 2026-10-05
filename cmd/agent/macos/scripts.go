// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package macos exposes the macOS Agent scripts to Go consumers that need to
// ship them outside of the repository checkout.
package macos

import "embed"

// UninstallScriptPath is the path of the Agent uninstall script in Scripts.
const UninstallScriptPath = "uninstall_mac_os.sh"

// Scripts contains the macOS Agent scripts needed by Go consumers.
//
//go:embed uninstall_mac_os.sh
var Scripts embed.FS
