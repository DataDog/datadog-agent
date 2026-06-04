// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package serializer

import (
	"fmt"
	"slices"
	"sync"
	"time"

	"github.com/google/uuid"

	"github.com/DataDog/datadog-agent/comp/core/config"
	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	"github.com/DataDog/datadog-agent/comp/forwarder/defaultforwarder/endpoints"
	"github.com/DataDog/datadog-agent/comp/forwarder/defaultforwarder/resolver"
	"github.com/DataDog/datadog-agent/comp/forwarder/defaultforwarder/transaction"
	"github.com/DataDog/datadog-agent/pkg/serializer/internal/metrics"
	statefulgrpc "github.com/DataDog/datadog-agent/pkg/serializer/metrics/grpc"
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

// metricsUseStateful returns true if the stateful gRPC path is enabled for
// this metric kind. PoC scope is series-only; sketches stays on HTTP.
//
// Config: serializer_experimental_use_v3_stateful_api.series.enabled (bool)
// per contract.md D9. When true, replaces the v3-over-HTTP path for the
// default (non-MRF, non-local) resolver. Other resolvers stay on HTTP.
func metricsUseStateful(config config.Component, kind metricsKind) bool {
	if kind != metricsKindSeries {
		return false // PoC: sketches not in scope
	}
	return config.GetBool("serializer_experimental_use_v3_stateful_api.series.enabled")
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
	useStateful := metricsUseStateful(s.config, kind) && s.statefulSender != nil && s.statefulDict != nil

	for _, resolver := range s.Forwarder.GetDomainResolvers() {
		useV3 := metricsUseV3(resolver, s.config, kind)
		validateV3 := useV3 && validateV3
		batchID := ""
		if validateV3 {
			batchID = s.genUUID()
		}

		dest := metrics.PipelineDestination{
			Resolver:          resolver,
			Endpoint:          metricsEndpointFor(kind, useV3),
			ValidationBatchID: batchID,
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
			// Stateful path replaces v3-over-HTTP for the default resolver
			// when serializer_experimental_use_v3_stateful_api.series.enabled
			// is set. The HTTP-v3 pipeline is NOT added for this resolver
			// (per contract.md D9 "replaced, not duplicated").
			if useStateful {
				conf := metrics.PipelineConfig{
					Filter:   metrics.AllowAllFilter{},
					V3:       true,
					Stateful: true,
				}
				pipelines.Add(conf, dest)
				// Borrow the serializer's stream-scoped dict + sender on
				// this pipeline's context. The encoder uses them for
				// interning + Submit, without owning them.
				ctx := pipelines[conf]
				ctx.StatefulDict = s.statefulDict
				ctx.StatefulSender = s.statefulSender
				continue
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
					Endpoint:          metricsEndpointFor(kind, false),
					ValidationBatchID: batchID,
				}
				pipelines.Add(vconf, vdest)
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

// Process-wide singletons for the stateful gRPC path. The AgentDemultiplexer
// constructs multiple Serializer instances (sharedSerializer + noAggSerializer
// in demultiplexer_agent.go), each of which would otherwise build its own
// gRPC sender + dictionary. To avoid (a) duplicate gRPC connections and (b)
// divergent dict ID spaces feeding the same intake stream, we centralize the
// stateful state in package-level singletons protected by sync.Once.
//
// Both Serializer instances reference the same statefulSenderShared and
// statefulDictShared; only the first NewSerializer call (with stateful
// enabled) constructs them and calls Start(). Subsequent calls fast-path
// to the shared instances.
var (
	statefulSingletonOnce sync.Once
	statefulSenderShared  *statefulgrpc.Sender
	statefulDictShared    *metrics.StreamDictionary
)

// initStatefulSingleton constructs the process-wide Sender + Dictionary
// the first time it's called. Idempotent: subsequent calls are no-ops.
// On construction failure, the singletons remain nil and the agent falls
// back to the HTTP v3 path (buildPipelines guards on nil sender).
func initStatefulSingleton(cfg config.Component, logger log.Component) {
	statefulSingletonOnce.Do(func() {
		sender, err := newStatefulSender(cfg, logger)
		if err != nil {
			logger.Warnf("stateful v3 metrics path is enabled but sender construction failed (%v); falling back to HTTP v3", err)
			return
		}
		statefulSenderShared = sender
		statefulDictShared = metrics.NewStreamDictionary()
		sender.Start()
		logger.Infof("stateful v3 metrics path enabled and sender started")
	})
}

// newStatefulSender constructs the stateful gRPC sender from config under
// serializer_experimental_use_v3_stateful_api.series.grpc.* (contract.md D9).
// Returns an error if the host is unset (config error) or the gRPC client
// cannot be constructed.
func newStatefulSender(cfg config.Component, _ log.Component) (*statefulgrpc.Sender, error) {
	apiKey := cfg.GetString("api_key")
	host := cfg.GetString("serializer_experimental_use_v3_stateful_api.series.grpc.host")
	if host == "" {
		return nil, fmt.Errorf("serializer_experimental_use_v3_stateful_api.series.grpc.host is required when series.enabled is true")
	}
	port := cfg.GetInt("serializer_experimental_use_v3_stateful_api.series.grpc.port")
	useSSL := cfg.GetBool("serializer_experimental_use_v3_stateful_api.series.grpc.use_ssl")
	compressionKind := cfg.GetString("serializer_experimental_use_v3_stateful_api.series.grpc.compression_kind")
	compressionLevel := cfg.GetInt("serializer_experimental_use_v3_stateful_api.series.grpc.compression_level")
	streamLifetime := time.Duration(cfg.GetInt("serializer_experimental_use_v3_stateful_api.series.grpc.stream_lifetime")) * time.Second
	maxInflight := cfg.GetInt("serializer_experimental_use_v3_stateful_api.series.grpc.max_inflight_payloads")
	drainTimeout := time.Duration(cfg.GetInt("serializer_experimental_use_v3_stateful_api.series.grpc.drain_timeout")) * time.Second
	connectionTimeout := time.Duration(cfg.GetInt("serializer_experimental_use_v3_stateful_api.series.grpc.connection_timeout")) * time.Second

	return statefulgrpc.NewSender(statefulgrpc.Config{
		Host:              host,
		Port:              port,
		APIKey:            apiKey,
		UseSSL:            useSSL,
		UseCompression:    compressionKind != "" && compressionKind != "identity",
		CompressionKind:   compressionKind,
		CompressionLevel:  compressionLevel,
		StreamLifetime:    streamLifetime,
		ConnectionTimeout: connectionTimeout,
		DrainTimeout:      drainTimeout,
		MaxInflight:       maxInflight,
		// Backoff: reuse forwarder defaults.
		BackoffFactor:    cfg.GetFloat64("forwarder_backoff_factor"),
		BackoffBase:      time.Duration(cfg.GetInt("forwarder_backoff_base")) * time.Second,
		BackoffMax:       time.Duration(cfg.GetInt("forwarder_backoff_max")) * time.Second,
		RecoveryInterval: cfg.GetInt("forwarder_recovery_interval"),
	})
}

// Note: the stateful gRPC sender's Start() is called once at process
// initialization via initStatefulSingleton (see above). PoC accepts no
// graceful Stop() — process exit cleans up the gRPC connection. A future
// revision could plug into a component lifecycle.
