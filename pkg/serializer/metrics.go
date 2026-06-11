// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package serializer

import (
	"errors"
	"fmt"
	"math/rand/v2"
	"slices"
	"time"

	"github.com/google/uuid"

	"github.com/DataDog/datadog-agent/comp/core/config"
	forwarder "github.com/DataDog/datadog-agent/comp/forwarder/defaultforwarder/def"
	"github.com/DataDog/datadog-agent/comp/forwarder/defaultforwarder/endpoints"
	"github.com/DataDog/datadog-agent/comp/forwarder/defaultforwarder/resolver"
	"github.com/DataDog/datadog-agent/comp/forwarder/defaultforwarder/transaction"
	configutils "github.com/DataDog/datadog-agent/pkg/config/utils"
	statefulgrpc "github.com/DataDog/datadog-agent/pkg/serializer/grpc"
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

func metricsUseV3(resolver resolver.DomainResolver, config config.Component, kind metricsKind) bool {
	return slices.Contains(
		config.GetStringSlice(fmt.Sprintf("serializer_experimental_use_v3_api.%s.endpoints", kind)),
		resolver.GetConfigName())
}

func metricsValidateV3(config config.Component, kind metricsKind) bool {
	return config.GetBool(fmt.Sprintf("serializer_experimental_use_v3_api.%s.validate", kind))
}

// metricsUseStateful reports whether the stateful gRPC path is enabled for this
// kind. Series only; when enabled (and a StatefulOutput is wired) it replaces
// the HTTP path for the default (non-MRF, non-local) resolver.
func metricsUseStateful(config config.Component, kind metricsKind) bool {
	if kind != metricsKindSeries {
		return false
	}
	return config.GetBool("serializer_experimental_use_v3_stateful_api.series.enabled")
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
	useStateful := metricsUseStateful(s.config, kind) && s.statefulOutput != nil

	for _, resolver := range s.Forwarder.GetDomainResolvers() {
		useV3 := metricsUseV3(resolver, s.config, kind)

		dest := metrics.PipelineDestination{
			Resolver: resolver,
			Endpoint: metricsEndpointFor(kind, useV3, s.config),
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
			// On the default resolver, the stateful gRPC path replaces the HTTP
			// pipeline. The per-flush writer borrows the serializer's
			// StatefulOutput for interning + Submit.
			if useStateful {
				conf := metrics.PipelineConfig{
					Filter:   metrics.AllowAllFilter{},
					V3:       true,
					Stateful: true,
				}
				pipelines.Add(conf, dest)
				pipelines[conf].StatefulOutput = s.statefulOutput
				continue
			}

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

// StatefulSeriesOutput is the public alias for metrics.StatefulOutput, exposed
// so the demultiplexer can construct it and pass it to the serializer without
// importing the internal/metrics package directly.
type StatefulSeriesOutput = metrics.StatefulOutput

// NewStatefulSeriesOutput constructs (but does not start) a StatefulOutput for
// the series stateful path; the caller owns its lifecycle (Start/Stop). The
// destination and API key come from the forwarder's default DomainResolver and
// compression reuses the serializer's compressor; only transport tuning comes
// from the grpc.* keys. Returns an error if there is no usable default resolver
// or the gRPC client cannot be constructed.
func NewStatefulSeriesOutput(cfg config.Component, fwd forwarder.Forwarder, compressor compression.Compressor) (*StatefulSeriesOutput, error) {
	res := defaultDomainResolver(fwd)
	if res == nil {
		return nil, errors.New("no default (non-MRF, non-local) domain resolver available for stateful series")
	}
	sender, err := newStatefulSender(cfg, res, compressor)
	if err != nil {
		return nil, err
	}
	return metrics.NewStatefulOutput([]*statefulgrpc.Sender{sender}), nil
}

// defaultDomainResolver returns the resolver that buildPipelines routes the
// stateful series path to: the first non-local, non-MRF resolver. Returns nil
// if none exists.
func defaultDomainResolver(fwd forwarder.Forwarder) resolver.DomainResolver {
	for _, r := range fwd.GetDomainResolvers() {
		if !r.IsLocal() && !r.IsMRF() {
			return r
		}
	}
	return nil
}

// newStatefulSender builds the gRPC sender. The destination (the resolver's
// base domain) and the API key (read per-RPC, rotation-aware) come from the
// resolver; compression reuses the serializer's compressor. The Sender parses
// the base domain into a dial target itself. Only the transport-tuning knobs
// come from the grpc.* config keys.
func newStatefulSender(cfg config.Component, res resolver.DomainResolver, compressor compression.Compressor) (*statefulgrpc.Sender, error) {
	return statefulgrpc.NewSender(statefulgrpc.Config{
		BaseURL: res.GetBaseDomain(),
		APIKey: func() string {
			keys := res.GetAPIKeys()
			if len(keys) == 0 {
				return ""
			}
			return keys[0]
		},
		Compression:       compressor,
		StreamLifetime:    time.Duration(cfg.GetInt("serializer_experimental_use_v3_stateful_api.series.grpc.stream_lifetime")) * time.Second,
		ConnectionTimeout: time.Duration(cfg.GetInt("serializer_experimental_use_v3_stateful_api.series.grpc.connection_timeout")) * time.Second,
		DrainTimeout:      time.Duration(cfg.GetInt("serializer_experimental_use_v3_stateful_api.series.grpc.drain_timeout")) * time.Second,
		MaxInflight:       cfg.GetInt("serializer_experimental_use_v3_stateful_api.series.grpc.max_inflight_payloads"),
		// Backoff: reuse forwarder defaults.
		BackoffFactor:    cfg.GetFloat64("forwarder_backoff_factor"),
		BackoffBase:      time.Duration(cfg.GetInt("forwarder_backoff_base")) * time.Second,
		BackoffMax:       time.Duration(cfg.GetInt("forwarder_backoff_max")) * time.Second,
		RecoveryInterval: cfg.GetInt("forwarder_recovery_interval"),
	})
}
