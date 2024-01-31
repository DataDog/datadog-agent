// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !otlp
// +build !otlp

package config

import "github.com/DataDog/opentelemetry-mapping-go/pkg/otlp/attributes"

// OTLP holds the configuration for the OpenTelemetry receiver.
type OTLP struct{}

func NewOTLP(host string, port int, spanNameRemappings map[string]string, spanNameAsResourceName bool,
	maxReqBytes int64, sample float64, attributesTranslator *attributes.Translator) *OTLP {
	return nil
}
