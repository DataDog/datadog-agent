// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build !windows

package du_test

// canSymlink reports whether the test environment can create symbolic
// links. On Unix this is always true (any user can create symlinks).
func canSymlink() bool { return true }
