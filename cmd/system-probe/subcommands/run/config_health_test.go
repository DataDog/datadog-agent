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
	spconfig "github.com/DataDog/datadog-agent/pkg/system-probe/config"
	"github.com/DataDog/datadog-agent/pkg/system-probe/config/types"
)

type configHealthClient struct {
	pb.AgentSecureClient
	report  *pb.ReportHealthIssueRequest
	resolve *pb.ResolveHealthIssueRequest
	err     error
}

func TestConfigFallbackReportAndResolve(t *testing.T) {
	cfg := config.NewMock(t)
	cfg.RecordConfigFallback(model.ConfigFallback{Key: "system_probe_config.sysprobe_socket", Consumer: "system-probe", Reason: "must be a supported socket address", DefaultValue: "/opt/datadog-agent/run/sysprobe.sock"})
	client := &configHealthClient{err: errors.New("IPC unavailable")}
	id := invalidconfig.ConfigAdjustmentIssueID(invalidconfig.FallbackIssueID, "system-probe", "test-host")
	require.Error(t, reportConfigFallbacks(context.Background(), client, cfg, id, true))
	require.Len(t, cfg.GetConfigFallbacks(), 1)
	client.err = nil
	require.NoError(t, reportConfigFallbacks(context.Background(), client, cfg, id, true))
	require.Equal(t, id, client.report.Issue.Id)
	require.Equal(t, invalidconfig.SystemProbeFallbackIssueName, client.report.Issue.IssueName)
	require.NoError(t, reportConfigFallbacks(context.Background(), client, cfg, id, false))
	require.Equal(t, id, client.resolve.IssueId)
	cfg.ClearConfigFallback("system_probe_config.sysprobe_socket", "system-probe")
	require.NoError(t, reportConfigFallbacks(context.Background(), client, cfg, id, true))
	require.Equal(t, id, client.resolve.IssueId)
}

func TestConfigFallbacksRequireRunningConsumer(t *testing.T) {
	cfg := config.NewMock(t)
	for _, consumer := range []string{"system-probe", string(spconfig.NetworkTracerModule), "network_process"} {
		cfg.RecordConfigFallback(model.ConfigFallback{Key: consumer, Consumer: consumer})
	}
	require.Len(t, activeConfigFallbacks(cfg, func(types.ModuleName) bool { return false }), 1)
	loaded := func(types.ModuleName) bool { return true }
	cfg.Set("event_monitoring_config.network_process.enabled", false, model.SourceFile)
	require.Len(t, activeConfigFallbacks(cfg, loaded), 2)
	cfg.Set("event_monitoring_config.network_process.enabled", true, model.SourceFile)
	require.Len(t, activeConfigFallbacks(cfg, loaded), 3)
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
