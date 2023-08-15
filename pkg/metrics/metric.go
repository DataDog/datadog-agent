// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package metrics

import (
	"github.com/DataDog/datadog-agent/pkg/metrics/model"
)

// APIMetricType represents an API metric type
type APIMetricType = model.APIMetricType

// Enumeration of the existing API metric types
const (
	APIGaugeType = model.APIGaugeType
	APIRateType  = model.APIRateType
	APICountType = model.APICountType
)

// NoSerieError is the error returned by a metric when not enough samples have been
// submitted to generate a serie
type NoSerieError = model.NoSerieError
