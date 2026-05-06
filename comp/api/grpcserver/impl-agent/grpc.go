// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

// Package agentimpl implements the grpc component interface for the core agent
package agentimpl

import (
	"net/http"

	grpc "github.com/DataDog/datadog-agent/comp/api/grpcserver/def"
	"github.com/DataDog/datadog-agent/comp/collector/collector"
	"github.com/DataDog/datadog-agent/comp/core/autodiscovery"
	"github.com/DataDog/datadog-agent/comp/core/config"
	configstream "github.com/DataDog/datadog-agent/comp/core/configstream/def"
	configstreamServer "github.com/DataDog/datadog-agent/comp/core/configstream/server"
	"github.com/DataDog/datadog-agent/comp/core/hostname/hostnameinterface"
	ipc "github.com/DataDog/datadog-agent/comp/core/ipc/def"
	remoteagentregistry "github.com/DataDog/datadog-agent/comp/core/remoteagentregistry/def"
	secrets "github.com/DataDog/datadog-agent/comp/core/secrets/def"
	tagger "github.com/DataDog/datadog-agent/comp/core/tagger/def"
	taggerserver "github.com/DataDog/datadog-agent/comp/core/tagger/server"
	telemetry "github.com/DataDog/datadog-agent/comp/core/telemetry/def"
	workloadfilter "github.com/DataDog/datadog-agent/comp/core/workloadfilter/def"
	workloadfilterServer "github.com/DataDog/datadog-agent/comp/core/workloadfilter/server"
	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	workloadmetaServer "github.com/DataDog/datadog-agent/comp/core/workloadmeta/server"
	compdef "github.com/DataDog/datadog-agent/comp/def"
	pidmap "github.com/DataDog/datadog-agent/comp/dogstatsd/pidmap/def"
	replay "github.com/DataDog/datadog-agent/comp/dogstatsd/replay/def"
	dogstatsdServer "github.com/DataDog/datadog-agent/comp/dogstatsd/server"
	rcservice "github.com/DataDog/datadog-agent/comp/remote-config/rcservice/def"
	rcservicemrf "github.com/DataDog/datadog-agent/comp/remote-config/rcservicemrf/def"
	pb "github.com/DataDog/datadog-agent/pkg/proto/pbgo/core"
	grpcutil "github.com/DataDog/datadog-agent/pkg/util/grpc"
	"github.com/DataDog/datadog-agent/pkg/util/option"

	googleGrpc "google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
)

// Requires defines the dependencies for the grpc component
type Requires struct {
	compdef.In

	DogstatsdServer     dogstatsdServer.Component
	Capture             replay.Component
	PidMap              pidmap.Component
	SecretResolver      secrets.Component
	RcService           option.Option[rcservice.Component]
	RcServiceMRF        option.Option[rcservicemrf.Component]
	IPC                 ipc.Component
	Tagger              tagger.Component
	TagProcessor        option.Option[tagger.Processor]
	Cfg                 config.Component
	AutoConfig          autodiscovery.Component
	Workloadfilter      workloadfilter.Component
	WorkloadMeta        workloadmeta.Component
	Collector           option.Option[collector.Component]
	RemoteAgentRegistry remoteagentregistry.Component
	Telemetry           telemetry.Component
	Hostname            hostnameinterface.Component
	ConfigStream        configstream.Component
}

type server struct {
	IPC                 ipc.Component
	tagger              tagger.Component
	tagProcessor        option.Option[tagger.Processor]
	workloadMeta        workloadmeta.Component
	workloadfilter      workloadfilter.Component
	configService       option.Option[rcservice.Component]
	configServiceMRF    option.Option[rcservicemrf.Component]
	dogstatsdServer     dogstatsdServer.Component
	capture             replay.Component
	pidMap              pidmap.Component
	remoteAgentRegistry remoteagentregistry.Component
	autodiscovery       autodiscovery.Component
	configComp          config.Component
	telemetry           telemetry.Component
	hostname            hostnameinterface.Component
	configStream        configstream.Component
}

