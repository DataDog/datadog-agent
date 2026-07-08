// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2021-present Datadog, Inc.

package serializerexporter

import (
	"context"
	"errors"
	"net/http"
	"strings"

	"github.com/DataDog/datadog-agent/comp/core/config"
	delegatedauthnoopfx "github.com/DataDog/datadog-agent/comp/core/delegatedauth/fx-noop"
	logdef "github.com/DataDog/datadog-agent/comp/core/log/def"
	secrets "github.com/DataDog/datadog-agent/comp/core/secrets/def"
	secretnooptypes "github.com/DataDog/datadog-agent/comp/core/secrets/noop-impl/types"
	defaultforwarder "github.com/DataDog/datadog-agent/comp/forwarder/defaultforwarder/def"
	defaultforwarderfx "github.com/DataDog/datadog-agent/comp/forwarder/defaultforwarder/fx"
	defaultforwarderimpl "github.com/DataDog/datadog-agent/comp/forwarder/defaultforwarder/impl"
	"github.com/DataDog/datadog-agent/comp/forwarder/orchestrator/orchestratorinterface"
	metricscompression "github.com/DataDog/datadog-agent/comp/serializer/metricscompression/def"
	metricscompressionfx "github.com/DataDog/datadog-agent/comp/serializer/metricscompression/fx-otel"
	"github.com/DataDog/datadog-agent/pkg/config/create"
	pkgconfigmodel "github.com/DataDog/datadog-agent/pkg/config/model"
	pkgconfigsetup "github.com/DataDog/datadog-agent/pkg/config/setup"
	configutils "github.com/DataDog/datadog-agent/pkg/config/utils"
	"github.com/DataDog/datadog-agent/pkg/serializer"
	"github.com/DataDog/datadog-agent/pkg/util/compression"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
	utilhttp "github.com/DataDog/datadog-agent/pkg/util/http"

	"github.com/open-telemetry/opentelemetry-collector-contrib/pkg/datadog"
	"go.uber.org/fx"
	"go.uber.org/fx/fxevent"
	"go.uber.org/zap"
	"golang.org/x/net/http/httpproxy"

	"github.com/DataDog/datadog-agent/pkg/opentelemetry-mapping-go/otlp/attributes/source"
)

const megaByte = 1024 * 1024

func setupForwarder(config pkgconfigmodel.Config) {
	// Forwarder
	config.Set("additional_endpoints", map[string][]string{}, pkgconfigmodel.SourceDefault)
	config.Set("forwarder_timeout", 20, pkgconfigmodel.SourceDefault)
	config.Set("forwarder_connection_reset_interval", 0, pkgconfigmodel.SourceDefault)                                               // in seconds, 0 means disabled
	config.Set("forwarder_apikey_validation_interval", pkgconfigsetup.DefaultAPIKeyValidationInterval, pkgconfigmodel.SourceDefault) // in minutes

	config.Set("forwarder_num_workers", 1, pkgconfigmodel.SourceDefault)
	config.Set("forwarder_stop_timeout", 2, pkgconfigmodel.SourceDefault)
	config.Set("forwarder_http_protocol", "auto", pkgconfigmodel.SourceDefault)

	// Forwarder retry settings
	config.Set("forwarder_backoff_factor", 2, pkgconfigmodel.SourceDefault)
	config.Set("forwarder_backoff_base", 2, pkgconfigmodel.SourceDefault)
	config.Set("forwarder_backoff_max", 64, pkgconfigmodel.SourceDefault)
	config.Set("forwarder_recovery_interval", pkgconfigsetup.DefaultForwarderRecoveryInterval, pkgconfigmodel.SourceDefault)
	config.Set("forwarder_recovery_reset", false, pkgconfigmodel.SourceDefault)

	// Forwarder storage on disk
	config.Set("forwarder_storage_path", "", pkgconfigmodel.SourceDefault)
	config.Set("forwarder_outdated_file_in_days", 10, pkgconfigmodel.SourceDefault)
	config.Set("forwarder_flush_to_disk_mem_ratio", 0.5, pkgconfigmodel.SourceDefault)
	config.Set("forwarder_storage_max_size_in_bytes", 0, pkgconfigmodel.SourceDefault)                // 0 means disabled. This is a BETA feature.
	config.Set("forwarder_storage_max_disk_ratio", 0.80, pkgconfigmodel.SourceDefault)                // Do not store transactions on disk when the disk usage exceeds 80% of the disk capacity. Use 80% as some applications do not behave well when the disk space is very small.
	config.Set("forwarder_retry_queue_capacity_time_interval_sec", 900, pkgconfigmodel.SourceDefault) // 15 mins

	// Forwarder channels buffer size
	config.Set("forwarder_high_prio_buffer_size", 100, pkgconfigmodel.SourceDefault)
	config.Set("forwarder_low_prio_buffer_size", 100, pkgconfigmodel.SourceDefault)
	config.Set("forwarder_requeue_buffer_size", 100, pkgconfigmodel.SourceDefault)
}

