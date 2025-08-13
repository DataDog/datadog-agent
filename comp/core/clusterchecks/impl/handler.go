// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build clusterchecks

// Package clusterchecksimpl implements the clusterchecks handler component
package clusterchecksimpl

import (
	"context"
	"errors"
	"net/http"

	"github.com/DataDog/datadog-agent/comp/core/autodiscovery"
	clusterchecks "github.com/DataDog/datadog-agent/comp/core/clusterchecks/def"
	config "github.com/DataDog/datadog-agent/comp/core/config"
	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	tagger "github.com/DataDog/datadog-agent/comp/core/tagger/def"
	compdef "github.com/DataDog/datadog-agent/comp/def"
	pkgclusterchecks "github.com/DataDog/datadog-agent/pkg/clusteragent/clusterchecks"
	"github.com/DataDog/datadog-agent/pkg/clusteragent/clusterchecks/types"
)

// handlerImpl wraps the existing Handler implementation
// This thin wrapper provides component lifecycle management while
// delegating all business logic to the pkg implementation.
// This pattern avoids exporting internal dispatcher methods.
type handlerImpl struct {
	handler *pkgclusterchecks.Handler
	log     log.Component
}

// Requires defines the dependencies for the clusterchecks handler component
type Requires struct {
	compdef.In

	Log           log.Component
	Config        config.Component
	Autodiscovery autodiscovery.Component
	Tagger        tagger.Component
}

// Provides defines the output of the clusterchecks handler component
type Provides struct {
	compdef.Out

	Component clusterchecks.Component
}

// NewComponent creates a new clusterchecks handler component.
// It acts as a bridge between the Fx component system and the pkg implementation,
// allowing gradual migration without disrupting the existing dispatcher architecture.
func NewComponent(deps Requires) (Provides, error) {
	if deps.Autodiscovery == nil {
		return Provides{}, errors.New("autodiscovery component is required")
	}

	// Create the handler using the existing implementation
	handler, err := pkgclusterchecks.NewHandler(deps.Autodiscovery, deps.Tagger)
	if err != nil {
		return Provides{}, err
	}

	impl := &handlerImpl{
		handler: handler,
		log:     deps.Log,
	}

	deps.Log.Info("Cluster checks handler component initialized")

	return Provides{
		Component: impl,
	}, nil
}

// Run delegates to the embedded Handler's Run method
func (h *handlerImpl) Run(ctx context.Context) {
	if h.handler != nil {
		if h.log != nil {
			h.log.Info("Starting cluster checks handler")
		}
		h.handler.Run(ctx)
	}
}

// RejectOrForwardLeaderQuery delegates to the handler
func (h *handlerImpl) RejectOrForwardLeaderQuery(rw http.ResponseWriter, req *http.Request) bool {
	if h.handler == nil {
		return false
	}
	return h.handler.RejectOrForwardLeaderQuery(rw, req)
}

// GetState delegates to the handler
func (h *handlerImpl) GetState() (types.StateResponse, error) {
	if h.handler == nil {
		return types.StateResponse{}, errors.New("cluster checks handler not initialized")
	}
	return h.handler.GetState()
}

// GetConfigs delegates to the handler
func (h *handlerImpl) GetConfigs(identifier string) (types.ConfigResponse, error) {
	if h.handler == nil {
		return types.ConfigResponse{}, errors.New("cluster checks handler not initialized")
	}
	return h.handler.GetConfigs(identifier)
}

// PostStatus delegates to the handler
func (h *handlerImpl) PostStatus(identifier, clientIP string, status types.NodeStatus) types.StatusResponse {
	if h.handler == nil {
		return types.StatusResponse{}
	}
	return h.handler.PostStatus(identifier, clientIP, status)
}

// GetEndpointsConfigs delegates to the handler
func (h *handlerImpl) GetEndpointsConfigs(nodeName string) (types.ConfigResponse, error) {
	if h.handler == nil {
		return types.ConfigResponse{}, errors.New("cluster checks handler not initialized")
	}
	return h.handler.GetEndpointsConfigs(nodeName)
}

// GetAllEndpointsCheckConfigs delegates to the handler
func (h *handlerImpl) GetAllEndpointsCheckConfigs() (types.ConfigResponse, error) {
	if h.handler == nil {
		return types.ConfigResponse{}, errors.New("cluster checks handler not initialized")
	}
	return h.handler.GetAllEndpointsCheckConfigs()
}

// RebalanceClusterChecks delegates to the handler
func (h *handlerImpl) RebalanceClusterChecks(force bool) ([]types.RebalanceResponse, error) {
	if h.handler == nil {
		return nil, errors.New("cluster checks handler not initialized")
	}
	return h.handler.RebalanceClusterChecks(force)
}

// IsolateCheck delegates to the handler
func (h *handlerImpl) IsolateCheck(isolateCheckID string) types.IsolateResponse {
	if h.handler == nil {
		return types.IsolateResponse{}
	}
	return h.handler.IsolateCheck(isolateCheckID)
}

// GetStats delegates to the handler
func (h *handlerImpl) GetStats() (*types.Stats, error) {
	return h.handler.GetStats()
}

// GetNodeTypeCounts delegates to the handler
func (h *handlerImpl) GetNodeTypeCounts() (clcRunnerCount, nodeAgentCount int, err error) {
	return h.handler.GetNodeTypeCounts()
}
