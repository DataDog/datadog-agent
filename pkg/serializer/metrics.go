// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package serializer

import (
	"fmt"
	"slices"

	"github.com/DataDog/datadog-agent/comp/core/config"
	"github.com/DataDog/datadog-agent/comp/forwarder/defaultforwarder/endpoints"
	"github.com/DataDog/datadog-agent/comp/forwarder/defaultforwarder/resolver"
	"github.com/DataDog/datadog-agent/comp/forwarder/defaultforwarder/transaction"
	"github.com/DataDog/datadog-agent/pkg/serializer/internal/metrics"
)

type metricsKind int

const (
	metricsKindSeries metricsKind = iota
	metricsKindSketches
)

func (k metricsKind) String() string {
	switch k {
	case metricsKindSeries:
		return "series"
	case metricsKindSketches:
		return "sketches"
	default:
		panic("invalid metricsKind value")
	}
}

func metricsUseV3(resolver resolver.DomainResolver, config config.Component, kind metricsKind) bool {
	return slices.Contains(
		config.GetStringSlice(fmt.Sprintf("serializer_experimental_use_v3_api.%s.endpoints", kind)),
		resolver.GetConfigName())
}

func metricsValidateV3(config config.Component, kind metricsKind) bool {
	return config.GetBool(fmt.Sprintf("serializer_experimental_use_v3_api.%s.validate", kind))
}

func metricsEndpointFor(kind metricsKind, useV3 bool) transaction.Endpoint {
	switch kind {
	case metricsKindSeries:
		if useV3 {
			return endpoints.V3SeriesEndpoint
		}
		return endpoints.SeriesEndpoint
	case metricsKindSketches:
		if useV3 {
			return endpoints.V3SketchSeriesEndpoint
		}
		return endpoints.SketchSeriesEndpoint
	default:
		panic("invalid metricsKind value")
	}
}

func (s *Serializer) buildPipelines(kind metricsKind) metrics.PipelineSet {
	pipelines := metrics.PipelineSet{}

	mrfFilter := s.getFailoverAllowlist()
	autoscalingFilter := s.getAutoscalingFailoverMetrics()
	validateV3 := metricsValidateV3(s.config, kind)

	for _, resolver := range s.Forwarder.GetDomainResolvers() {
		useV3 := metricsUseV3(resolver, s.config, kind)
		validateV3 := useV3 && validateV3

		dest := metrics.PipelineDestination{
			Resolver:             resolver,
			Endpoint:             metricsEndpointFor(kind, useV3),
			AddValidationHeaders: validateV3,
		}

		switch {
		case resolver.IsLocal():
			if autoscalingFilter != nil && kind == metricsKindSeries {
				conf := metrics.PipelineConfig{
					Filter: autoscalingFilter,
				}
				pipelines.Add(conf, dest)
			}

		case resolver.IsMRF():
			if mrfFilter != nil {
				conf := metrics.PipelineConfig{
					Filter: mrfFilter,
					V3:     useV3,
				}
				pipelines.Add(conf, dest)
			}

		default:
			conf := metrics.PipelineConfig{
				Filter: metrics.AllowAllFilter{},
				V3:     useV3,
			}
			pipelines.Add(conf, dest)

			// On a regular route if using v3 and validation is enabled, send a v2 payload too.
			if validateV3 {
				vconf := metrics.PipelineConfig{
					Filter: metrics.AllowAllFilter{},
					V3:     false,
				}
				vdest := metrics.PipelineDestination{
					Resolver:             resolver,
					Endpoint:             metricsEndpointFor(kind, false),
					AddValidationHeaders: true,
				}
				pipelines.Add(vconf, vdest)
			}
		}
	}

	return pipelines
}
