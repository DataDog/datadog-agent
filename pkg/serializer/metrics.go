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
		return "distributions"
	default:
		panic("invalid metricsKind value")
	}
}

// metricsUseV3 decides whether a given resolver should ship the given metrics kind via the v3 intake.
func metricsUseV3(r resolver.DomainResolver, config config.Component, logger log.Component, kind metricsKind) bool {
	if r.IsMetricToVector() {
		if kind == metricsKindSeries {
			return vectorSeriesUseV3(config)
		}
		return false
	}

	if kind == metricsKindSketches && slices.Contains(
		config.GetStringSlice("serializer_experimental_use_v3_api.sketches.endpoints"),
		r.GetConfigName()) {
		return true
	}

	endpointsKey := fmt.Sprintf("use_v3_api.%s.endpoints", kind)
	endpoints := config.GetStringMapString(endpointsKey)
	if value, ok := endpoints[r.GetConfigName()]; ok {
		key := fmt.Sprintf("%s.%q", endpointsKey, r.GetConfigName())
		return evalSeriesV3(key, value, r, logger)
	}

	enabledKey := fmt.Sprintf("use_v3_api.%s.enabled", kind)
	return evalSeriesV3(enabledKey, config.GetString(enabledKey), r, logger)
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

// metricsShadowSampleRate returns the per-flush probability of also sending
// this kind of metrics to v3beta as a validation shadow when v3 is not the
// authoritative endpoint. Series shadowing is retired: the series rollout is
// driven by use_v3_api.series, so it never samples.
func metricsShadowSampleRate(config config.Component, kind metricsKind) float64 {
	if kind == metricsKindSeries {
		return 0
	}
	return config.GetFloat64("serializer_experimental_use_v3_api.sketches.shadow_sample_rate")
}

// v3BetaShadowEndpoint returns the v3beta shadow endpoint for a metrics kind,
// with the sketches route overridable via beta_route.
func v3BetaShadowEndpoint(config config.Component, kind metricsKind) transaction.Endpoint {
	switch kind {
	case metricsKindSeries:
		return endpoints.V3BetaSeriesEndpoint
	case metricsKindSketches:
		route := config.GetString("serializer_experimental_use_v3_api.sketches.beta_route")
		return transaction.Endpoint{Route: route, Name: endpoints.V3BetaSketchSeriesEndpoint.Name}
	default:
		panic("invalid metricsKind value")
	}
}

// metricsShadowSites returns the list of Datadog sites for which v3beta shadow
// sampling is enabled for this kind. Sites are matched against the resolved v2
// destination via configutils.ExtractSiteFromURL. Defaults to US1 only.
func metricsShadowSites(config config.Component, kind metricsKind) []string {
	if kind == metricsKindSeries {
		return nil
	}
	return config.GetStringSlice("serializer_experimental_use_v3_api.sketches.shadow_sites")
}

// metricsShadowAllowed reports whether the resolver targets a site that opts
// into v3beta shadowing. It resolves the kind's v2 endpoint so that when v2
// metrics are diverted to a non-Datadog destination (e.g. vector/OPW), the
// resolved domain falls outside the allow list and shadowing is skipped.
func metricsShadowAllowed(r resolver.DomainResolver, kind metricsKind, sites []string) bool {
	site := configutils.ExtractSiteFromURL(r.Resolve(metricsEndpointFor(kind, false)))
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
	return s.buildPipelinesRng(kind, stdRand{})
}

func (s *Serializer) buildPipelinesRng(kind metricsKind, rng prng) metrics.PipelineSet {
	return s.buildPipelinesWithMRF(kind, rng, s.getFailoverAllowlist())
}

// buildPipelinesWithMRF sends to the MRF domain through mrfFilter, or leaves the MRF
// domain out when mrfFilter is nil.
func (s *Serializer) buildPipelinesWithMRF(kind metricsKind, rng prng, mrfFilter metrics.Filter) metrics.PipelineSet {
	pipelines := metrics.PipelineSet{}

	autoscalingFilter := s.getAutoscalingFailoverMetrics()
	shadowRate := metricsShadowSampleRate(s.config, kind)
	shadowSites := metricsShadowSites(s.config, kind)

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
			Endpoint: metricsEndpointFor(kind, useV3),
		}

		switch {
		case resolver.IsLocal():
			// Cluster agent only speaks v2
			dest.Endpoint = metricsEndpointFor(kind, false)

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
			shadowV3 := !useV3 && metricsShadowAllowed(resolver, kind, shadowSites) && shadowRate > 0 && rng.Float64() < shadowRate
			if shadowV3 {
				dest.ValidationBatchID = s.genUUID()
			}

			conf := metrics.PipelineConfig{
				Filter: metrics.AllowAllFilter{},
				V3:     useV3,
			}
			pipelines.Add(conf, dest)

			// On a regular v2 route, send a sampled shadow copy to v3beta for
			// intake-side validation. The same batchID correlates v2 and v3beta.
			if shadowV3 {
				sconf := metrics.PipelineConfig{
					Filter: metrics.AllowAllFilter{},
					V3:     true,
				}
				sdest := metrics.PipelineDestination{
					Resolver:          resolver,
					Endpoint:          v3BetaShadowEndpoint(s.config, kind),
					ValidationBatchID: dest.ValidationBatchID,
				}
				pipelines.Add(sconf, sdest)
			}
		}
	}

	return pipelines
}

// SketchesV2Domains returns the configured domains that sketches are sent to over the v2
// intake, which cannot carry OpenTelemetry histograms. It uses the same routing as SendSketch,
// but counts the MRF domain even while failover is off, since failover can be turned on later.
// The names are the ones matched against use_v3_api.distributions.endpoints.
func (s *Serializer) SketchesV2Domains() []string {
	var domains []string
	for conf, ctx := range s.buildPipelinesWithMRF(metricsKindSketches, stdRand{}, metrics.AllowAllFilter{}) {
		if conf.V3 {
			continue
		}
		for _, dest := range ctx.Destinations {
			domains = append(domains, dest.Resolver.GetConfigName())
		}
	}
	slices.Sort(domains)
	return slices.Compact(domains)
}

func (s *Serializer) genUUID() string {
	uuid, err := uuid.NewV7()
	if err != nil {
		s.logger.Warnf("failed to generate payload batch id: %v", err)
		return ""
	}
	return uuid.String()
}
