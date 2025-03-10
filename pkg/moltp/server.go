// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package moltp is a molecule-based otlp server
package moltp

import (
	"context"
	"net"
	"runtime/trace"

	"google.golang.org/grpc"

	"github.com/DataDog/datadog-agent/pkg/metrics"
	"github.com/DataDog/datadog-agent/pkg/moltp/v1"
	"github.com/DataDog/datadog-agent/pkg/serializer"

	om "go.opentelemetry.io/proto/otlp/collector/metrics/v1"
)

type server struct {
	v1.UnsafeMetricsServiceServer
	ser serializer.MetricSerializer
}

func newServer(ser serializer.MetricSerializer) *server {
	return &server{
		ser: ser,
	}
}

// Export implements MetricsService#Export grpc method
func (s *server) Export(ctx context.Context, r *v1.ExportMetricsServiceRequest) (*om.ExportMetricsServiceResponse, error) {
	_, t := trace.NewTask(ctx, "Export")
	defer t.End()

	var err error
	metrics.Serialize(
		metrics.NewIterableSeries(func(_ *metrics.Serie) {}, 4, 128),
		metrics.NewIterableSketches(func(_ *metrics.SketchSeries) {}, 4, 128),
		func(serieSink metrics.SerieSink, sketchesSink metrics.SketchesSink) {
			cx := newCtx(serieSink, sketchesSink)
			err = exportMetricsRequest.parseBytes(cx, r.ProtoReflect().GetUnknown())
		},
		func(serieSource metrics.SerieSource) {
			err = s.ser.SendIterableSeries(serieSource)
		},
		func(sketchesSource metrics.SketchesSource) {
			err = s.ser.SendSketch(sketchesSource)
		})

	return nil, err
}

// Serve starts the server at addr and runs forever unless it encounters an error.
func Serve(addr string, ser serializer.MetricSerializer) error {
	l, err := net.Listen("tcp", addr)
	if err != nil {
		return err
	}
	s := grpc.NewServer()

	s.RegisterService(&grpc.ServiceDesc{
		// We need to use a custom protobuf definition to avoid protobuf deserialization but respond
		// to the upstream service name. Protobuf allows only one instance of the protobuf package
		// name per binary, so we can't put this in the .proto file.
		ServiceName: "opentelemetry.proto.collector.metrics.v1.MetricsService",
		HandlerType: (*v1.MetricsServiceServer)(nil),
		Methods:     v1.MetricsService_ServiceDesc.Methods,
		Streams:     []grpc.StreamDesc{},
		Metadata:    "metrics_service.proto",
	}, newServer(ser))

	return s.Serve(l)
}
