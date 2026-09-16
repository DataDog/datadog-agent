// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build windows

package command

import "github.com/DataDog/datadog-agent/comp/core/config"

// shouldServeGUIIdentityAPI reports whether process-agent should stay alive to serve its
// /pid/{pid}/sid API even when it has no checks of its own to run. On Windows the GUI (running in
// the core agent as the lower-privileged ddagentuser) depends on that API to bind intent tokens to
// the caller's OS identity (CWE-214). It is needed exactly when the GUI itself is enabled, which is
// whenever GUI_port is not the "-1" disabled sentinel.
func shouldServeGUIIdentityAPI(cfg config.Component) bool {
	return cfg.GetString("GUI_port") != "-1"
}
