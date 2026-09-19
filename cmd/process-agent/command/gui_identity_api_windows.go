// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build windows

package command

import "github.com/DataDog/datadog-agent/comp/core/config"

// shouldServeGUIIdentityAPI reports whether process-agent must stay alive to serve /connection/owner-sid, which the Windows GUI (ddagentuser) uses to bind intent tokens to the caller's OS identity (CWE-214); needed whenever the GUI is enabled, i.e. GUI_port is not the "-1" disabled sentinel.
func shouldServeGUIIdentityAPI(cfg config.Component) bool {
	return cfg.GetString("GUI_port") != "-1"
}
