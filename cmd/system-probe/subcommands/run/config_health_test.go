// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package run

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/protobuf/types/known/emptypb"

	"github.com/DataDog/datadog-agent/comp/core/config"
	"github.com/DataDog/datadog-agent/comp/healthplatform/issues/invalidconfig"
	"github.com/DataDog/datadog-agent/pkg/config/model"
	pb "github.com/DataDog/datadog-agent/pkg/proto/pbgo/core"
)

type configHealthClient struct {
	pb.AgentSecureClient
	report  *pb.ReportHealthIssueRequest
	resolve *pb.ResolveHealthIssueRequest
	err     error
}

func (c *configHealthClient) ReportHealthIssue(_ context.Context, req *pb.ReportHealthIssueRequest, _ ...grpc.CallOption) (*emptypb.Empty, error) {
	c.report = req
	return &emptypb.Empty{}, c.err
}

func (c *configHealthClient) ResolveHealthIssue(_ context.Context, req *pb.ResolveHealthIssueRequest, _ ...grpc.CallOption) (*emptypb.Empty, error) {
	c.resolve = req
	return &emptypb.Empty{}, c.err
}

func TestConfigHealthReportAndResolve(t *testing.T) {
	cfg := config.NewMockFromYAML(t, `dogstatsd_port: "9000"`)
	client := &configHealthClient{err: errors.New("IPC unavailable")}
	id := invalidconfig.ConfigAdjustmentIssueID(invalidconfig.ConversionIssueID, "system-probe", "test-host")
	require.Error(t, reportConfigConversions(context.Background(), client, cfg, id, true))
	require.Nil(t, client.resolve)
	require.Len(t, cfg.GetConfigTypeConversions(), 1)
	client.err = nil
	require.NoError(t, reportConfigConversions(context.Background(), client, cfg, id, true))
	require.Equal(t, id, client.report.Issue.Id)
	require.Empty(t, client.report.RemoteAgentSessionId)
	require.Contains(t, client.report.Issue.Description, "System-probe converted")
	require.NotContains(t, client.report.Issue.Description, "9000")
	cfg.Set("dogstatsd_port", 9000, model.SourceFile)
	require.NoError(t, reportConfigConversions(context.Background(), client, cfg, id, true))
	require.Equal(t, id, client.resolve.IssueId)
	// Disabling the check must also resolve a previously reported conversion.
	cfg.Set("dogstatsd_port", "9001", model.SourceFile)
	require.NoError(t, reportConfigConversions(context.Background(), client, cfg, id, false))
	require.Equal(t, id, client.resolve.IssueId)
}
