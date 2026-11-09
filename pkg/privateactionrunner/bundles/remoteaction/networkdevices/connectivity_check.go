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
	"net/http"
	"time"

	ipc "github.com/DataDog/datadog-agent/comp/core/ipc/def"
	"github.com/DataDog/datadog-agent/pkg/networkdevices/connectivity"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/adapters/config"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/libs/encryptioncontext"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/libs/privateconnection"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/types"
)

type ConnectivityCheckRequest struct {
	TimeoutSeconds       int32                               `json:"timeout,omitempty"`
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
	cfg             config.Config
	encryptionStore *encryptioncontext.Store
	ipcClient       ipc.HTTPClient
}

func NewConnectivityCheckHandler(cfg config.Config, encryptionStore *encryptioncontext.Store, ipcClient ipc.HTTPClient) *ConnectivityCheckHandler {
	return &ConnectivityCheckHandler{cfg: cfg, encryptionStore: encryptionStore, ipcClient: ipcClient}
}

func (h *ConnectivityCheckHandler) Run(ctx context.Context, task *types.Task, _ *privateconnection.PrivateCredentials) (interface{}, error) {
	if h.ipcClient == nil {
		return nil, errors.New("IPC client is not available")
	}

	req, err := types.ExtractInputs[ConnectivityCheckRequest](task)
	if err != nil {
		return nil, fmt.Errorf("failed to parse connectivityCheck inputs: %w", err)
	}
	if req.TimeoutSeconds > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, time.Duration(req.TimeoutSeconds)*time.Second)
		defer cancel()
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

	url, err := h.connectivityCheckURL()
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

func (h *ConnectivityCheckHandler) connectivityCheckURL() (string, error) {
	baseURL, err := h.cfg.AgentAPIURL()
	if err != nil {
		return "", fmt.Errorf("failed to get IPC address: %w", err)
	}

	return baseURL + "/agent/networkdevices/connectivity-check", nil
}
