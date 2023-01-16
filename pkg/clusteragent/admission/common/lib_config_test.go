// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver
// +build kubeapiserver

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

		/*
			TracingServiceMapping          []TracingServiceMapEntry
			TracingAgentTimeout            *int
			TracingHeaderTags              []TracingHeaderTagEntry
			TracingPartialFlushMinSpans    *int
			TracingDebug                   *bool
			TracingLogLevel                *string
			TracingMethods                 []string
			TracingPropagationStyleInject  []string
			TracingPropagationStyleExtract []string
		*/
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

				/*
					TracingServiceMapping:          tt.fields.TracingServiceMapping,
					TracingAgentTimeout:            tt.fields.TracingAgentTimeout,
					TracingHeaderTags:              tt.fields.TracingHeaderTags,
					TracingPartialFlushMinSpans:    tt.fields.TracingPartialFlushMinSpans,
					TracingDebug:                   tt.fields.TracingDebug,
					TracingLogLevel:                tt.fields.TracingLogLevel,
					TracingMethods:                 tt.fields.TracingMethods,
					TracingPropagationStyleInject:  tt.fields.TracingPropagationStyleInject,
					TracingPropagationStyleExtract: tt.fields.TracingPropagationStyleExtract,
				*/
			}
			require.EqualValues(t, tt.want, lc.ToEnvs())
		})
	}
}
