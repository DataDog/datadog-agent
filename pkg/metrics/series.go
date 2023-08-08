// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package metrics

import (
	"github.com/DataDog/datadog-agent/pkg/metrics/model"
)

// Point represents a metric value at a specific time
type Point = model.Point

// Resource holds a resource name and type
type Resource = model.Resource

// Serie holds a timeseries (w/ json serialization to DD API format)
type Serie = model.Serie

// Series is a collection of `Serie`
type Series = model.Series

// SerieSink is a sink for series.
// It provides a way to append a serie into `Series` or `IterableSerie`
type SerieSink = model.SerieSink