func (s *server) BuildServer() http.Handler {
	// `agent_ipc.grpc_max_message_size` is the canonical setting; the older
	// `cluster_agent.cluster_tagger.grpc_max_message_size` is deprecated but still honoured
	// for backwards compatibility. Use the larger of the two so neither setting can
	// silently shrink the limit.
	ipcMaxMessageSize := s.configComp.GetInt("agent_ipc.grpc_max_message_size")
	legacyMaxMessageSize := s.configComp.GetInt("cluster_agent.cluster_tagger.grpc_max_message_size")
	maxMessageSize := max(ipcMaxMessageSize, legacyMaxMessageSize)

	// Use the convenience function that combines metrics and auth interceptors
	opts := grpcutil.ServerOptionsWithMetricsAndAuth(
		grpcutil.RequireClientCert,
		grpcutil.RequireClientCertStream,
	)

	opts = append(opts,
		googleGrpc.Creds(credentials.NewTLS(s.IPC.GetTLSServerConfig())),
		googleGrpc.MaxRecvMsgSize(maxMessageSize),
		googleGrpc.MaxSendMsgSize(maxMessageSize),
	)

	// Emit telemetry when an outgoing message exceeds the soft threshold, well below
	// the hard `MaxSendMsgSize`. Chained after the metrics+auth interceptors so its
	// ServerStream wrapper is the one the actual handler holds.
	if unary, stream := newOversizedMessageInterceptors(s.configComp.GetInt("agent_ipc.grpc_warning_message_size"), s.telemetry); unary != nil {
		opts = append(opts,
			googleGrpc.ChainUnaryInterceptor(unary),
			googleGrpc.ChainStreamInterceptor(stream),
		)
	}

	// The tagger and workloadmeta servers chunk their batches up to a per-event size cap.
	// They are tuned around the legacy `cluster_agent.cluster_tagger.grpc_max_message_size`
	// (4 MiB by default), so derive the chunk size from that key and not the much larger
	// `agent_ipc.grpc_max_message_size` introduced for configstream — bumping the server
	// frame size shouldn't silently grow tagger/wmeta batch sizes.
	maxEventSize := s.configComp.GetInt("cluster_agent.cluster_tagger.grpc_max_message_size") / 2
	grpcServer := googleGrpc.NewServer(opts...)
	pb.RegisterAgentServer(grpcServer, &agentServer{hostname: s.hostname})
	pb.RegisterAgentSecureServer(grpcServer, &serverSecure{
		configService:    s.configService,
		configServiceMRF: s.configServiceMRF,
		taggerServer:     taggerserver.NewServer(s.tagger, s.telemetry, maxEventSize, s.configComp.GetInt("remote_tagger.max_concurrent_sync")),
		tagProcessor:     s.tagProcessor,
		// TODO(components): decide if workloadmetaServer should be componentized itself
		workloadmetaServer:   workloadmetaServer.NewServer(s.workloadMeta, maxEventSize),
		workloadfilterServer: workloadfilterServer.NewServer(s.workloadfilter),
		dogstatsdServer:      s.dogstatsdServer,
		capture:              s.capture,
		pidMap:               s.pidMap,
		remoteAgentRegistry:  s.remoteAgentRegistry,
		autodiscovery:        s.autodiscovery,
		configComp:           s.configComp,
		configStreamServer:   configstreamServer.NewServer(s.configComp, s.configStream, s.remoteAgentRegistry),
	})

	return grpcServer
}

// Provides defines the output of the grpc component
type Provides struct {
	Comp grpc.Component
}

// NewComponent creates a new grpc component
func NewComponent(reqs Requires) (Provides, error) {
	provides := Provides{
		Comp: &server{
			IPC:                 reqs.IPC,
			configService:       reqs.RcService,
			configServiceMRF:    reqs.RcServiceMRF,
			tagger:              reqs.Tagger,
			tagProcessor:        reqs.TagProcessor,
			workloadMeta:        reqs.WorkloadMeta,
			workloadfilter:      reqs.Workloadfilter,
			dogstatsdServer:     reqs.DogstatsdServer,
			capture:             reqs.Capture,
			pidMap:              reqs.PidMap,
			remoteAgentRegistry: reqs.RemoteAgentRegistry,
			autodiscovery:       reqs.AutoConfig,
			configComp:          reqs.Cfg,
			telemetry:           reqs.Telemetry,
			hostname:            reqs.Hostname,
			configStream:        reqs.ConfigStream,
		},
	}
	return provides, nil
}
