// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2021-present Datadog, Inc.

package main

import (
	"context"
	"log"
	"os"
	"time"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetricgrpc"
	"go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetrichttp"
	"go.opentelemetry.io/otel/metric/global"
	"go.opentelemetry.io/otel/metric/unit"
	"go.opentelemetry.io/otel/sdk/instrumentation"
	"go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"
	"go.opentelemetry.io/otel/sdk/resource"
	semconv "go.opentelemetry.io/otel/semconv/v1.4.0"
)

var (
	now = time.Now()

	res = resource.NewSchemaless(
		semconv.ServiceNameKey.String("otlpmetric-example"),
	)

	min = 0.6
	max = 77.7

	mockData = metricdata.ResourceMetrics{
		Resource: res,
		ScopeMetrics: []metricdata.ScopeMetrics{
			{
				Scope: instrumentation.Scope{Name: "example", Version: "v0.0.1"},
				Metrics: []metricdata.Metrics{
					{
						Name:        "otlp.test.request",
						Description: "Number of requests received",
						Unit:        unit.Dimensionless,
						Data: metricdata.Sum[int64]{
							IsMonotonic: true,
							Temporality: metricdata.DeltaTemporality,
							DataPoints: []metricdata.DataPoint[int64]{
								{
									Attributes: attribute.NewSet(attribute.String("server", "central")),
									StartTime:  now,
									Time:       now.Add(1 * time.Second),
									Value:      5,
								},
							},
						},
					},
					{
						Name:        "otlp.test.latency",
						Description: "Time spend processing received requests",
						Unit:        unit.Milliseconds,
						Data: metricdata.Histogram{
							Temporality: metricdata.DeltaTemporality,
							DataPoints: []metricdata.HistogramDataPoint{
								{
									Attributes:   attribute.NewSet(attribute.String("server", "central")),
									StartTime:    now,
									Time:         now.Add(1 * time.Second),
									Count:        10,
									Bounds:       []float64{1, 5, 10},
									BucketCounts: []uint64{1, 3, 6, 0},
									Sum:          57,
									Min:          &min,
									Max:          &max,
								},
							},
						},
					},
					{
						Name:        "otlp.test.temperature",
						Description: "CPU global temperature",
						Unit:        unit.Unit("cel(1 K)"),
						Data: metricdata.Gauge[float64]{
							DataPoints: []metricdata.DataPoint[float64]{
								{
									Attributes: attribute.NewSet(attribute.String("server", "central")),
									Time:       now.Add(1 * time.Second),
									Value:      32.4,
								},
							},
						},
					},
				},
			},
		},
	}
)

func deltaSelector(metric.InstrumentKind) metricdata.Temporality {
	return metricdata.DeltaTemporality
}

func main() {
	var protocol string
	if len(os.Args) > 1 {
		protocol = os.Args[1]
	}
	grpcExpOpt := []otlpmetricgrpc.Option{otlpmetricgrpc.WithInsecure()}
	httpExpOpt := []otlpmetrichttp.Option{otlpmetrichttp.WithInsecure()}
	if len(os.Args) > 2 {
		grpcExpOpt = append(grpcExpOpt, otlpmetricgrpc.WithEndpoint(os.Args[2]))
		httpExpOpt = append(httpExpOpt, otlpmetrichttp.WithEndpoint(os.Args[2]))
	}
	if len(os.Args) > 3 && os.Args[3] == "delta" {
		grpcExpOpt = append(grpcExpOpt, otlpmetricgrpc.WithTemporalitySelector(deltaSelector))
		httpExpOpt = append(httpExpOpt, otlpmetrichttp.WithTemporalitySelector(deltaSelector))
	}

	ctx := context.Background()
	var exp metric.Exporter
	var err error
	if protocol == "http" {
		exp, err = otlpmetrichttp.New(ctx, httpExpOpt...)
	} else {
		exp, err = otlpmetricgrpc.New(ctx, grpcExpOpt...)
	}
	if err != nil {
		log.Fatal(err)
	}

	sdk := metric.NewMeterProvider(metric.WithReader(metric.NewPeriodicReader(exp)))
	global.SetMeterProvider(sdk)

	// This is where the sdk would be used to create a Meter and from that
	// instruments that would make measurments of your code. To simulate that
	// behavior, call export directly with mocked data.
	err = exp.Export(ctx, mockData)
	if err != nil {
		panic(err)
	}

	// Ensure the periodic reader is cleaned up by shutting down the sdk.
	err = sdk.Shutdown(ctx)
	if err != nil {
		panic(err)
	}
}
