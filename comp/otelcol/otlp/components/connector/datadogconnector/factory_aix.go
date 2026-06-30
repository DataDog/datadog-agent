// Copyright The OpenTelemetry Authors
// SPDX-License-Identifier: Apache-2.0

//go:build aix

package datadogconnector

import "go.opentelemetry.io/collector/connector"

// NewFactory panics on AIX, which the Datadog connector does not support.
//
// The parameter count must match the non-AIX NewFactory: callers such as
// comp/otelcol/collector/impl/collector.go are gated on the otlp build tag
// only (not !aix), so this stub is still compiled into AIX builds and must
// satisfy the same call sites. Parameters are typed `any` rather than the
// real types (types.TaggerClient, SourceProviderFunc, *stats.Concentrator)
// so this file doesn't need to import the connector implementation packages.
func NewFactory(_, _, _ any) connector.Factory {
	panic("aix is not supported")
}
