// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build otlp
// +build otlp

package config

import "github.com/DataDog/opentelemetry-mapping-go/pkg/otlp/attributes"

// OTLP holds the configuration for the OpenTelemetry receiver.
type OTLP struct {
	// BindHost specifies the host to bind the receiver to.
	BindHost string `mapstructure:"-"`

	// GRPCPort specifies the port to use for the plain HTTP receiver.
	// If unset (or 0), the receiver will be off.
	GRPCPort int `mapstructure:"grpc_port"`

	// SpanNameRemappings is the map of datadog span names and preferred name to map to. This can be used to
	// automatically map Datadog Span Operation Names to an updated value. All entries should be key/value pairs.
	SpanNameRemappings map[string]string `mapstructure:"span_name_remappings"`

	// SpanNameAsResourceName specifies whether the OpenTelemetry span's name should be
	// used as the Datadog span's operation name. By default (when this is false), the
	// operation name is deduced from a combination between the instrumentation scope
	// name and the span kind.
	//
	// For context, the OpenTelemetry 'Span Name' is equivalent to the Datadog 'resource name'.
	// The Datadog Span's Operation Name equivalent in OpenTelemetry does not exist, but the span's
	// kind comes close.
	SpanNameAsResourceName bool `mapstructure:"span_name_as_resource_name"`

	// MaxRequestBytes specifies the maximum number of bytes that will be read
	// from an incoming HTTP request.
	MaxRequestBytes int64 `mapstructure:"-"`

	// ProbabilisticSampling specifies the percentage of traces to ingest. Exceptions are made for errors
	// and rare traces (outliers) if "RareSamplerEnabled" is true. Invalid values are equivalent to 100.
	// If spans have the "sampling.priority" attribute set, probabilistic sampling is skipped and the user's
	// decision is followed.
	ProbabilisticSampling float64

	// AttributesTranslator specifies an OTLP to Datadog attributes translator.
	AttributesTranslator *attributes.Translator `mapstructure:"-"`
}

func NewOTLP(host string, port int, spanNameRemappings map[string]string, spanNameAsResourceName bool,
	maxReqBytes int64, sample float64, attributesTranslator *attributes.Translator) *OTLP {
	return &OTLP{
		BindHost:               host,
		GRPCPort:               port,
		MaxRequestBytes:        maxReqBytes,
		SpanNameRemappings:     spanNameRemappings,
		SpanNameAsResourceName: spanNameAsResourceName,
		ProbabilisticSampling:  sample,
		AttributesTranslator:   attributesTranslator,
	}
}
