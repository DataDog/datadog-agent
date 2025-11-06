// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package serializer

import (
	"github.com/DataDog/datadog-agent/comp/forwarder/defaultforwarder/endpoints"
	"github.com/DataDog/datadog-agent/comp/forwarder/defaultforwarder/transaction"
	"github.com/DataDog/datadog-agent/pkg/serializer/internal/metrics"
)

type metricsKind int

const (
	metricsKindSeries metricsKind = iota
	metricsKindSketches
)

func metricsEndpointFor(kind metricsKind) transaction.Endpoint {
	switch kind {
	case metricsKindSeries:
		return endpoints.SeriesEndpoint
	case metricsKindSketches:
		return endpoints.SketchSeriesEndpoint
	default:
		panic("invalid metricsKind value")
	}
}

func (s *Serializer) buildPipelines(kind metricsKind) metrics.PipelineSet {
	pipelines := metrics.PipelineSet{}

	mrfFilter := s.getFailoverAllowlist()
	autoscalingFilter := s.getAutoscalingFailoverMetrics()

	for _, resolver := range s.Forwarder.GetDomainResolvers() {
		dest := metrics.PipelineDestination{
			Resolver: resolver,
			Endpoint: metricsEndpointFor(kind),
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
				}
				pipelines.Add(conf, dest)
			}

		default:
			conf := metrics.PipelineConfig{
				Filter: metrics.AllowAllFilter{},
			}
			pipelines.Add(conf, dest)
		}
	}

	return pipelines
}
