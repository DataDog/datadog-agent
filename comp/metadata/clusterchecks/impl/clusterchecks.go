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

	"github.com/DataDog/datadog-agent/comp/core/config"
	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	clusterchecksmetadata "github.com/DataDog/datadog-agent/comp/metadata/clusterchecks/def"
	"github.com/DataDog/datadog-agent/comp/metadata/internal/util"
	"github.com/DataDog/datadog-agent/comp/metadata/runner/runnerimpl"
	pkgclusterchecks "github.com/DataDog/datadog-agent/pkg/clusteragent/clusterchecks"
	checkid "github.com/DataDog/datadog-agent/pkg/collector/check/id"
	"github.com/DataDog/datadog-agent/pkg/serializer"
	"github.com/DataDog/datadog-agent/pkg/serializer/marshaler"

	"github.com/DataDog/datadog-agent/pkg/util/kubernetes/clustername"
	"github.com/DataDog/datadog-agent/pkg/util/uuid"
)

type metadata map[string]interface{}

// State constants for cluster checks handler
const (
	// StateLeader indicates the cluster agent is the leader (NotRunning is empty)
	StateLeader = ""
	// StateFollower indicates the cluster agent is a follower
	StateFollower = "currently follower"
	// StateNotReady indicates the cluster checks handler is not ready (startup in progress)
	StateNotReady = "Startup in progress"
)

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

	// Cluster checks handler
	clusterHandler *pkgclusterchecks.Handler
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
		deps.Log.Debugf("Error retrieving cluster ID: %v", err)
		clusterID = "" // Handle error gracefully like clusteragent
	}

	cc := &clusterChecksImpl{
		log:            deps.Log,
		conf:           deps.Conf,
		clustername:    clusterName,
		clusterID:      clusterID,
		clusterHandler: nil, // Will be set later via SetClusterHandler
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
		// Check if it's because the feature is disabled
		if err.Error() == "inventory metadata is disabled" {
			w.WriteHeader(http.StatusServiceUnavailable)
			w.Write([]byte(`{"error": "cluster checks metadata is disabled"}`))
			return
		}
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(`{"error": "Unable to marshal cluster checks metadata payload"}`))
		return
	}

	// Check if payload is null (no data available)
	if string(jsonPayload) == "null" || string(jsonPayload) == "null\n" {
		w.WriteHeader(http.StatusNoContent)
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

	if cc.clusterHandler == nil {
		cc.log.Debug("Cluster checks handler not available, skipping clusterchecks payload generation")
		return nil
	}

	// Only generate payload from leader
	if !cc.isLeader(cc.clusterHandler) {
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

// SetClusterHandler sets the cluster handler for the metadata component
func (cc *clusterChecksImpl) SetClusterHandler(handler interface{}) {
	cc.m.Lock()
	defer cc.m.Unlock()

	if h, ok := handler.(*pkgclusterchecks.Handler); ok {
		cc.clusterHandler = h
		cc.log.Debug("Cluster handler set for metadata component")
	} else {
		cc.log.Warn("Failed to set cluster handler: invalid type")
	}
}

// isLeader checks if the cluster agent is the leader using GetState.
// Returns true only when the cluster agent is the leader (NotRunning == StateLeader).
// Assumes the caller already holds a read lock on cc.m
func (cc *clusterChecksImpl) isLeader(handler *pkgclusterchecks.Handler) bool {
	state, err := handler.GetState()
	if err != nil {
		return false
	}

	// Check if we're the leader (NotRunning is empty)
	return state.NotRunning == StateLeader
}

// collectClusterCheckMetadata populates the payload with cluster check metadata
func (cc *clusterChecksImpl) collectClusterCheckMetadata(payload *Payload) {
	if cc.clusterHandler == nil {
		cc.log.Debugf("Cluster checks handler not available")
		return
	}

	// Get full state with dispatch information
	state, err := cc.clusterHandler.GetState()
	if err != nil {
		cc.log.Debugf("Error getting cluster check state: %s", err)
		return
	}

	// Add status information to payload
	// dangling_count: how many checks are not assigned to any node
	payload.ClusterCheckStatus["dangling_count"] = len(state.Dangling)
	// node_count: how many nodes are available for check distribution
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
				"status":          "DISPATCHED",
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
			"status":          "DANGLING",
			"errors":          "Check not assigned to any node",
		}

		// Handle instances
		if len(config.Instances) > 0 {
			checkMetadata["instance_config"] = string(config.Instances[0])
		}

		payload.ClusterCheckMetadata[checkName] = append(payload.ClusterCheckMetadata[checkName], checkMetadata)
	}
}
