// Copyright The OpenTelemetry Authors
// SPDX-License-Identifier: Apache-2.0

// Package datadogconnector provides the Datadog connector for the embedded
// (DDOT) Collector.
//
// The connector derives APM stats from service traces (traces-to-metrics) and
// can forward traces to another pipeline (traces-to-traces). Its configuration
// schema and feature-gate definitions come from the pkg/datadog module.
package datadogconnector
