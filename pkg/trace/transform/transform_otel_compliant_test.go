// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package transform

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"go.opentelemetry.io/collector/component/componenttest"
	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/ptrace"
	semconv127 "go.opentelemetry.io/otel/semconv/v1.27.0"

	"github.com/DataDog/datadog-agent/pkg/opentelemetry-mapping-go/otlp/attributes"

	"github.com/DataDog/datadog-agent/pkg/trace/config"
)

func TestOtelSpanToDDSpanDBNameMapping_OTelCompliantTranslation(t *testing.T) {
	tests := []struct {
		name                string
		sattrs              map[string]string
		rattrs              map[string]string
		expectedName        string
		shouldMap           bool
		wrongPlaceKeysCount int
	}{
		{
			name:         "datadog.* namespace takes precedence",
			sattrs:       map[string]string{attributes.DDNamespaceKeys.DBName(): "dd-db", string(semconv127.DBNamespaceKey): "testdb"},
			expectedName: "dd-db",
		},
		{
			name:         "db.namespace in span attributes, no db.name",
			sattrs:       map[string]string{string(semconv127.DBNamespaceKey): "testdb"},
			expectedName: "testdb",
		},
		{
			name:                "db.namespace ignored in resource attributes",
			rattrs:              map[string]string{string(semconv127.DBNamespaceKey): "testdb"},
			expectedName:        "",
			wrongPlaceKeysCount: 1,
		},
		{
			name:         "db.name already exists, should not map",
			sattrs:       map[string]string{"db.name": "existing-db", string(semconv127.DBNamespaceKey): "testdb"},
			expectedName: "existing-db",
		},
		{
			name:   "no db.namespace, should not map",
			sattrs: map[string]string{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			span := ptrace.NewSpan()
			span.SetName("test-span")
			for k, v := range tt.sattrs {
				span.Attributes().PutStr(k, v)
			}

			res := pcommon.NewResource()
			for k, v := range tt.rattrs {
				res.Attributes().PutStr(k, v)
			}

			lib := pcommon.NewInstrumentationScope()
			lib.SetName("test-lib")

			cfg := &config.AgentConfig{}
			cfg.Features = map[string]struct{}{"enable_otel_compliant_translation": {}}
			cfg.OTLPReceiver = &config.OTLP{}
			cfg.OTLPReceiver.AttributesTranslator, _ = attributes.NewTranslator(componenttest.NewNopTelemetrySettings())

			ddspan, wrongPlaceKeysCount := OtelSpanToDDSpan(span, res, lib, cfg)
			assert.Equal(t, tt.wrongPlaceKeysCount, wrongPlaceKeysCount)

			assert.Equal(t, tt.expectedName, ddspan.Meta["db.name"])
		})
	}
}
