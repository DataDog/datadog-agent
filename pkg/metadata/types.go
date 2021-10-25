// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package metadata

import (
	"context"

	"github.com/DataDog/datadog-agent/pkg/serializer"
)

// Collector is anything capable to collect and send metadata payloads
// through the forwarder.
// A Metadata Collector normally uses a Metadata Provider to fill the payload.
type Collector interface {
	Send(ctx context.Context, s serializer.MetricSerializer) error
}

// CollectorWithInit is an optional interface that collectors that need to be
// initialized can implement. If implemented, the Init method will be called
// when the collector is scheduled
type CollectorWithInit interface {
	Init() error
}
