// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build clusterchecks

package clustercheckimpl

import (
	"context"
	"net/http"

	"go.uber.org/fx"

	"github.com/DataDog/datadog-agent/comp/core/autodiscovery"
	clusterchecks "github.com/DataDog/datadog-agent/comp/core/clusterchecks/def"
	"github.com/DataDog/datadog-agent/comp/core/config"
	tagger "github.com/DataDog/datadog-agent/comp/core/tagger/def"
	compdef "github.com/DataDog/datadog-agent/comp/def"
)

type dependencies struct {
	fx.In

	Config     config.Component
	Autoconfig autodiscovery.Component
	Tagger     tagger.Component
	Lifecycle  compdef.Lifecycle
}

type provides struct {
	fx.Out

	Component clusterchecks.Component
}

// NewComponent creates a new clusterchecks component
//
//nolint:revive // TODO(CINT) Fix revive linter
func NewComponent(deps dependencies) provides {
	// Only create the component if cluster checks are enabled
	if !deps.Config.GetBool("cluster_checks.enabled") {
		return provides{
			Component: &noopImpl{},
		}
	}

	handler, err := NewHandler(deps.Autoconfig, deps.Tagger)
	if err != nil {
		// Return noop implementation on error
		return provides{
			Component: &noopImpl{},
		}
	}

	// Register lifecycle hooks
	deps.Lifecycle.Append(compdef.Hook{
		OnStart: func(ctx context.Context) error {
			// Run the handler in a goroutine
			go handler.Run(ctx)
			return nil
		},
	})

	return provides{
		Component: handler,
	}
}

// noopImpl is a no-op implementation of the clusterchecks component
type noopImpl struct{}

// Run is a no-op
func (n *noopImpl) Run(_ context.Context) {}

// RejectOrForwardLeaderQuery always returns false for noop
func (n *noopImpl) RejectOrForwardLeaderQuery(_ http.ResponseWriter, _ *http.Request) bool {
	return false
}

// GetState returns empty state for noop
func (n *noopImpl) GetState() (clusterchecks.StateResponse, error) {
	return clusterchecks.StateResponse{NotRunning: "clusterchecks disabled"}, nil
}

// GetConfigs returns empty config for noop
func (n *noopImpl) GetConfigs(_ string) (clusterchecks.ConfigResponse, error) {
	return clusterchecks.ConfigResponse{}, nil
}

// PostStatus returns empty response for noop
func (n *noopImpl) PostStatus(_, _ string, _ clusterchecks.NodeStatus) clusterchecks.StatusResponse {
	return clusterchecks.StatusResponse{}
}

// GetEndpointsConfigs returns empty config for noop
func (n *noopImpl) GetEndpointsConfigs(_ string) (clusterchecks.ConfigResponse, error) {
	return clusterchecks.ConfigResponse{}, nil
}

// GetAllEndpointsCheckConfigs returns empty config for noop
func (n *noopImpl) GetAllEndpointsCheckConfigs() (clusterchecks.ConfigResponse, error) {
	return clusterchecks.ConfigResponse{}, nil
}

// RebalanceClusterChecks returns empty response for noop
func (n *noopImpl) RebalanceClusterChecks(_ bool) ([]clusterchecks.RebalanceResponse, error) {
	return nil, nil
}

// IsolateCheck returns empty response for noop
func (n *noopImpl) IsolateCheck(_ string) clusterchecks.IsolateResponse {
	return clusterchecks.IsolateResponse{}
}

// GetStats returns nil for noop
func (n *noopImpl) GetStats() (*clusterchecks.Stats, error) {
	return nil, nil
}

// GetNodeTypeCounts returns zeros for noop
func (n *noopImpl) GetNodeTypeCounts() (clcRunnerCount, nodeAgentCount int, err error) {
	return 0, 0, nil
}
