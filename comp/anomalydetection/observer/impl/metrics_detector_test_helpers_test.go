// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package observerimpl

import observer "github.com/DataDog/datadog-agent/comp/anomalydetection/observer/def"

type manualSeriesRemover interface {
	RemoveSeries([]observer.SeriesRef)
}
