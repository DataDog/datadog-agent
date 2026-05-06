// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Stub for builds that do not include the observer component (e.g. IoT agent).
// See command_observer.go for the full implementation and build tag rationale.

//go:build !python

package run

import "go.uber.org/fx"

func getObserverOptions() fx.Option {
	return fx.Options()
}
