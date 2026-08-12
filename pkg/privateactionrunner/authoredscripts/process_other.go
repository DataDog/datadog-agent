// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build !linux

package authoredscripts

import "os/exec"

// The temporary authored-script catalog only publishes Linux variants.
func configureCommand(_ *exec.Cmd) {}

func terminateCommand(_ *exec.Cmd) error { return nil }
