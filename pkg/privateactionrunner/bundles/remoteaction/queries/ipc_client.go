// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package com_datadoghq_remoteaction_queries

import (
	"context"
	"fmt"
	"strconv"
	"time"

	"github.com/DataDog/datadog-agent/pkg/api/security/cert"
	pkgconfigsetup "github.com/DataDog/datadog-agent/pkg/config/setup"
	agentgrpc "github.com/DataDog/datadog-agent/pkg/util/grpc"
	"github.com/DataDog/datadog-agent/pkg/util/system"
)

// NewDefaultBridgeClient creates the local Agent IPC client used by the registered PAR action.
func NewDefaultBridgeClient() (BridgeClient, error) {
	cfg := pkgconfigsetup.Datadog()

	clientTLSConfig, _, _, err := cert.FetchIPCCert(cfg)
	if err != nil {
		return nil, fmt.Errorf("fetch Agent IPC certificate: %w", err)
	}

	ipcHost, err := agentIPCHost()
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	client, err := agentgrpc.GetDDAgentSecureClient(ctx, ipcHost, strconv.Itoa(cfg.GetInt("cmd_port")), clientTLSConfig)
	if err != nil {
		return nil, fmt.Errorf("create AgentSecure client: %w", err)
	}
	return client, nil
}

func agentIPCHost() (string, error) {
	cfg := pkgconfigsetup.Datadog()
	cmdHostKey := "cmd_host"
	if cfg.IsConfigured("ipc_address") {
		cmdHostKey = "ipc_address"
	}

	ipcHost, err := system.IsLocalAddress(cfg.GetString(cmdHostKey))
	if err != nil {
		return "", fmt.Errorf("%s: %w", cmdHostKey, err)
	}
	return ipcHost, nil
}
