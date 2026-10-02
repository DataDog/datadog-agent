// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package serializer

import (
	"github.com/DataDog/datadog-agent/pkg/metrics"
	"github.com/DataDog/datadog-agent/pkg/serializer/marshaler"
)

// CaptureTransformer is an optional interception point before serialization.
// It must consume metric sources completely, including on failure, to release
// producer goroutines. Returned samples must not share mutable data with the
// originals. Errors must contain no raw telemetry. Normal Agents omit it.
type CaptureTransformer interface {
	Series(metrics.SerieSource) (metrics.SerieSource, error)
	HostMetadata(marshaler.JSONMarshaler) (marshaler.JSONMarshaler, error)
}
