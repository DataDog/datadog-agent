// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

// Package statusrclistener provides an RC task listener that collects agent
// status on demand and sends it to the fleet-api backend.
package statusrclistener

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"time"

	"go.uber.org/fx"

	"github.com/DataDog/datadog-agent/comp/core/config"
	"github.com/DataDog/datadog-agent/comp/core/status"
	rcclienttypes "github.com/DataDog/datadog-agent/comp/remote-config/rcclient/types"
	pkgconfigmodel "github.com/DataDog/datadog-agent/pkg/config/model"
	configUtils "github.com/DataDog/datadog-agent/pkg/config/utils"
	httputils "github.com/DataDog/datadog-agent/pkg/util/http"
)

const (
	fleetStatusPath = "/api/unstable/fleet/agent/status"
	httpTimeout     = 60 * time.Second
)

type dependencies struct {
	fx.In

	Status status.Component
	Config config.Component
}

// Provides exposes the RC task listener to the Fx group.
type Provides struct {
	fx.Out

	RCListener rcclienttypes.TaskListenerProvider
}

// NewStatusRCListener creates the RC task listener for agent status collection.
func NewStatusRCListener(deps dependencies) Provides {
	listener := &statusRCListener{
		status: deps.Status,
		config: deps.Config,
	}
	return Provides{
		RCListener: rcclienttypes.NewTaskListener(listener.onAgentTaskEvent),
	}
}

type statusRCListener struct {
	status status.Component
	config pkgconfigmodel.Reader
}

func (l *statusRCListener) onAgentTaskEvent(taskType rcclienttypes.TaskType, task rcclienttypes.AgentTaskConfig) (bool, error) {
	if taskType != rcclienttypes.TaskStatus {
		return false, nil
	}

	collectID, found := task.Config.TaskArgs["collect_id"]
	if !found {
		return true, errors.New("collect_id was not provided in the status agent task")
	}

	statusJSON, err := l.status.GetStatus("json", false)
	if err != nil {
		return true, fmt.Errorf("failed to get agent status: %w", err)
	}

	return true, sendAgentStatus(l.config, collectID, statusJSON)
}

type statusPayload struct {
	CollectID     string          `json:"collect_id"`
	StatusContent json.RawMessage `json:"status_content"`
}

func sendAgentStatus(cfg pkgconfigmodel.Reader, collectID string, statusJSON []byte) error {
	apiKey := configUtils.SanitizeAPIKey(cfg.GetString("api_key"))
	url := configUtils.GetInfraEndpoint(cfg) + fleetStatusPath

	payload := statusPayload{
		CollectID:     collectID,
		StatusContent: json.RawMessage(statusJSON),
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("failed to marshal status payload: %w", err)
	}

	client := &http.Client{
		Transport: httputils.CreateHTTPTransport(cfg),
		Timeout:   httpTimeout,
	}

	req, err := http.NewRequest(http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("failed to create status POST request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("DD-API-KEY", apiKey)

	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("failed to send agent status: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("fleet-api returned non-200 status %d: %s", resp.StatusCode, string(respBody))
	}

	return nil
}
