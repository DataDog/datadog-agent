// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build !ndm

package run

import "go.uber.org/fx"

// The IoT Agent has no use for Network Device Monitoring, so neither the
// Remote Configuration provider nor the discovery component is registered.
// Leaving both out keeps the sweep, the probing packages and the SNMP
// dependency out of that binary.
func ndmRemoteConfigModules() fx.Option {
	return fx.Options()
}
