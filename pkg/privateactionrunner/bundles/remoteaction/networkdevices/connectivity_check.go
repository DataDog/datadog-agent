// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package com_datadoghq_remoteaction_networkdevices

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"strconv"

	ipc "github.com/DataDog/datadog-agent/comp/core/ipc/def"
	pkgconfighelper "github.com/DataDog/datadog-agent/pkg/config/helper"
	pkgconfigsetup "github.com/DataDog/datadog-agent/pkg/config/setup"
	"github.com/DataDog/datadog-agent/pkg/networkdevices/connectivity"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/libs/encryptioncontext"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/libs/privateconnection"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/types"
)

type ConnectivityCheckRequest struct {
	TargetIPs            []string                            `json:"targetIPs"`
	Checks               []string                            `json:"checks"`
	PingOptions          *connectivity.PingOptions           `json:"pingOptions,omitempty"`
	SNMPOptions          *connectivity.SNMPOptions           `json:"snmpOptions,omitempty"`
	Workers              int                                 `json:"workers,omitempty"`
	EncryptedCredentials string                              `json:"encryptedCredentials"`
	EncryptionContext    encryptioncontext.EncryptionContext `json:"encryptionContext"`
}

type secretInputs struct {
	SNMP []connectivity.SNMPCredential `json:"snmp"`
}

type ConnectivityCheckHandler struct {
	encryptionStore *encryptioncontext.Store
	ipcClient       ipc.HTTPClient
}

func NewConnectivityCheckHandler(encryptionStore *encryptioncontext.Store, ipcClient ipc.HTTPClient) *ConnectivityCheckHandler {
	return &ConnectivityCheckHandler{encryptionStore: encryptionStore, ipcClient: ipcClient}
}

func (h *ConnectivityCheckHandler) Run(ctx context.Context, task *types.Task, _ *privateconnection.PrivateCredentials) (interface{}, error) {
	if h.ipcClient == nil {
		return nil, errors.New("IPC client is not available")
	}

	req, err := types.ExtractInputs[ConnectivityCheckRequest](task)
	if err != nil {
		return nil, fmt.Errorf("failed to parse connectivityCheck inputs: %w", err)
	}

	var secrets secretInputs
	if req.EncryptedCredentials != "" {
		secrets, err = encryptioncontext.DecryptInto[secretInputs](h.encryptionStore, req.EncryptionContext, req.EncryptedCredentials)
		if err != nil {
			return nil, fmt.Errorf("failed to decrypt secret inputs: %w", err)
		}
	}

	res, err := h.runChecks(ctx, req, secrets)
	if err != nil {
		return nil, fmt.Errorf("failed to run connectivity checks: %w", err)
	}

	return res, nil
}

func (h *ConnectivityCheckHandler) runChecks(ctx context.Context, req ConnectivityCheckRequest, secrets secretInputs) (*connectivity.Result, error) {
	body, err := json.Marshal(connectivity.Request{
		Targets:     req.TargetIPs,
		Checks:      req.Checks,
		PingOptions: req.PingOptions,
		SNMPOptions: req.SNMPOptions,
		Workers:     req.Workers,
		Credentials: secrets.SNMP,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	url, err := connectivityCheckURL()
	if err != nil {
		return nil, err
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewBuffer(body))
	if err != nil {
		return nil, fmt.Errorf("failed to build request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")

	respBody, err := h.ipcClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("failed to reach the Agent: %w", err)
	}

	var res connectivity.Result
	if err := json.Unmarshal(respBody, &res); err != nil {
		return nil, fmt.Errorf("failed to parse the Agent response: %w", err)
	}
	return &res, nil
}

func connectivityCheckURL() (string, error) {
	ipcAddress, err := pkgconfighelper.GetIPCAddress(pkgconfigsetup.Datadog())
	if err != nil {
		return "", fmt.Errorf("failed to get IPC address: %w", err)
	}

	port := pkgconfigsetup.Datadog().GetInt("cmd_port")
	return fmt.Sprintf("https://%s/agent/networkdevices/connectivity-check", net.JoinHostPort(ipcAddress, strconv.Itoa(port))), nil
}
