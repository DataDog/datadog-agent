// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build tools

// Package rshelldeps retains dependencies used only by the standalone rshell
// command built by tasks/rshell.py. The Agent imports rshell's library
// packages, but not cmd/rshell, so these dependencies would otherwise be
// removed by go mod tidy and omitted from license generation.
package rshelldeps

import _ "github.com/landlock-lsm/go-landlock/landlock/syscall"
