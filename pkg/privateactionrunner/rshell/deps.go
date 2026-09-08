// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build tools

// Package rshelldeps retains the standalone rshell command's product-only
// dependencies so go mod tidy and license generation include them. The Agent
// imports rshell's libraries, while Bazel builds the command itself.
package rshelldeps

import _ "github.com/landlock-lsm/go-landlock/landlock/syscall"
