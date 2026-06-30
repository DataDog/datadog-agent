// Copyright The OpenTelemetry Authors
// SPDX-License-Identifier: Apache-2.0

// Package datadogconnector provides the Datadog connector for the embedded
// (DDOT) Collector.
//
// It is a local copy of opentelemetry-collector-contrib's
// connector/datadogconnector factory together with the connector
// implementation from pkg/datadog/apmstats, vendored into the Agent so the
// embedded Collector no longer depends on contrib for the connector logic. The
// config schema and feature-gate definitions are still shared with upstream via
// the pkg/datadog module.
package datadogconnector
