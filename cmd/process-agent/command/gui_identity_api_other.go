// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build !windows

package command

import "github.com/DataDog/datadog-agent/comp/core/config"

// shouldServeGUIIdentityAPI is always false off Windows: the GUI resolves peer identity in-process
// (Linux /proc/net/tcp, Darwin sysctl) without process-agent's help, and the /pid/{pid}/sid API only
// exists on Windows (see cmd/process-agent/api/server_other.go). So there is never a reason to keep
// process-agent alive for the GUI on these platforms.
func shouldServeGUIIdentityAPI(_ config.Component) bool {
	return false
}
