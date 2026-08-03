// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package serializer

import (
	"fmt"
	"math/rand/v2"
	"slices"
	"strings"

	"github.com/google/uuid"

	"github.com/DataDog/datadog-agent/comp/core/config"
	"github.com/DataDog/datadog-agent/comp/core/log/def"
	"github.com/DataDog/datadog-agent/comp/forwarder/defaultforwarder/endpoints"
	"github.com/DataDog/datadog-agent/comp/forwarder/defaultforwarder/resolver"
	"github.com/DataDog/datadog-agent/comp/forwarder/defaultforwarder/transaction"
	configutils "github.com/DataDog/datadog-agent/pkg/config/utils"
	"github.com/DataDog/datadog-agent/pkg/serializer/internal/metrics"
	"github.com/DataDog/datadog-agent/pkg/util/compression"
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

// metricsUseV3 decides whether a given resolver should ship the given metrics kind via the v3 intake.
func metricsUseV3(r resolver.DomainResolver, config config.Component, logger log.Component, kind metricsKind) bool {
	exp := slices.Contains(
		config.GetStringSlice(fmt.Sprintf("serializer_experimental_use_v3_api.%s.endpoints", kind)),
		r.GetConfigName())
	if exp {
		return true
	}

	if kind == metricsKindSeries {
		return seriesUseV3(r, config, logger)
	}

	return false
}

// zlibForcesV2 reports whether the active metrics compressor is zlib, which is incompatible
// with the v3 metrics intake.
func (s *Serializer) zlibForcesV2() bool {
	return s.Strategy.ContentEncoding() == compression.ZlibEncoding
}

// warnZlibDisablesV3 logs, once per serializer, that zlib compression is forcing
// requested v3 pipelines back to v2.
func (s *Serializer) warnZlibDisablesV3() {
	s.zlibV3WarnOnce.Do(func() {
		s.logger.Info(
			"the active metrics compressor is zlib (deflate); disabling v3 metrics intake " +
				"(use_v3_api.* and serializer_experimental_use_v3_api.*.endpoints are ignored). " +
				"Switch to zstd to use the v3 endpoint.")
	})
}

// evalSeriesV3 maps a single use_v3_api.series.enabled / endpoints[url] value onto a
// v3-vs-v2 decision for r.
func evalSeriesV3(key, value string, r resolver.DomainResolver, logger log.Component) bool {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "true", "1", "t", "yes", "on": // yaml truth values
		return true
	case "false", "0", "f", "no", "off": // yaml false values
		return false
	case "datadog_only":
		return configutils.IsDatadogURL(r.GetConfigName())
	}
	logger.Warnf("%s value %q is invalid, using false", key, value)
	return false
}

func vectorSeriesUseV3(config config.Component) bool {
	if config.GetBool("observability_pipelines_worker.metrics.enabled") {
		return config.GetBool("observability_pipelines_worker.metrics.use_v3_api.series")
	}
	return config.GetBool("vector.metrics.use_v3_api.series")
}

func seriesUseV3(r resolver.DomainResolver, config config.Component, logger log.Component) bool {
	if r.IsMetricToVector() {
		return vectorSeriesUseV3(config)
	}

	endpointsKey := "use_v3_api.series.endpoints"
	endpoints := config.GetStringMapString(endpointsKey)
	if value, ok := endpoints[r.GetConfigName()]; ok {
		key := fmt.Sprintf("%s.%q", endpointsKey, r.GetConfigName())
		return evalSeriesV3(key, value, r, logger)
	}

	return evalSeriesV3("use_v3_api.series.enabled", config.GetString("use_v3_api.series.enabled"), r, logger)
}

func metricsValidateV3(config config.Component, kind metricsKind) bool {
	return config.GetBool(fmt.Sprintf("serializer_experimental_use_v3_api.%s.validate", kind))
}

// metricsShadowSampleRate returns the per-flush probability of also sending
// this kind of metrics to v3beta as a validation shadow when v3 is not the
// authoritative endpoint. Only series are supported today.
func metricsShadowSampleRate(config config.Component, kind metricsKind) float64 {
	if kind != metricsKindSeries {
		return 0
	}
	return config.GetFloat64("serializer_experimental_use_v3_api.series.shadow_sample_rate")
}

