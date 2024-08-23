// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

// Package impl implements the grpcClient component interface
package grpcClientimpl

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"net"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/metadata"

	"github.com/golang/protobuf/ptypes/empty"

	"github.com/DataDog/datadog-agent/comp/api/authtoken"
	"github.com/DataDog/datadog-agent/comp/core/config"
	grpcClient "github.com/DataDog/datadog-agent/comp/core/grpcClient/def"
	compdef "github.com/DataDog/datadog-agent/comp/def"

	pb "github.com/DataDog/datadog-agent/pkg/proto/pbgo/core"
)

// Requires defines the dependencies for the grpcClient component
type Requires struct {
	Lifecycle compdef.Lifecycle
	AuthToken authtoken.Component
	Config    config.Component
}

// Provides defines the output of the grpcClient component
type Provides struct {
	Comp grpcClient.Component
}

type client struct {
	ctx    context.Context
	cancel context.CancelFunc
	c      pb.AgentSecureClient
	conn   *grpc.ClientConn
	token  string
}

func (c *client) TaggerStreamEntities(ctx context.Context, in *pb.StreamTagsRequest, opts ...grpc.CallOption) (pb.AgentSecure_TaggerStreamEntitiesClient, error) {
	return c.c.TaggerStreamEntities(ctx, in, opts...)
}

func (c *client) TaggerFetchEntity(ctx context.Context, in *pb.FetchEntityRequest, opts ...grpc.CallOption) (*pb.FetchEntityResponse, error) {
	return c.c.TaggerFetchEntity(ctx, in, opts...)
}

func (c *client) DogstatsdCaptureTrigger(ctx context.Context, in *pb.CaptureTriggerRequest, opts ...grpc.CallOption) (*pb.CaptureTriggerResponse, error) {
	return c.c.DogstatsdCaptureTrigger(ctx, in, opts...)
}

func (c *client) DogstatsdSetTaggerState(ctx context.Context, in *pb.TaggerState, opts ...grpc.CallOption) (*pb.TaggerStateResponse, error) {
	return c.c.DogstatsdSetTaggerState(ctx, in, opts...)
}
func (c *client) ClientGetConfigs(ctx context.Context, in *pb.ClientGetConfigsRequest, opts ...grpc.CallOption) (*pb.ClientGetConfigsResponse, error) {
	return c.c.ClientGetConfigs(ctx, in, opts...)
}
func (c *client) GetConfigState(ctx context.Context, in *empty.Empty, opts ...grpc.CallOption) (*pb.GetStateConfigResponse, error) {
	return c.c.GetConfigState(ctx, in, opts...)
}
func (c *client) ClientGetConfigsHA(ctx context.Context, in *pb.ClientGetConfigsRequest, opts ...grpc.CallOption) (*pb.ClientGetConfigsResponse, error) {
	return c.c.ClientGetConfigsHA(ctx, in, opts...)
}
func (c *client) GetConfigStateHA(ctx context.Context, in *empty.Empty, opts ...grpc.CallOption) (*pb.GetStateConfigResponse, error) {
	return c.c.GetConfigStateHA(ctx, in, opts...)
}

func (c *client) WorkloadmetaStreamEntities(ctx context.Context, in *pb.WorkloadmetaStreamRequest, opts ...grpc.CallOption) (pb.AgentSecure_WorkloadmetaStreamEntitiesClient, error) {
	return c.c.WorkloadmetaStreamEntities(ctx, in, opts...)
}
func (c *client) AutodiscoveryStreamConfig(ctx context.Context, in *empty.Empty, opts ...grpc.CallOption) (pb.AgentSecure_AutodiscoveryStreamConfigClient, error) {
	return c.c.AutodiscoveryStreamConfig(ctx, in, opts...)
}
func (c *client) NewStreamContextWithTimeout(timeout time.Duration) (context.Context, context.CancelFunc) {
	return context.WithTimeout(
		metadata.NewOutgoingContext(
			c.ctx,
			metadata.MD{
				"authorization": []string{
					fmt.Sprintf("Bearer %s", c.token),
				},
			},
		),
		timeout,
	)
}

func (c *client) NewStreamContext() (context.Context, context.CancelFunc) {
	return context.WithCancel(
		metadata.NewOutgoingContext(
			c.ctx,
			metadata.MD{
				"authorization": []string{
					fmt.Sprintf("Bearer %s", c.token),
				},
			},
		),
	)
}

func (c *client) Cancel() {
	c.cancel()
}

func (c *client) Context() context.Context {
	return c.ctx
}

// NewComponent creates a new grpcClient component
func NewComponent(reqs Requires) (Provides, error) {
	ctx, cancel := context.WithCancel(context.Background())

	opts := []grpc.DialOption{grpc.WithContextDialer(func(ctx context.Context, url string) (net.Conn, error) {
		return net.Dial("tcp", url)
	})}

	// NOTE: we're using InsecureSkipVerify because the gRPC server only
	// persists its TLS certs in memory, and we currently have no
	// infrastructure to make them available to clients. This is NOT
	// equivalent to grpc.WithInsecure(), since that assumes a non-TLS
	// connection.
	creds := credentials.NewTLS(&tls.Config{
		InsecureSkipVerify: true,
	})
	opts = append(opts, grpc.WithTransportCredentials(creds))

	conn, err := grpc.DialContext( //nolint:staticcheck // TODO (ASC) fix grpc.DialContext is deprecated
		ctx,
		fmt.Sprintf("%v:%v", reqs.Config.GetString("cmd_host"), reqs.Config.GetInt("cmd_port")),
		opts...,
	)
	if err != nil {
		return Provides{}, err
	}

	token := reqs.AuthToken.Get()

	if token == "" {
		return Provides{}, errors.New("auth token is empty")
	}

	c := pb.NewAgentSecureClient(conn)

	client := &client{
		cancel: cancel,
		ctx:    ctx,
		c:      c,
		token:  token,
		conn:   conn,
	}

	reqs.Lifecycle.Append(compdef.Hook{
		OnStop: func(ctx context.Context) error {
			conn.Close()
			cancel()
			return nil
		},
	})

	return Provides{
		Comp: client,
	}, nil
}
