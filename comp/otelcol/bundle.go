// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package otelcol contains the OTLP ingest bundle pipeline to be included
// into the agent components.
package otelcol

import (
	"github.com/DataDog/datadog-agent/comp/otelcol/collector"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

// team: opentelemetry

// Bundle specifies the bundle for the OTLP ingest pipeline.
var Bundle = fxutil.Bundle(
	collector.Module,
)
