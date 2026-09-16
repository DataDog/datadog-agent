// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build !windows

package guiimpl

import (
	ipc "github.com/DataDog/datadog-agent/comp/core/ipc/def"
	pkgconfigmodel "github.com/DataDog/datadog-agent/pkg/config/model"
)

// configurePeerIdentityResolution is a no-op on this platform: only Windows's sidForPID (see
// peeridentity_windows.go) needs the agent-wide IPC client and config to call process-agent.
func configurePeerIdentityResolution(_ ipc.Component, _ pkgconfigmodel.Reader) {}
