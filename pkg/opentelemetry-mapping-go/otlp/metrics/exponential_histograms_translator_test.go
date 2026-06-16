package metrics

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/pdata/pmetric"
)

func TestExponentialHistogramDropsUnsupportedBucketBounds(t *testing.T) {
	md := pmetric.NewMetrics()
	metric := md.ResourceMetrics().AppendEmpty().ScopeMetrics().AppendEmpty().Metrics().AppendEmpty()
	metric.SetName("test")
	histogram := metric.SetEmptyExponentialHistogram()
	histogram.SetAggregationTemporality(pmetric.AggregationTemporalityDelta)

	unsupportedPoint := histogram.DataPoints().AppendEmpty()
	unsupportedPoint.SetCount(44)
	unsupportedPoint.SetSum(-44)
	unsupportedPoint.SetScale(-4)
	unsupportedPoint.Negative().SetOffset(64)
	unsupportedPoint.Negative().BucketCounts().Append(44)

	validPoint := histogram.DataPoints().AppendEmpty()
	validPoint.SetCount(1)
	validPoint.SetSum(2)
	validPoint.SetScale(0)
	validPoint.Positive().BucketCounts().Append(1)

	translator := NewTestTranslator(t)
	consumer := newTestConsumer()

	require.NotPanics(t, func() {
		_, err := translator.MapMetrics(context.Background(), md, &consumer, nil)
		require.NoError(t, err)
	})
	require.Len(t, consumer.data.Metrics.Sketches, 1)
	require.Equal(t, "test", consumer.data.Metrics.Sketches[0].Name)
}
