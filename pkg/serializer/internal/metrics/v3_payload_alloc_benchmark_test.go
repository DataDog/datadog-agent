// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build test

package metrics

import (
	"fmt"
	"testing"

	pkgmetrics "github.com/DataDog/datadog-agent/pkg/metrics"
	"github.com/DataDog/datadog-agent/pkg/tagset"
	noopimpl "github.com/DataDog/datadog-agent/pkg/util/compression/impl-noop"
)

var benchmarkV3PayloadBytes int

const benchmarkV3PayloadRows = 8192

func benchmarkV3PipelineConfig() PipelineConfig {
	return PipelineConfig{Filter: AllowAllFilter{}, V3: true}
}

func benchmarkV3Rows(rows int, uniqueIdentity bool) ([]pkgmetrics.V3MetricPointRow, []pkgmetrics.SerieRow, pkgmetrics.Series) {
	pointRows := make([]pkgmetrics.V3MetricPointRow, rows)
	serieRows := make([]pkgmetrics.SerieRow, rows)
	series := make(pkgmetrics.Series, rows)

	for i := 0; i < rows; i++ {
		identityID := i
		if !uniqueIdentity {
			identityID = i % 128
		}

		tags := tagset.CompositeTagsFromSlice([]string{
			fmt.Sprintf("env:bench-%d", identityID%4),
			fmt.Sprintf("service:svc-%d", identityID%64),
			fmt.Sprintf("pod:pod-%d", identityID),
			fmt.Sprintf("az:az-%d", identityID%3),
			fmt.Sprintf("team:team-%d", identityID%32),
			fmt.Sprintf("version:%d", identityID%256),
		})
		name := fmt.Sprintf("bench.metric.%d", identityID)
		host := fmt.Sprintf("host-%d", identityID%256)
		value := float64((i % 1000) + 1)
		timestamp := int64(1756737000 + i%10)

		pointRows[i] = pkgmetrics.V3MetricPointRow{
			Name:           name,
			Timestamp:      timestamp,
			Value:          value,
			Tags:           tags,
			Host:           host,
			MType:          pkgmetrics.APICountType,
			Interval:       10,
			SourceTypeName: "dogstatsd",
			Source:         pkgmetrics.MetricSourceDogstatsd,
		}

		points := []pkgmetrics.Point{{Ts: float64(timestamp), Value: value}}
		serieRows[i] = pkgmetrics.SerieRow{
			Name:           name,
			Points:         points,
			Tags:           tags,
			Host:           host,
			MType:          pkgmetrics.APICountType,
			Interval:       10,
			SourceTypeName: "dogstatsd",
			Source:         pkgmetrics.MetricSourceDogstatsd,
		}
		series[i] = &pkgmetrics.Serie{
			Name:           name,
			Points:         points,
			Tags:           tags,
			Host:           host,
			MType:          pkgmetrics.APICountType,
			Interval:       10,
			SourceTypeName: "dogstatsd",
			Source:         pkgmetrics.MetricSourceDogstatsd,
		}
	}

	return pointRows, serieRows, series
}

func benchmarkNewV3PayloadBuilder(ctx *PipelineContext) (*payloadsBuilderV3, error) {
	return newPayloadsBuilderV3(
		64*1024*1024,
		256*1024*1024,
		10_000_000,
		noopimpl.New(),
		benchmarkV3PipelineConfig(),
		ctx,
	)
}

func BenchmarkV3PayloadBuilderAllocation(b *testing.B) {
	for _, tc := range []struct {
		name           string
		uniqueIdentity bool
	}{
		{name: "reused_identity", uniqueIdentity: false},
		{name: "unique_identity", uniqueIdentity: true},
	} {
		pointRows, serieRows, series := benchmarkV3Rows(benchmarkV3PayloadRows, tc.uniqueIdentity)

		b.Run(tc.name+"/v3_metric_point_rows", func(b *testing.B) {
			b.ReportAllocs()
			for i := 0; i < b.N; i++ {
				ctx := &PipelineContext{}
				pb, err := benchmarkNewV3PayloadBuilder(ctx)
				if err != nil {
					b.Fatal(err)
				}
				for j := range pointRows {
					if err := pb.writeV3MetricPointRow(&pointRows[j]); err != nil {
						b.Fatal(err)
					}
				}
				if err := pb.finishPayload(); err != nil {
					b.Fatal(err)
				}
				for _, payload := range ctx.payloads {
					benchmarkV3PayloadBytes += len(payload.GetContent())
				}
			}
		})

		b.Run(tc.name+"/serie_rows", func(b *testing.B) {
			b.ReportAllocs()
			for i := 0; i < b.N; i++ {
				ctx := &PipelineContext{}
				pb, err := benchmarkNewV3PayloadBuilder(ctx)
				if err != nil {
					b.Fatal(err)
				}
				for j := range serieRows {
					if err := pb.writeSerieRow(&serieRows[j]); err != nil {
						b.Fatal(err)
					}
				}
				if err := pb.finishPayload(); err != nil {
					b.Fatal(err)
				}
				for _, payload := range ctx.payloads {
					benchmarkV3PayloadBytes += len(payload.GetContent())
				}
			}
		})

		b.Run(tc.name+"/series", func(b *testing.B) {
			b.ReportAllocs()
			for i := 0; i < b.N; i++ {
				ctx := &PipelineContext{}
				pb, err := benchmarkNewV3PayloadBuilder(ctx)
				if err != nil {
					b.Fatal(err)
				}
				for _, serie := range series {
					if err := pb.writeSerie(serie); err != nil {
						b.Fatal(err)
					}
				}
				if err := pb.finishPayload(); err != nil {
					b.Fatal(err)
				}
				for _, payload := range ctx.payloads {
					benchmarkV3PayloadBytes += len(payload.GetContent())
				}
			}
		})
	}
}
