// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package run

import (
	"context"
	"net"
	"time"

	"github.com/DataDog/datadog-agent/comp/core/remoteagent/helper"
	"github.com/DataDog/datadog-agent/comp/healthplatform/issues/invalidconfig"
	"github.com/DataDog/datadog-agent/pkg/config/model"
	pb "github.com/DataDog/datadog-agent/pkg/proto/pbgo/core"
	"github.com/DataDog/datadog-agent/pkg/system-probe/api/module"
	spconfig "github.com/DataDog/datadog-agent/pkg/system-probe/config"
	"github.com/DataDog/datadog-agent/pkg/system-probe/config/types"
)

// Report this process's config, not the core Agent's separately loaded copy.
// The secure client works independently of remote-agent registry enrollment.
func startConfigHealth(deps module.FactoryDependencies, running bool) func() {
	address := net.JoinHostPort(deps.CoreConfig.GetString("cmd_host"), deps.CoreConfig.GetString("cmd_port"))
	client, conn, err := helper.NewAgentSecureClient(address, deps.Ipc.GetAuthToken(), deps.Ipc.GetTLSClientConfig(), deps.CoreConfig.GetString("vsock_addr"), deps.Log)
	if err != nil {
		deps.Log.Warnf("Cannot initialize configuration health reporting: %v", err)
		return func() {}
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		defer close(done)
		defer conn.Close()
		ticker := time.NewTicker(time.Minute)
		defer ticker.Stop()
		for {
			reportCtx, reportCancel := context.WithTimeout(ctx, 5*time.Second)
			id := invalidconfig.ConfigAdjustmentIssueID(invalidconfig.ConversionIssueID, "system-probe", deps.Hostname.GetSafe(reportCtx))
			enabled := running && deps.CoreConfig.GetBool("health_platform.enabled") && deps.CoreConfig.GetBool("health_platform.invalidconfig_check.enabled")
			err := reportConfigConversions(reportCtx, client, deps.SysprobeConfig, id, enabled)
			reportCancel()
			if err != nil && ctx.Err() == nil {
				deps.Log.Debugf("Configuration health report will be retried: %v", err)
			}
			reportCtx, reportCancel = context.WithTimeout(ctx, 5*time.Second)
			id = invalidconfig.ConfigAdjustmentIssueID(invalidconfig.FallbackIssueID, "system-probe", deps.Hostname.GetSafe(reportCtx))
			err = reportConfigFallbacks(reportCtx, client, deps.SysprobeConfig, id, enabled)
			reportCancel()
			if err != nil && ctx.Err() == nil {
				deps.Log.Debugf("Configuration fallback report will be retried: %v", err)
			}
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
			}
		}
	}()
	return func() { cancel(); <-done }
}

func reportConfigConversions(ctx context.Context, client pb.AgentSecureClient, cfg model.Reader, id string, enabled bool) error {
	conversions := invalidconfig.FilterConfigConversions(cfg.GetConfigTypeConversions(), activeConfigFallbacks(cfg, module.IsLoaded))
	if !enabled || len(conversions) == 0 {
		_, err := client.ResolveHealthIssue(ctx, &pb.ResolveHealthIssueRequest{IssueId: id})
		return err
	}
	issue, err := invalidconfig.BuildConversionIssue("system-probe", cfg.ConfigFileUsed(), conversions)
	if err != nil {
		return err
	}
	issue.Id = id
	_, err = client.ReportHealthIssue(ctx, &pb.ReportHealthIssueRequest{Issue: issue})
	return err
}

func reportConfigFallbacks(ctx context.Context, client pb.AgentSecureClient, cfg model.Reader, id string, enabled bool) error {
	fallbacks := activeConfigFallbacks(cfg, module.IsLoaded)
	if !enabled || len(fallbacks) == 0 {
		_, err := client.ResolveHealthIssue(ctx, &pb.ResolveHealthIssueRequest{IssueId: id})
		return err
	}
	issue, err := invalidconfig.BuildFallbackIssue("system-probe", cfg.ConfigFileUsed(), fallbacks)
	if err != nil {
		return err
	}
	issue.Id = id
	_, err = client.ReportHealthIssue(ctx, &pb.ReportHealthIssueRequest{Issue: issue})
	return err
}

// Configuration is adjusted even for disabled modules. Only report replacements used by running consumers.
func activeConfigFallbacks(cfg model.Reader, loaded func(types.ModuleName) bool) []model.ConfigFallback {
	var active []model.ConfigFallback
	for _, fallback := range cfg.GetConfigFallbacks() {
		inUse := false
		switch fallback.Consumer {
		case "system-probe":
			inUse = true
		case string(spconfig.NetworkTracerModule):
			inUse = loaded(spconfig.NetworkTracerModule)
		case "network_process":
			inUse = loaded(spconfig.NetworkTracerModule) && cfg.GetBool("event_monitoring_config.network_process.enabled")
		}
		if inUse {
			active = append(active, fallback)
		}
	}
	return active
}
