// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build clusterchecks

// Package clusterchecksimpl implements the clusterchecks component interface.
package clusterchecksimpl

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/integration"
	"github.com/DataDog/datadog-agent/comp/core/config"
	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	clusterchecksmetadata "github.com/DataDog/datadog-agent/comp/metadata/clusterchecks/def"
	"github.com/DataDog/datadog-agent/comp/metadata/internal/util"
	"github.com/DataDog/datadog-agent/comp/metadata/runner/runnerimpl"
	clusterchecksHandler "github.com/DataDog/datadog-agent/pkg/clusteragent/clusterchecks"
	checkid "github.com/DataDog/datadog-agent/pkg/collector/check/id"
	"github.com/DataDog/datadog-agent/pkg/serializer"
	"github.com/DataDog/datadog-agent/pkg/serializer/marshaler"
	"github.com/DataDog/datadog-agent/pkg/util/flavor"

	"github.com/DataDog/datadog-agent/pkg/util/kubernetes/clustername"
	"github.com/DataDog/datadog-agent/pkg/util/uuid"
)

type metadata map[string]interface{}

// Payload handles the JSON unmarshalling of the cluster checks metadata payload
type Payload struct {
	// Cluster identification (required)
	Clustername string `json:"clustername"`
	ClusterID   string `json:"cluster_id"`

	// Metadata timestamp
	Timestamp int64 `json:"timestamp"`

	// Cluster check metadata (tailored for cluster checks)
	ClusterCheckMetadata map[string][]metadata `json:"clustercheck_metadata"`

	// Unique identifier for this payload
	UUID string `json:"uuid"`
}

// MarshalJSON serialization a Payload to JSON
func (p *Payload) MarshalJSON() ([]byte, error) {
	type PayloadAlias Payload
	return json.Marshal((*PayloadAlias)(p))
}

// SplitPayload implements marshaler.AbstractMarshaler#SplitPayload.
//
// In this case, the payload can't be split any further.
func (p *Payload) SplitPayload(_ int) ([]marshaler.AbstractMarshaler, error) {
	return nil, fmt.Errorf("could not split cluster checks payload any more, payload is too big for intake")
}

type clusterChecksImpl struct {
	util.InventoryPayload

	m sync.RWMutex

	log  log.Component
	conf config.Component

	// Cluster identification
	clustername string
	clusterID   string

	// Cluster checks handler
	clusterHandler interface{}
}

// Requires defines the dependencies for the clusterchecks metadata component
type Requires struct {
	Log        log.Component
	Conf       config.Component
	Serializer serializer.MetricSerializer
}

// Provides defines the output of the clusterchecks metadata component
type Provides struct {
	Comp     clusterchecksmetadata.Component
	Provider runnerimpl.Provider
}

// NewComponent creates a new clusterchecks component
func NewComponent(deps Requires) Provides {
	// Get cluster identification
	clusterName := clustername.GetClusterName(context.TODO(), "")
	clusterID, err := clustername.GetClusterID()
	if err != nil {
		clusterID = "" // Handle error gracefully like clusteragent
	}

	cc := &clusterChecksImpl{
		log:         deps.Log,
		conf:        deps.Conf,
		clustername: clusterName,
		clusterID:   clusterID,
	}

	// Only initialize inventory payload for cluster agent
	if flavor.GetFlavor() == flavor.ClusterAgent {
		cc.InventoryPayload = util.CreateInventoryPayload(
			deps.Conf,
			deps.Log,
			deps.Serializer,
			cc.getPayloadAsMarshaler,
			"cluster-checks-metadata.json",
		)
	} else {
		// For non-cluster agents, create a no-op payload
		cc.InventoryPayload = util.CreateInventoryPayload(
			deps.Conf,
			deps.Log,
			nil,
			cc.getPayloadAsMarshaler,
			"cluster-checks-metadata.json",
		)
	}

	return Provides{
		Comp:     cc,
		Provider: cc.MetadataProvider(),
	}
}

// getAsJSON returns the cluster checks metadata payload as JSON (delegating to InventoryPayload)
func (cc *clusterChecksImpl) getAsJSON() ([]byte, error) {
	return cc.InventoryPayload.GetAsJSON()
}

// WritePayloadAsJSON writes the cluster checks payload as JSON to HTTP response
func (cc *clusterChecksImpl) WritePayloadAsJSON(w http.ResponseWriter, _ *http.Request) {
	jsonPayload, err := cc.getAsJSON()
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(`{"error": "Unable to marshal cluster checks metadata payload"}`))
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.Write(jsonPayload)
}

