// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build !ndm

package run

import "go.uber.org/fx"

// The IoT Agent ships no Private Action Runner, so nothing can reach the
// connectivity check endpoint. Leaving the component out keeps the probing
// packages and their SNMP dependency out of that binary.
func ndmConnectivityCheckModule() fx.Option {
	return fx.Options()
}
