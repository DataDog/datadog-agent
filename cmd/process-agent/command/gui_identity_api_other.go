// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build !windows

package command

import "github.com/DataDog/datadog-agent/comp/core/config"

// shouldServeGUIIdentityAPI is always false off Windows: the GUI resolves peer identity in-process and the /connection/owner-sid API is Windows-only, so there is no reason to keep process-agent alive for it.
func shouldServeGUIIdentityAPI(_ config.Component) bool {
	return false
}
