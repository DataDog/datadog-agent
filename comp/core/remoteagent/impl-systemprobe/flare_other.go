// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build !linux

package systemprobeimpl

import log "github.com/DataDog/datadog-agent/comp/core/log/def"

// collectRuntimeSecurityKernelArtifacts is a no-op on non-Linux platforms:
// CWS only runs on Linux, and /proc and tracefs are Linux-only.
func collectRuntimeSecurityKernelArtifacts(_ log.Component, _ map[string][]byte) {}
