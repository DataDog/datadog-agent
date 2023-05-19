// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package common

import (
	"testing"

	"github.com/DataDog/datadog-agent/pkg/util/pointer"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
)

func TestLibConfig_ToEnvs(t *testing.T) {
	type fields struct {
		ServiceName         *string
		Env                 *string
		Tracing             *bool
		LogInjection        *bool
		HealthMetrics       *bool
		RuntimeMetrics      *bool
		TracingSamplingRate *float64
		TracingRateLimit    *int
		TracingTags         []string

		TracingServiceMapping          []TracingServiceMapEntry
		TracingAgentTimeout            *int
		TracingHeaderTags              []TracingHeaderTagEntry
		TracingPartialFlushMinSpans    *int
		TracingDebug                   *bool
		TracingLogLevel                *string
		TracingMethods                 []string
		TracingPropagationStyleInject  []string
		TracingPropagationStyleExtract []string
	}
	tests := []struct {
		name   string
		fields fields
		want   []corev1.EnvVar
	}{
		{
			name: "all",
			fields: fields{
				ServiceName:         pointer.Ptr("svc"),
				Env:                 pointer.Ptr("dev"),
				Tracing:             pointer.Ptr(true),
				LogInjection:        pointer.Ptr(true),
				HealthMetrics:       pointer.Ptr(true),
				RuntimeMetrics:      pointer.Ptr(true),
				TracingSamplingRate: pointer.Ptr(0.5),
				TracingRateLimit:    pointer.Ptr(50),
				TracingTags:         []string{"k1:v1", "k2:v2"},
				TracingServiceMapping: []TracingServiceMapEntry{{
					FromKey: "svc1",
					ToName:  "svc2",
				}, {
					FromKey: "svc3",
					ToName:  "svc4",
				},
				},
				TracingAgentTimeout: pointer.Ptr(2),
				TracingHeaderTags: []TracingHeaderTagEntry{
					{
						Header:  "X-Test-Header",
						TagName: "x.test.header",
					},
					{
						Header:  "X-Test-Other-Header",
						TagName: "x.test.other.header",
					},
				},
				TracingPartialFlushMinSpans:    pointer.Ptr(100),
				TracingDebug:                   pointer.Ptr(true),
				TracingLogLevel:                pointer.Ptr("DEBUG"),
				TracingMethods:                 []string{"modA.method", "modB.method"},
				TracingPropagationStyleInject:  []string{"Datadog", "B3", "W3C"},
				TracingPropagationStyleExtract: []string{"W3C", "B3", "Datadog"},
			},
			want: []corev1.EnvVar{
				{
					Name:  "DD_SERVICE",
					Value: "svc",
				},
				{
					Name:  "DD_ENV",
					Value: "dev",
				},
				{
					Name:  "DD_TRACE_ENABLED",
					Value: "true",
				},
				{
					Name:  "DD_LOGS_INJECTION",
					Value: "true",
				},
				{
					Name:  "DD_TRACE_HEALTH_METRICS_ENABLED",
					Value: "true",
				},
				{
					Name:  "DD_RUNTIME_METRICS_ENABLED",
					Value: "true",
				},
				{
					Name:  "DD_TRACE_SAMPLE_RATE",
					Value: "0.50",
				},
				{
					Name:  "DD_TRACE_RATE_LIMIT",
					Value: "50",
				},
				{
					Name:  "DD_TAGS",
					Value: "k1:v1,k2:v2",
				},
				{
					Name:  "DD_TRACE_SERVICE_MAPPING",
					Value: "svc1:svc2, svc3:svc4",
				},
				{
					Name:  "DD_TRACE_AGENT_TIMEOUT",
					Value: "2",
				},
				{
					Name:  "DD_TRACE_HEADER_TAGS",
					Value: "X-Test-Header:x.test.header, X-Test-Other-Header:x.test.other.header",
				},
				{
					Name:  "DD_TRACE_PARTIAL_FLUSH_MIN_SPANS",
					Value: "100",
				},
				{
					Name:  "DD_TRACE_DEBUG",
					Value: "true",
				},
				{
					Name:  "DD_TRACE_LOG_LEVEL",
					Value: "DEBUG",
				},
				{
					Name:  "DD_TRACE_METHODS",
					Value: "modA.method;modB.method",
				},
				{
					Name:  "DD_PROPAGATION_STYLE_INJECT",
					Value: "Datadog,B3,W3C",
				},
				{
					Name:  "DD_PROPAGATION_STYLE_EXTRACT",
					Value: "W3C,B3,Datadog",
				},
			},
		},
		{
			name: "only service and env",
			fields: fields{
				ServiceName: pointer.Ptr("svc"),
				Env:         pointer.Ptr("dev"),
			},
			want: []corev1.EnvVar{
				{
					Name:  "DD_SERVICE",
					Value: "svc",
				},
				{
					Name:  "DD_ENV",
					Value: "dev",
				},
			},
		},
		{
			name:   "empty",
			fields: fields{},
			want:   nil,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			lc := LibConfig{
				ServiceName:         tt.fields.ServiceName,
				Env:                 tt.fields.Env,
				Tracing:             tt.fields.Tracing,
				LogInjection:        tt.fields.LogInjection,
				HealthMetrics:       tt.fields.HealthMetrics,
				RuntimeMetrics:      tt.fields.RuntimeMetrics,
				TracingSamplingRate: tt.fields.TracingSamplingRate,
				TracingRateLimit:    tt.fields.TracingRateLimit,
				TracingTags:         tt.fields.TracingTags,

				TracingServiceMapping:          tt.fields.TracingServiceMapping,
				TracingAgentTimeout:            tt.fields.TracingAgentTimeout,
				TracingHeaderTags:              tt.fields.TracingHeaderTags,
				TracingPartialFlushMinSpans:    tt.fields.TracingPartialFlushMinSpans,
				TracingDebug:                   tt.fields.TracingDebug,
				TracingLogLevel:                tt.fields.TracingLogLevel,
				TracingMethods:                 tt.fields.TracingMethods,
				TracingPropagationStyleInject:  tt.fields.TracingPropagationStyleInject,
				TracingPropagationStyleExtract: tt.fields.TracingPropagationStyleExtract,
			}
			require.EqualValues(t, tt.want, lc.ToEnvs())
		})
	}
}
