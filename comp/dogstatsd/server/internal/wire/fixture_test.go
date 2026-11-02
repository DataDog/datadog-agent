// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package wire

import (
	"testing"

	noops "github.com/DataDog/datadog-agent/comp/core/telemetry/impl/noops"
)

func testOptions() Options {
	return Options{InternerSize: 4096, ReadTimestamps: true, OriginDetection: false}
}

func testParser(t testing.TB, options Options) *Parser {
	t.Helper()
	telemetry := noops.GetCompatComponent()
	return NewParser(options, NewFloat64ListPool(false, telemetry), 1, NewInternerTelemetry(false, telemetry))
}