// SetClusterHandler sets the cluster checks handler for collecting cluster check metadata
func (cc *clusterChecksImpl) SetClusterHandler(handler interface{}) {
	cc.m.Lock()
	defer cc.m.Unlock()

	// Type assert to clusterchecks.Handler (we know this is always the correct type)
	if clusterHandler, ok := handler.(*clusterchecksHandler.Handler); ok {
		cc.clusterHandler = clusterHandler
		cc.log.Debug("Cluster checks handler set successfully for clusterchecks metadata")
	} else {
		cc.log.Warn("Invalid cluster checks handler type provided to clusterchecks metadata")
	}
}

// getPayload creates the cluster checks metadata payload
func (cc *clusterChecksImpl) getPayload() *Payload {
	// Check if cluster checks configuration is enabled
	if !cc.conf.GetBool("inventories_checks_configuration_enabled") {
		cc.log.Debug("Cluster checks configuration disabled, skipping clusterchecks payload generation")
		return nil
	}

	cc.m.RLock()
	defer cc.m.RUnlock()

	if cc.clusterHandler == nil {
		cc.log.Debug("Cluster checks handler not available, skipping clusterchecks payload generation")
		return nil
	}

	// Only generate payload from leader
	if !cc.isLeader() {
		cc.log.Debug("Not the leader cluster agent, skipping clusterchecks payload generation")
		return nil
	}

	payload := &Payload{
		Clustername:          cc.clustername,
		ClusterID:            cc.clusterID,
		Timestamp:            time.Now().UnixNano(),
		ClusterCheckMetadata: make(map[string][]metadata),
		UUID:                 uuid.GetUUID(),
	}

	// Collect cluster check metadata
	cc.collectClusterCheckMetadata(payload)

	cc.log.Debugf("Generated cluster checks metadata payload for cluster %s", cc.clustername)
	return payload
}

// getPayloadAsMarshaler returns the payload as a marshaler for the inventory system
func (cc *clusterChecksImpl) getPayloadAsMarshaler() marshaler.JSONMarshaler {
	return cc.getPayload()
}

// MetadataProvider returns the metadata provider for cluster checks (delegating to InventoryPayload)
func (cc *clusterChecksImpl) MetadataProvider() runnerimpl.Provider {
	return cc.InventoryPayload.MetadataProvider()
}

// isLeader checks if the cluster agent is the leader using GetState
// Assumes the caller already holds a read lock on cc.m
func (cc *clusterChecksImpl) isLeader() bool {
	if cc.clusterHandler == nil {
		return false
	}

	// Type assert to the actual handler type
	clusterHandler := cc.clusterHandler.(*clusterchecksHandler.Handler)
	state, err := clusterHandler.GetState()
	if err != nil {
		return false
	}

	// If NotRunning is empty, it means it's the leader
	return state.NotRunning == ""
}

// collectClusterCheckMetadata populates the payload with cluster check metadata
func (cc *clusterChecksImpl) collectClusterCheckMetadata(payload *Payload) {
	configs, err := cc.getClusterCheckConfigs()
	if err != nil {
		cc.log.Debugf("Error collecting cluster check configs: %s", err)
		return
	}

	for _, config := range configs {
		checkName := config.Name
		if checkName == "" {
			continue
		}

		checkMetadata := metadata{
			"config.hash":     checkid.BuildID(checkName, config.IntDigest(), config.Instances[0], config.InitConfig),
			"config.provider": config.Provider,
			"config.source":   config.Source,
			"init_config":     string(config.InitConfig),
		}

		// Handle instances
		if len(config.Instances) > 0 {
			checkMetadata["instance_config"] = string(config.Instances[0])
		}

		payload.ClusterCheckMetadata[checkName] = append(payload.ClusterCheckMetadata[checkName], checkMetadata)
	}
}

// getClusterCheckConfigs retrieves cluster check configurations from the stored handler
func (cc *clusterChecksImpl) getClusterCheckConfigs() ([]integration.Config, error) {
	if cc.clusterHandler == nil {
		return nil, fmt.Errorf("cluster checks handler not set")
	}

	// Only collect cluster checks from leader
	if !cc.isLeader() {
		return nil, fmt.Errorf("cluster checks only available on leader cluster agent")
	}

	// Type assert to get the actual handler to call GetAllClusterCheckConfigs
	handler := cc.clusterHandler.(*clusterchecksHandler.Handler)
	return handler.GetAllClusterCheckConfigs()
}
