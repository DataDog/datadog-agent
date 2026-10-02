// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build !windows && !darwin

package command

import "fmt"

func captureSupported() error {
	return fmt.Errorf("capture requires a real Windows or macOS device; this host may replay separately supplied bundles")
}
