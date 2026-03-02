// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package rawotlpexporter

import (
	"context"
	"time"

	pb "github.com/DataDog/datadog-agent/pkg/proto/pbgo/trace"
	"go.opentelemetry.io/collector/component"
	"go.opentelemetry.io/collector/config/configretry"
	exp "go.opentelemetry.io/collector/exporter"
	"go.opentelemetry.io/collector/exporter/exporterhelper"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

const (
	// TypeStr is the type of the raw OTLP trace exporter.
	TypeStr   = "otlpraw"
	stability = component.StabilityLevelDevelopment
)

// NewFactory creates a factory for the raw OTLP trace exporter.
func NewFactory() exp.Factory {
	return exp.NewFactory(
		component.MustNewType(TypeStr),
		func() component.Config {
			return &Config{
				TLSSetting: struct {
					Insecure bool `mapstructure:"insecure"`
				}{Insecure: true},
			}
		},
		exp.WithTraces(createTracesExporter, stability),
	)
}

func createTracesExporter(
	ctx context.Context,
	set exp.Settings,
	cfg component.Config,
) (exp.Traces, error) {
	config := cfg.(*Config)
	if err := config.Validate(); err != nil {
		return nil, err
	}

	opts := []grpc.DialOption{}
	if config.TLSSetting.Insecure {
		opts = append(opts, grpc.WithTransportCredentials(insecure.NewCredentials()))
	}

	conn, err := grpc.DialContext(ctx, config.Endpoint, opts...)
	if err != nil {
		return nil, err
	}

	client := pb.NewRawTraceServiceClient(conn)
	exp := &tracesExporter{client: client, logger: set.Logger}

	return exporterhelper.NewTraces(
		ctx,
		set,
		cfg,
		exp.consumeTraces,
		exporterhelper.WithTimeout(exporterhelper.TimeoutConfig{Timeout: 10 * time.Second}),
		exporterhelper.WithRetry(configretry.NewDefaultBackOffConfig()),
		exporterhelper.WithQueue(config.QueueSettings),
		exporterhelper.WithShutdown(func(context.Context) error {
			return conn.Close()
		}),
	)
}
