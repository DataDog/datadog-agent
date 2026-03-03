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
	"github.com/DataDog/datadog-agent/comp/core/telemetry"
	workloadfilter "github.com/DataDog/datadog-agent/comp/core/workloadfilter/def"
	workloadfilterServer "github.com/DataDog/datadog-agent/comp/core/workloadfilter/server"
	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	workloadmetaServer "github.com/DataDog/datadog-agent/comp/core/workloadmeta/server"
	compdef "github.com/DataDog/datadog-agent/comp/def"
	"github.com/DataDog/datadog-agent/comp/dogstatsd/pidmap"
	replay "github.com/DataDog/datadog-agent/comp/dogstatsd/replay/def"
	dogstatsdServer "github.com/DataDog/datadog-agent/comp/dogstatsd/server"
	"github.com/DataDog/datadog-agent/comp/remote-config/rcservice"
	"github.com/DataDog/datadog-agent/comp/remote-config/rcservicemrf"
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
	maxMessageSize := s.configComp.GetInt("cluster_agent.cluster_tagger.grpc_max_message_size")

	// Use the convenience function that combines metrics and auth interceptors
	var opts []googleGrpc.ServerOption
	if vsockAddr := s.configComp.GetString("vsock_addr"); vsockAddr == "" {
		opts = append(opts,
			grpcutil.ServerOptionsWithMetricsAndAuth(
				grpcutil.RequireClientCert,
				grpcutil.RequireClientCertStream,
			)...,
		)
	}

	opts = append(opts,
		googleGrpc.Creds(credentials.NewTLS(s.IPC.GetTLSServerConfig())),
		googleGrpc.MaxRecvMsgSize(maxMessageSize),
		googleGrpc.MaxSendMsgSize(maxMessageSize),
	)

	// event size should be small enough to fit within the grpc max message size
	maxEventSize := maxMessageSize / 2
	grpcServer := googleGrpc.NewServer(opts...)
	pb.RegisterAgentServer(grpcServer, &agentServer{hostname: s.hostname})
	pb.RegisterAgentSecureServer(grpcServer, &serverSecure{
		configService:    s.configService,
		configServiceMRF: s.configServiceMRF,
		taggerServer:     taggerserver.NewServer(s.tagger, s.telemetry, maxEventSize, s.configComp.GetInt("remote_tagger.max_concurrent_sync")),
		tagProcessor:     s.tagProcessor,
		// TODO(components): decide if workloadmetaServer should be componentized itself
		workloadmetaServer:   workloadmetaServer.NewServer(s.workloadMeta),
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
