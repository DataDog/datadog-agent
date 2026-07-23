// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package agenttelemetryimpl

import (
	"testing"

	dto "github.com/prometheus/client_model/go"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"
)

func TestAggregateMetricHistogramDoesNotAliasSources(t *testing.T) {
	newHistogramMetric := func(sampleCount, cumulativeCount uint64, exemplarValue float64, timestampSeconds int64) *dto.Metric {
		upperBound := 10.0
		labelName := "trace_id"
		labelValue := "abc123"

		return &dto.Metric{
			Histogram: &dto.Histogram{
				SampleCount: &sampleCount,
				Bucket: []*dto.Bucket{
					{
						CumulativeCount: &cumulativeCount,
						UpperBound:      &upperBound,
						Exemplar: &dto.Exemplar{
							Label: []*dto.LabelPair{
								{Name: &labelName, Value: &labelValue},
							},
							Value:     &exemplarValue,
							Timestamp: &timestamppb.Timestamp{Seconds: timestampSeconds, Nanos: 123},
						},
					},
				},
			},
		}
	}

	src1 := newHistogramMetric(3, 2, 1.5, 100)
	src2 := newHistogramMetric(5, 4, 2.5, 200)
	src1Snapshot := proto.Clone(src1).(*dto.Metric)
	src2Snapshot := proto.Clone(src2).(*dto.Metric)

	total := &dto.Metric{}
	group := &dto.Metric{}
	aggregateMetric(dto.MetricType_HISTOGRAM, total, src1)
	aggregateMetric(dto.MetricType_HISTOGRAM, group, src1)
	aggregateMetric(dto.MetricType_HISTOGRAM, total, src2)

	require.Equal(t, uint64(8), total.GetHistogram().GetSampleCount())
	require.Equal(t, uint64(6), total.GetHistogram().GetBucket()[0].GetCumulativeCount())
	require.Equal(t, uint64(3), group.GetHistogram().GetSampleCount())
	require.Equal(t, uint64(2), group.GetHistogram().GetBucket()[0].GetCumulativeCount())
	require.Empty(t, total.GetHistogram().GetBucket()[0].GetExemplar().GetLabel())
	require.Empty(t, group.GetHistogram().GetBucket()[0].GetExemplar().GetLabel())
	require.Equal(t, 1.5, total.GetHistogram().GetBucket()[0].GetExemplar().GetValue())
	require.Equal(t, int64(100), total.GetHistogram().GetBucket()[0].GetExemplar().GetTimestamp().GetSeconds())
	require.True(t, proto.Equal(src1Snapshot, src1), "first source histogram was mutated")
	require.True(t, proto.Equal(src2Snapshot, src2), "second source histogram was mutated")

	requireHistogramPointersIndependent(t, total.GetHistogram(), src1.GetHistogram())
	requireHistogramPointersIndependent(t, total.GetHistogram(), src2.GetHistogram())
	requireHistogramPointersIndependent(t, group.GetHistogram(), src1.GetHistogram())
	requireHistogramPointersIndependent(t, total.GetHistogram(), group.GetHistogram())

	groupSnapshot := proto.Clone(group).(*dto.Metric)
	*total.Histogram.SampleCount = 80
	*total.Histogram.Bucket[0].CumulativeCount = 60
	*total.Histogram.Bucket[0].UpperBound = 20
	*total.Histogram.Bucket[0].Exemplar.Value = 3.5
	total.Histogram.Bucket[0].Exemplar.Timestamp.Seconds = 300

	require.True(t, proto.Equal(src1Snapshot, src1), "mutating aggregate changed first source histogram")
	require.True(t, proto.Equal(src2Snapshot, src2), "mutating aggregate changed second source histogram")
	require.True(t, proto.Equal(groupSnapshot, group), "mutating total aggregate changed group aggregate")
}

func requireHistogramPointersIndependent(t *testing.T, left, right *dto.Histogram) {
	t.Helper()
	require.NotNil(t, left)
	require.NotNil(t, right)
	require.NotSame(t, left.SampleCount, right.SampleCount)
	require.Len(t, left.Bucket, 1)
	require.Len(t, right.Bucket, 1)
	require.NotSame(t, left.Bucket[0], right.Bucket[0])
	require.NotSame(t, left.Bucket[0].CumulativeCount, right.Bucket[0].CumulativeCount)
	require.NotSame(t, left.Bucket[0].UpperBound, right.Bucket[0].UpperBound)
	require.NotNil(t, left.Bucket[0].Exemplar)
	require.NotNil(t, right.Bucket[0].Exemplar)
	require.NotSame(t, left.Bucket[0].Exemplar, right.Bucket[0].Exemplar)
	require.NotSame(t, left.Bucket[0].Exemplar.Value, right.Bucket[0].Exemplar.Value)
	require.NotNil(t, left.Bucket[0].Exemplar.Timestamp)
	require.NotNil(t, right.Bucket[0].Exemplar.Timestamp)
	require.NotSame(t, left.Bucket[0].Exemplar.Timestamp, right.Bucket[0].Exemplar.Timestamp)
}
