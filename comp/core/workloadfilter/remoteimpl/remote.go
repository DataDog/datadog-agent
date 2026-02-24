// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package remoteimpl contains the remote implementation of the workloadfilter component.
package remoteimpl

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"net"
	"strconv"

	"github.com/mdlayher/vsock"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"

	"github.com/DataDog/datadog-agent/comp/core/config"
	flaretypes "github.com/DataDog/datadog-agent/comp/core/flare/types"
	ipc "github.com/DataDog/datadog-agent/comp/core/ipc/def"
	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	coretelemetry "github.com/DataDog/datadog-agent/comp/core/telemetry"
	baseimpl "github.com/DataDog/datadog-agent/comp/core/workloadfilter/baseimpl"
	"github.com/DataDog/datadog-agent/comp/core/workloadfilter/catalog"
	workloadfilter "github.com/DataDog/datadog-agent/comp/core/workloadfilter/def"
	"github.com/DataDog/datadog-agent/comp/core/workloadfilter/program"
	compdef "github.com/DataDog/datadog-agent/comp/def"
	pb "github.com/DataDog/datadog-agent/pkg/proto/pbgo/core"
	"github.com/DataDog/datadog-agent/pkg/util/common"
	"github.com/DataDog/datadog-agent/pkg/util/system/socket"
)

/*
Below is a diagram of the workloadfilter component in the core agent and a remote agent.

  Workloadfilter component in                            Remote Agent
  core agent evaluates the          ┌─────────────────────────────────────────────────┐
  filter program result             │                ╔═══════════════════╗            │
                                    │                ║                   ║            │
                                    │                ║      Client       ║            │
        Core Agent                  │                ║                   ║            │
┌──────────────────────────┐        │                ╚══▲═══╤════════════╝            │
│       WorkloadFilter     │        │         Remote    │   │          Remote         │
│  ┌─────────────────────┐ │        │     FilterProgram │   │      WorkloadFilter     │
│  │  ╔════════╗         │ │        │   ┌───────────────┼───┼─┐   ┌───────────────┐   │
│  │  ║        ║         │ │        │   │ ┌─────────┐   │   │ │   │               │   │
│  │  ║ Filter ║       ──┼─┼────────┼───┼─┼▶      ──┼───┘   │ │   │   Agent IPC   │   │
│  │  ║ Store◁─╫─▷ Eval  │ │        │   │ │  Cache  │       │ │   │     Client    │   │
│  │  ║        ║       ◀─┼─┼────────┼───┼─┼─      ◀─┼───────┘ │   │               │   │
│  │  ╚════════╝         │ │        │   │ └────▲────┘         │   │       │       │   │
│  └─────────────────────┘ │        │   └──────┼──────────────┘   └───────┼───────┘   │
└──────────────────────────┘        │          └──────────────────────────┘           │
                                    └─────────────────────────────────────────────────┘
*/

// remoteFilterStore is the remote implementation of the workloadfilter component.
type remoteFilterStore struct {
	*baseimpl.BaseFilterStore

	conn      *grpc.ClientConn
	tlsConfig *tls.Config
	authToken string
	target    string
	ctx       context.Context
	cancel    context.CancelFunc
	client    pb.AgentSecureClient
}

// Requires defines the dependencies for the remote workloadfilter.
type Requires struct {
	compdef.In

	Lc        compdef.Lifecycle
	Config    config.Component
	Log       log.Component
	Telemetry coretelemetry.Component
	IPC       ipc.Component
}

// Provides defines the fields provided by the remote workloadfilter constructor.
type Provides struct {
	compdef.Out

	Comp          workloadfilter.Component
	FlareProvider flaretypes.Provider
}

// NewComponent returns a new remote filter client
func NewComponent(req Requires) (Provides, error) {
	remoteFilter := newFilter(req.Config, req.Log, req.Telemetry, req.IPC)

	req.Lc.Append(compdef.Hook{
		OnStart: remoteFilter.start,
		OnStop:  remoteFilter.stop,
	})

	return Provides{
		Comp:          remoteFilter,
		FlareProvider: flaretypes.NewProvider(remoteFilter.FlareCallback),
	}, nil
}

