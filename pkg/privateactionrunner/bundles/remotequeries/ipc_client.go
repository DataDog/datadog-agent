// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package com_datadoghq_remotequeries

import (
	"fmt"
	"net"
	"net/url"
	"strconv"

	ipchttp "github.com/DataDog/datadog-agent/comp/core/ipc/httphelpers"
	pkgtoken "github.com/DataDog/datadog-agent/pkg/api/security"
	"github.com/DataDog/datadog-agent/pkg/api/security/cert"
	pkgconfigmodel "github.com/DataDog/datadog-agent/pkg/config/model"
	pkgconfigsetup "github.com/DataDog/datadog-agent/pkg/config/setup"
	"github.com/DataDog/datadog-agent/pkg/util/system"
)

// NewDefaultBridgeClient creates the local Agent IPC client used by the registered PAR action.
func NewDefaultBridgeClient() (BridgeClient, string, error) {
	cfg := pkgconfigsetup.Datadog()

	token, err := pkgtoken.FetchAuthToken(cfg)
	if err != nil {
		return nil, "", fmt.Errorf("fetch Agent IPC auth token: %w", err)
	}
	clientTLSConfig, _, _, err := cert.FetchIPCCert(cfg)
	if err != nil {
		return nil, "", fmt.Errorf("fetch Agent IPC certificate: %w", err)
	}

	endpointURL, err := agentIPCURL(cfg, AgentRemoteQueryExecuteEndpointPath)
	if err != nil {
		return nil, "", err
	}
	return ipchttp.NewClient(token, clientTLSConfig, cfg), endpointURL, nil
}

func agentIPCURL(cfg pkgconfigmodel.Reader, endpointPath string) (string, error) {
	cmdHostKey := "cmd_host"
	if cfg.IsConfigured("ipc_address") {
		cmdHostKey = "ipc_address"
	}

	ipcHost, err := system.IsLocalAddress(cfg.GetString(cmdHostKey))
	if err != nil {
		return "", fmt.Errorf("%s: %w", cmdHostKey, err)
	}

	endpointURL := url.URL{
		Scheme: "https",
		Host:   net.JoinHostPort(ipcHost, strconv.Itoa(cfg.GetInt("cmd_port"))),
		Path:   endpointPath,
	}
	return endpointURL.String(), nil
}
