// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package bench

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestCardinalityCounter(t *testing.T) {
	t.Run("exact below threshold", func(t *testing.T) {
		counter := newCardinalityCounter()
		counter.Add(metricSeriesHash("cpu", []string{"host:a"}))
		counter.Add(metricSeriesHash("cpu", []string{"host:a"}))
		counter.Add(metricSeriesHash("cpu", []string{"host:b"}))
		require.Equal(t, 2, counter.Count())
		require.Nil(t, counter.registers)
	})

	t.Run("bounded sketch above threshold", func(t *testing.T) {
		counter := newCardinalityCounter()
		const want = 100_000
		for i := 0; i < want; i++ {
			counter.Add(metricSeriesHash("cpu", []string{fmt.Sprintf("host:%d", i)}))
		}
		require.Nil(t, counter.exact)
		require.Len(t, counter.registers, hllRegisterCount)
		require.InDelta(t, want, counter.Count(), want*0.03)
	})
}
