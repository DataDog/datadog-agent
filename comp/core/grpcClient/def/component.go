// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

// Package grpcClient ... /* TODO: detailed doc comment for the component */
package grpcClient

import (
	"context"
	"time"

	"github.com/golang/protobuf/ptypes/empty"
	"google.golang.org/grpc"

	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	pb "github.com/DataDog/datadog-agent/pkg/proto/pbgo/core"
)

// team: /* TODO: add team name */

// Component is the component type.
type Component interface {
	AutodiscoveryStreamConfig(ctx context.Context, in *empty.Empty, opts ...grpc.CallOption) (pb.AgentSecure_AutodiscoveryStreamConfigClient, error)
	WorkloadmetaGetKubernetesPodForContainer(containerID string) (*workloadmeta.KubernetesPod, error)
	WorkloadmetaGetContainer(containerID string) (*workloadmeta.Container, error)
	NewStreamContextWithTimeout(timeout time.Duration) (context.Context, context.CancelFunc)
	NewStreamContext() (context.Context, context.CancelFunc)
	Cancel()
	Context() context.Context
}