func setupSerializer(config pkgconfigmodel.Config, cfg *ExporterConfig) {
	// Serializer
	config.Set("enable_json_stream_shared_compressor_buffers", true, pkgconfigmodel.SourceDefault)

	// Warning: do not change the following values. Your payloads will get dropped by Datadog's intake.
	config.Set("serializer_max_payload_size", 2*megaByte+megaByte/2, pkgconfigmodel.SourceDefault)
	config.Set("serializer_max_uncompressed_payload_size", 4*megaByte, pkgconfigmodel.SourceDefault)
	config.Set("serializer_max_series_points_per_payload", 10000, pkgconfigmodel.SourceDefault)
	config.Set("serializer_max_series_payload_size", 512000, pkgconfigmodel.SourceDefault)
	config.Set("serializer_max_series_uncompressed_payload_size", 5242880, pkgconfigmodel.SourceDefault)
	config.Set("serializer_compressor_kind", pkgconfigsetup.DefaultCompressorKind, pkgconfigmodel.SourceDefault)
	config.Set("serializer_zstd_compressor_level", pkgconfigsetup.DefaultZstdCompressionLevel, pkgconfigmodel.SourceDefault)

	config.Set("use_v2_api.series", true, pkgconfigmodel.SourceDefault)

	// The serializer exporter forces zlib compression (metricscompressionfx
	// fx-otel), which is incompatible with the v3 metrics intake.
	config.Set("use_v3_api.series.enabled", "false", pkgconfigmodel.SourceAgentRuntime)
	config.Set("serializer_experimental_use_v3_api.series.shadow_sample_rate", float64(0), pkgconfigmodel.SourceAgentRuntime)

	// Serializer: allow user to blacklist any kind of payload to be sent
	config.Set("enable_payloads.events", true, pkgconfigmodel.SourceDefault)
	config.Set("enable_payloads.series", true, pkgconfigmodel.SourceDefault)
	config.Set("enable_payloads.service_checks", true, pkgconfigmodel.SourceDefault)
	config.Set("enable_payloads.sketches", true, pkgconfigmodel.SourceDefault)
	config.Set("enable_payloads.json_to_v1_intake", true, pkgconfigmodel.SourceDefault)

	// Proxy Setup
	proxyConfig := httpproxy.FromEnvironment()
	if proxyConfig.HTTPProxy != "" {
		config.Set("proxy.http", proxyConfig.HTTPProxy, pkgconfigmodel.SourceDefault)
	}
	if proxyConfig.HTTPSProxy != "" {
		config.Set("proxy.https", proxyConfig.HTTPSProxy, pkgconfigmodel.SourceDefault)
	}

	// ProxyURL takes precedence over proxy environment variables if set
	if cfg.HTTPConfig.ProxyURL != "" {
		config.Set("proxy.http", cfg.HTTPConfig.ProxyURL, pkgconfigmodel.SourceFile)
		config.Set("proxy.https", cfg.HTTPConfig.ProxyURL, pkgconfigmodel.SourceFile)
	}

	// Handle no_proxy environment variable
	var noProxy []any
	for v := range strings.SplitSeq(proxyConfig.NoProxy, ",") {
		noProxy = append(noProxy, v)
	}
	config.Set("proxy.no_proxy", noProxy, pkgconfigmodel.SourceAgentRuntime)
}

// ForwarderLifecycle is the minimum interface needed to manage a forwarder's
// lifecycle. Returned by InitSerializer so callers do not need to depend on the
// concrete forwarder type, which varies with the UseSyncForwarder feature gate.
type ForwarderLifecycle interface {
	Start() error
	Stop()
}

// stoppableForwarder is an internal alias for ForwarderLifecycle.
type stoppableForwarder = ForwarderLifecycle

// InitSerializer initializes the serializer and the forwarder behind it.
// Should only be used in the OSS Datadog exporter or in tests.
func InitSerializer(logger *zap.Logger, cfg *ExporterConfig, sourceProvider source.Provider) (*serializer.Serializer, ForwarderLifecycle, error) {
	return initSerializerInternal(logger, cfg, sourceProvider)
}

