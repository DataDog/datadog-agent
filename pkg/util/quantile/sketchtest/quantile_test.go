// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2022-present Datadog, Inc.
package sketchtest

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestQuantileCDFInverses(t *testing.T) {
	for _, tt := range []struct {
		name string
		cdf  CDF
		qf   QuantileFunction
	}{
		{
			name: "Exponential(lambda=1/100)",
			cdf:  ExponentialCDF(1 / 100.0),
			qf:   ExponentialQ(1 / 100.0),
		},
		{
			name: "Exponential(lambda=1/200)",
			cdf:  ExponentialCDF(1 / 200.0),
			qf:   ExponentialQ(1 / 200.0),
		},
		{
			name: "Normal(0, 1)",
			cdf:  NormalCDF(0, 1),
			qf:   NormalQ(0, 1),
		},
		{
			name: "Normal(-3, 10)",
			cdf:  NormalCDF(-3, 10),
			qf:   NormalQ(-3, 10),
		},
		{
			name: "Truncated normal (0, 1e-3)",
			cdf:  TruncateCDF(-8, 8, NormalCDF(0, 1e-3)),
			qf:   TruncateQ(-8, 8, NormalQ(0, 1e-3), NormalCDF(0, 1e-3)),
		},
	} {
		for i := 0; i <= 100; i++ {
			assert.InDelta(t, tt.cdf(tt.qf(float64(i)/100.0)), float64(i)/100.0, 1e-15)
		}
	}
}
