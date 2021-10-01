// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2021-present Datadog, Inc.

// Package model provides utilities to transform from the OpenTelemetry OTLP data model to
// the Datadog Agent data model.
//
// This module is used in the Datadog Agent and the OpenTelemetry Collector Datadog exporter.
// End-user behavior is stable, but there are no stability guarantees on its public Go API.
// Nonetheless, if editing try to avoid breaking API changes if possible and double check
// the API usage on all module dependents.
//
// The 'attributes' packages provide utilities for semantic conventions mapping, while the
// translator model translates telemetry signals (currently only metrics are translated).
package model
