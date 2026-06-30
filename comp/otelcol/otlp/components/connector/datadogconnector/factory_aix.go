// Copyright The OpenTelemetry Authors
// SPDX-License-Identifier: Apache-2.0

//go:build aix

package datadogconnector

import "go.opentelemetry.io/collector/connector"

// NewFactory panics on AIX, which the Datadog connector does not support.
//
// The signature intentionally differs from the non-AIX NewFactory: the embedded
// (DDOT) Collector that injects the Agent dependencies is not built on AIX, and
// keeping this stub import-minimal avoids pulling in apmstats, which does not
// build on AIX. This mirrors the upstream connector/datadogconnector stub.
func NewFactory() connector.Factory {
	panic("aix is not supported")
}