// newFilter creates a remote implementation
func newFilter(cfg config.Component, logger log.Component, telemetryComp coretelemetry.Component, ipc ipc.Component) *remoteFilterStore {
	base := baseimpl.NewBaseFilterStore(cfg, logger, telemetryComp)

	remoteFilter := &remoteFilterStore{
		BaseFilterStore: base,
		tlsConfig:       ipc.GetTLSClientConfig(),
		authToken:       ipc.GetAuthToken(),
		target:          fmt.Sprintf(":%v", cfg.GetInt("cmd_port")),
	}

	for _, cfg := range remoteProgramConfig {
		if remoteFilter.FilterConfig.CELProductRules[cfg.productType][cfg.filterID.TargetResource()] != nil {
			fn := func(builder *catalog.ProgramBuilder) program.FilterProgram {
				return catalog.NewRemoteProgram(cfg.filterID.GetFilterName(), cfg.filterID.TargetResource(), builder, remoteFilter)
			}
			remoteFilter.RegisterFactory(cfg.filterID, fn)
		}
	}

	return remoteFilter
}

// Evaluate serves the request to evaluate a program for a given entity.
func (r *remoteFilterStore) Evaluate(_ string, _ workloadfilter.Filterable) (workloadfilter.Result, error) {
	return workloadfilter.Unknown, errors.New("not implemented")
}

func (r *remoteFilterStore) start(_ context.Context) error {

	mainCtx, _ := common.GetMainCtxCancel()
	r.ctx, r.cancel = context.WithCancel(mainCtx)

	var err error
	r.conn, err = grpc.DialContext( //nolint:staticcheck // TODO (ASC) fix grpc.DialContext is deprecated
		r.ctx,
		r.target,
		grpc.WithTransportCredentials(credentials.NewTLS(r.tlsConfig)),
		grpc.WithContextDialer(func(_ context.Context, url string) (net.Conn, error) {
			if vsockAddr := r.Config.GetString("vsock_addr"); vsockAddr != "" {
				_, sPort, err := net.SplitHostPort(url)
				if err != nil {
					return nil, err
				}

				port, err := strconv.Atoi(sPort)
				if err != nil {
					return nil, fmt.Errorf("invalid port for vsock listener: %v", err)
				}

				cid, err := socket.ParseVSockAddress(vsockAddr)
				if err != nil {
					return nil, err
				}

				return vsock.Dial(cid, uint32(port), &vsock.Config{})
			}
			return net.Dial("tcp", url)
		}),
	)
	if err != nil {
		return err
	}

	r.client = pb.NewAgentSecureClient(r.conn)

	r.Log.Info("remote workloadfilter initialized successfully")

	return nil
}

func (r *remoteFilterStore) stop(_ context.Context) error {
	r.cancel()
	if r.conn != nil {
		return r.conn.Close()
	}
	return nil
}

// GetClient returns the secure agent client
func (r *remoteFilterStore) GetClient() (pb.AgentSecureClient, error) {
	if r.client == nil {
		return nil, errors.New("client not initialized")
	}
	return r.client, nil
}

// GetContext returns the context
func (r *remoteFilterStore) GetContext() context.Context {
	return r.ctx
}

// GetAuthToken returns the authentication token
func (r *remoteFilterStore) GetAuthToken() string {
	return r.authToken
}

var remoteProgramConfig = []struct {
	filterID    workloadfilter.FilterIdentifier
	productType workloadfilter.Product
}{
	{
		filterID:    workloadfilter.ContainerCELMetrics,
		productType: workloadfilter.ProductMetrics,
	},
	{
		filterID:    workloadfilter.ContainerCELLogs,
		productType: workloadfilter.ProductLogs,
	},
	{
		filterID:    workloadfilter.ContainerCELSBOM,
		productType: workloadfilter.ProductSBOM,
	},
	{
		filterID:    workloadfilter.ContainerCELGlobal,
		productType: workloadfilter.ProductGlobal,
	},
	{
		filterID:    workloadfilter.KubeServiceCELMetrics,
		productType: workloadfilter.ProductMetrics,
	},
	{
		filterID:    workloadfilter.KubeServiceCELGlobal,
		productType: workloadfilter.ProductGlobal,
	},
	{
		filterID:    workloadfilter.KubeEndpointCELMetrics,
		productType: workloadfilter.ProductMetrics,
	},
	{
		filterID:    workloadfilter.KubeEndpointCELGlobal,
		productType: workloadfilter.ProductGlobal,
	},
	{
		filterID:    workloadfilter.PodCELMetrics,
		productType: workloadfilter.ProductMetrics,
	},
	{
		filterID:    workloadfilter.PodCELGlobal,
		productType: workloadfilter.ProductGlobal,
	},
	{
		filterID:    workloadfilter.ProcessCELLogs,
		productType: workloadfilter.ProductLogs,
	},
	{
		filterID:    workloadfilter.ProcessCELGlobal,
		productType: workloadfilter.ProductGlobal,
	},
}
