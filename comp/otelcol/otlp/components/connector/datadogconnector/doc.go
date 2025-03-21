// Copyright The OpenTelemetry Authors
// SPDX-License-Identifier: Apache-2.0

//go:generate mdatagen metadata.yaml

// Package datadogconnector contains a connector component that derives APM statistics, in the form of metrics, from service traces, for display in the Datadog APM product. This component is required for trace-emitting services and their statistics to appear in Datadog APM.
package datadogconnector // import "github.com/DataDog/datadog-agent/comp/otelcol/otlp/components/connector/datadogconnector"