// initSerializerInternal builds the serializer and a forwarder.
// When the UseSyncForwarder feature gate is enabled it creates an OTelSyncForwarder;
// otherwise it creates a DefaultForwarder. Used only by the OSS Datadog exporter.
// DDOT injects its forwarder directly through the Fx graph in
// cmd/otel-agent/subcommands/run/command.go.
func initSerializerInternal(logger *zap.Logger, cfg *ExporterConfig, sourceProvider source.Provider) (*serializer.Serializer, stoppableForwarder, error) {
	var f defaultforwarder.Forwarder
	var s *serializer.Serializer

	opts := []fx.Option{
		fx.WithLogger(func(log *zap.Logger) fxevent.Logger {
			return &fxevent.ZapLogger{Logger: log}
		}),
		fx.Supply(logger),
		fxutil.FxAgentBase(),
		fx.Provide(func() config.Component {
			pkgconfig := create.NewConfig("DD")
			pkgconfigsetup.InitConfig(pkgconfig)
			pkgconfig.BuildSchema()

			// Set the API Key
			pkgconfig.Set("api_key", string(cfg.API.Key), pkgconfigmodel.SourceFile)
			pkgconfig.Set("site", cfg.API.Site, pkgconfigmodel.SourceFile)
			if cfg.Metrics.Metrics.TCPAddrConfig.Endpoint != "" {
				pkgconfig.Set("dd_url", cfg.Metrics.Metrics.TCPAddrConfig.Endpoint, pkgconfigmodel.SourceFile)
			}
			setupSerializer(pkgconfig, cfg)
			setupForwarder(pkgconfig)
			pkgconfig.Set("skip_ssl_validation", cfg.ClientConfig.InsecureSkipVerify, pkgconfigmodel.SourceFile)

			// Disable regular "Successfully posted payload" logs, since flushing is user-controlled and may happen frequently.
			// Successful export operations can be monitored with exporterhelper metrics.
			pkgconfig.Set("logging_frequency", int64(0), pkgconfigmodel.SourceAgentRuntime)

			return pkgconfig
		}),
		fx.Provide(func(log *zap.Logger) (logdef.Component, error) {
			zp := &datadog.Zaplogger{Logger: log}
			return zp, nil
		}),
		// this is the hostname argument for serializer.NewSerializer
		// this should probably be wrapped by a type
		fx.Provide(func() string {
			s, err := sourceProvider.Source(context.TODO())
			if err != nil {
				return ""
			}
			return s.Identifier.Primary
		}),
		fx.Provide(newOrchestratorinterfaceimpl),
		fx.Provide(serializer.NewSerializer),
		metricscompressionfx.Module(),
		fx.Provide(func(c metricscompression.Component) compression.Compressor {
			return c
		}),
		fx.Provide(func() secrets.Component { return &secretnooptypes.SecretNoop{} }),
		delegatedauthnoopfx.Module(),
		fx.Populate(&f),
		fx.Populate(&s),
	}

	if IsSyncForwarderEnabled() {
		opts = append(opts,
			fx.Provide(func(c config.Component, l logdef.Component, sec secrets.Component) (defaultforwarder.Forwarder, error) {
				eds, err := configutils.GetMultipleEndpoints(c)
				if err != nil {
					return nil, err
				}
				timeout := cfg.HTTPConfig.Timeout
				if timeout == 0 {
					timeout = LegacyForwarderTimeout
				}
				httpClient := &http.Client{
					Timeout:   timeout,
					Transport: utilhttp.CreateHTTPTransport(c),
				}
				return defaultforwarderimpl.NewOTelSyncForwarder(c, l, sec, eds, httpClient)
			}),
		)
	} else {
		opts = append(opts,
			// casts the defaultforwarder.Component to a defaultforwarder.Forwarder
			fx.Provide(func(c defaultforwarder.Component) (defaultforwarder.Forwarder, error) {
				return defaultforwarder.Forwarder(c), nil
			}),
			defaultforwarderfx.Module(defaultforwarder.NewParams()),
		)
	}

	app := fx.New(opts...)
	if err := app.Err(); err != nil {
		return nil, nil, err
	}

	sf, ok := f.(stoppableForwarder)
	if !ok {
		return nil, nil, errors.New("forwarder does not implement Start/Stop lifecycle")
	}
	return s, sf, nil
}

type orchestratorinterfaceimpl struct {
	f defaultforwarder.Forwarder
}

func newOrchestratorinterfaceimpl(f defaultforwarder.Forwarder) orchestratorinterface.Component {
	return &orchestratorinterfaceimpl{
		f: f,
	}
}

func (o *orchestratorinterfaceimpl) Get() (defaultforwarder.Forwarder, bool) {
	return o.f, true
}