func v3BetaShadowEndpoint(config config.Component) transaction.Endpoint {
	route := config.GetString("serializer_experimental_use_v3_api.series.beta_route")
	return transaction.Endpoint{Route: route, Name: endpoints.V3BetaSeriesEndpoint.Name}
}

// metricsShadowSites returns the list of Datadog sites for which v3beta shadow
// sampling is enabled. Sites are matched against the resolved v2 series
// destination via configutils.ExtractSiteFromURL. Defaults to US1 only.
func metricsShadowSites(config config.Component) []string {
	return config.GetStringSlice("serializer_experimental_use_v3_api.series.shadow_sites")
}

// metricsShadowAllowed reports whether the resolver targets a site that opts
// into v3beta shadowing. It resolves the v2 series endpoint so that when v2
// metrics are diverted to a non-Datadog destination (e.g. vector/OPW), the
// resolved domain falls outside the allow list and shadowing is skipped.
func metricsShadowAllowed(r resolver.DomainResolver, sites []string) bool {
	site := configutils.ExtractSiteFromURL(r.Resolve(endpoints.SeriesEndpoint))
	if site == "" {
		return false
	}
	return slices.Contains(sites, site)
}

// prng is the minimal pseudo-random source used by buildPipelines to make
// per-flush sampling decisions. Tests inject a deterministic implementation.
type prng interface {
	Float64() float64
}

// stdRand delegates to math/rand/v2's package-level generator.
type stdRand struct{}

func (stdRand) Float64() float64 { return rand.Float64() }

func metricsEndpointFor(kind metricsKind, useV3 bool, config config.Component) transaction.Endpoint {
	switch kind {
	case metricsKindSeries:
		if useV3 {
			if config.GetBool("serializer_experimental_use_v3_api.series.use_beta") {
				route := config.GetString("serializer_experimental_use_v3_api.series.beta_route")
				return transaction.Endpoint{Route: route, Name: endpoints.V3BetaSeriesEndpoint.Name}
			}
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
	return s.buildPipelinesRng(kind, stdRand{})
}

func (s *Serializer) buildPipelinesRng(kind metricsKind, rng prng) metrics.PipelineSet {
	pipelines := metrics.PipelineSet{}

	mrfFilter := s.getFailoverAllowlist()
	autoscalingFilter := s.getAutoscalingFailoverMetrics()
	validateV3 := metricsValidateV3(s.config, kind)
	shadowRate := metricsShadowSampleRate(s.config, kind)
	shadowSites := metricsShadowSites(s.config)

	zlib := s.zlibForcesV2()
	if zlib {
		shadowRate = 0
	}

	for _, resolver := range s.Forwarder.GetDomainResolvers() {
		useV3 := metricsUseV3(resolver, s.config, s.logger, kind)
		if zlib && useV3 {
			s.warnZlibDisablesV3()
			useV3 = false
		}

		dest := metrics.PipelineDestination{
			Resolver: resolver,
			Endpoint: metricsEndpointFor(kind, useV3, s.config),
		}

		switch {
		case resolver.IsLocal():
			// Cluster agent only speaks v2
			dest.Endpoint = metricsEndpointFor(kind, false, s.config)

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
			validateV3 := useV3 && validateV3
			shadowV3 := !useV3 && metricsShadowAllowed(resolver, shadowSites) && shadowRate > 0 && rng.Float64() < shadowRate
			if validateV3 || shadowV3 {
				dest.ValidationBatchID = s.genUUID()
			}

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
					Resolver:          resolver,
					Endpoint:          metricsEndpointFor(kind, false, s.config),
					ValidationBatchID: dest.ValidationBatchID,
				}
				pipelines.Add(vconf, vdest)
			}

			// On a regular v2 route, send a sampled shadow copy to v3beta for
			// intake-side validation. The same batchID correlates v2 and v3beta.
			if shadowV3 {
				sconf := metrics.PipelineConfig{
					Filter: metrics.AllowAllFilter{},
					V3:     true,
				}
				sdest := metrics.PipelineDestination{
					Resolver:          resolver,
					Endpoint:          v3BetaShadowEndpoint(s.config),
					ValidationBatchID: dest.ValidationBatchID,
				}
				pipelines.Add(sconf, sdest)
			}
		}
	}

	return pipelines
}

func (s *Serializer) genUUID() string {
	uuid, err := uuid.NewV7()
	if err != nil {
		s.logger.Warnf("failed to generate payload batch id: %v", err)
		return ""
	}
	return uuid.String()
}
