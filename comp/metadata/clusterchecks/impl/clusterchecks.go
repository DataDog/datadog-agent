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

	clustercheckhandler "github.com/DataDog/datadog-agent/comp/core/clusterchecks/def"
	"github.com/DataDog/datadog-agent/comp/core/config"
	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	clusterchecksmetadata "github.com/DataDog/datadog-agent/comp/metadata/clusterchecks/def"
	"github.com/DataDog/datadog-agent/comp/metadata/internal/util"
	"github.com/DataDog/datadog-agent/comp/metadata/runner/runnerimpl"
	checkid "github.com/DataDog/datadog-agent/pkg/collector/check/id"
	"github.com/DataDog/datadog-agent/pkg/serializer"
	"github.com/DataDog/datadog-agent/pkg/serializer/marshaler"
	"github.com/DataDog/datadog-agent/pkg/util/option"

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

	// Cluster check status information
	ClusterCheckStatus map[string]interface{} `json:"clustercheck_status,omitempty"`

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

	// Cluster checks handler component
	clusterHandler option.Option[clustercheckhandler.Component]
}

// Requires defines the dependencies for the clusterchecks metadata component
type Requires struct {
	Log            log.Component
	Conf           config.Component
	Serializer     serializer.MetricSerializer
	ClusterHandler option.Option[clustercheckhandler.Component]
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
		log:            deps.Log,
		conf:           deps.Conf,
		clustername:    clusterName,
		clusterID:      clusterID,
		clusterHandler: deps.ClusterHandler,
	}

	// Initialize inventory payload - we're always in cluster agent when this is compiled
	cc.InventoryPayload = util.CreateInventoryPayload(
		deps.Conf,
		deps.Log,
		deps.Serializer,
		cc.getPayloadAsMarshaler,
		"cluster-checks-metadata.json",
	)

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

// getPayload creates the cluster checks metadata payload
func (cc *clusterChecksImpl) getPayload() *Payload {
	// Check if cluster checks configuration is enabled
	if !cc.conf.GetBool("inventories_checks_configuration_enabled") {
		cc.log.Debug("Cluster checks configuration disabled, skipping clusterchecks payload generation")
		return nil
	}

	cc.m.RLock()
	defer cc.m.RUnlock()

	handler, ok := cc.clusterHandler.Get()
	if !ok {
		cc.log.Debug("Cluster checks handler not available, skipping clusterchecks payload generation")
		return nil
	}

	// Only generate payload from leader
	if !cc.isLeader(handler) {
		cc.log.Debug("Not the leader cluster agent, skipping clusterchecks payload generation")
		return nil
	}

	payload := &Payload{
		Clustername:          cc.clustername,
		ClusterID:            cc.clusterID,
		Timestamp:            time.Now().UnixNano(),
		ClusterCheckMetadata: make(map[string][]metadata),
		ClusterCheckStatus:   make(map[string]interface{}),
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

// isLeader checks if the cluster agent is the leader using GetState.
// The handler's GetState() returns:
// - NotRunning == "" when state is leader
// - NotRunning == "currently follower" when state is follower
// - NotRunning == "Startup in progress" when state is unknown
// Assumes the caller already holds a read lock on cc.m
func (cc *clusterChecksImpl) isLeader(handler clustercheckhandler.Component) bool {
	state, err := handler.GetState()
	if err != nil {
		return false
	}

	// NotRunning is empty only when the handler is in leader state
	return state.NotRunning == ""
}

// collectClusterCheckMetadata populates the payload with cluster check metadata
func (cc *clusterChecksImpl) collectClusterCheckMetadata(payload *Payload) {
	handler, ok := cc.clusterHandler.Get()
	if !ok {
		cc.log.Debugf("Cluster checks handler not available")
		return
	}

	// Get full state with dispatch information
	state, err := handler.GetState()
	if err != nil {
		cc.log.Debugf("Error getting cluster check state: %s", err)
		return
	}

	// Add status information to payload
	payload.ClusterCheckStatus["warmup"] = state.Warmup
	payload.ClusterCheckStatus["dangling_count"] = len(state.Dangling)
	payload.ClusterCheckStatus["node_count"] = len(state.Nodes)

	// Process configs from all nodes
	for _, node := range state.Nodes {
		for _, config := range node.Configs {
			checkName := config.Name
			if checkName == "" {
				continue
			}

			checkMetadata := metadata{
				"config.hash":     checkid.BuildID(checkName, config.IntDigest(), config.Instances[0], config.InitConfig),
				"config.provider": config.Provider,
				"config.source":   config.Source,
				"init_config":     string(config.InitConfig),
				"node_name":       node.Name,
				"status":          "dispatched",
				"errors":          "", // Empty for now, ready for future error tracking
			}

			// Handle instances
			if len(config.Instances) > 0 {
				checkMetadata["instance_config"] = string(config.Instances[0])
			}

			payload.ClusterCheckMetadata[checkName] = append(payload.ClusterCheckMetadata[checkName], checkMetadata)
		}
	}

	// Also include dangling configs
	for _, config := range state.Dangling {
		checkName := config.Name
		if checkName == "" {
			continue
		}

		checkMetadata := metadata{
			"config.hash":     checkid.BuildID(checkName, config.IntDigest(), config.Instances[0], config.InitConfig),
			"config.provider": config.Provider,
			"config.source":   config.Source,
			"init_config":     string(config.InitConfig),
			"status":          "dangling",
			"errors":          "Check not assigned to any node",
		}

		// Handle instances
		if len(config.Instances) > 0 {
			checkMetadata["instance_config"] = string(config.Instances[0])
		}

		payload.ClusterCheckMetadata[checkName] = append(payload.ClusterCheckMetadata[checkName], checkMetadata)
	}
}
