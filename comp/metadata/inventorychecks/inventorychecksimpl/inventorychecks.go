// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package inventorychecksimpl implements the inventorychecks component interface.
package inventorychecksimpl

import (
	"context"
	"encoding/json"
	"expvar"
	"fmt"
	"maps"
	"net/http"
	"reflect"
	"sync"
	"time"

	"go.uber.org/fx"

	api "github.com/DataDog/datadog-agent/comp/api/api/def"
	"github.com/DataDog/datadog-agent/comp/collector/collector"

	"github.com/DataDog/datadog-agent/comp/core/config"
	flaretypes "github.com/DataDog/datadog-agent/comp/core/flare/types"
	"github.com/DataDog/datadog-agent/comp/core/hostname/hostnameinterface"
	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	logagent "github.com/DataDog/datadog-agent/comp/logs/agent"
	"github.com/DataDog/datadog-agent/comp/metadata/internal/util"
	"github.com/DataDog/datadog-agent/comp/metadata/inventorychecks"
	"github.com/DataDog/datadog-agent/comp/metadata/runner/runnerimpl"
	"github.com/DataDog/datadog-agent/pkg/collector/check"
	checkid "github.com/DataDog/datadog-agent/pkg/collector/check/id"
	"github.com/DataDog/datadog-agent/pkg/logs/sources"
	"github.com/DataDog/datadog-agent/pkg/serializer"
	"github.com/DataDog/datadog-agent/pkg/serializer/marshaler"
	"github.com/DataDog/datadog-agent/pkg/util/flavor"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
	httputils "github.com/DataDog/datadog-agent/pkg/util/http"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes/clustername"
	"github.com/DataDog/datadog-agent/pkg/util/option"
	"github.com/DataDog/datadog-agent/pkg/util/uuid"
)

// ClusterHandlerInterface is a common interface for cluster handler
// We use interface{} to allow different handler types across build tags
type ClusterHandlerInterface interface{}

// Module defines the fx options for this component.
func Module() fxutil.Module {
	return fxutil.Component(
		fx.Provide(newInventoryChecksProvider),
	)
}

type metadata map[string]interface{}
type checksMetadata map[string][]metadata

// Payload handles the JSON unmarshalling of the metadata payload
type Payload struct {
	// Detection field for easy backend routing
	IsClusterCheck bool `json:"is_cluster_check"`

	// Regular agent fields
	Hostname string `json:"hostname,omitempty"`

	// Cluster agent fields (mirrors clusteragent metadata)
	Clustername string `json:"clustername,omitempty"`
	ClusterID   string `json:"cluster_id,omitempty"`

	// Common fields
	Timestamp    int64                 `json:"timestamp"`
	Metadata     map[string][]metadata `json:"check_metadata"`
	LogsMetadata map[string][]metadata `json:"logs_metadata"`
	UUID         string                `json:"uuid"`
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
	return nil, fmt.Errorf("could not split inventories host payload any more, payload is too big for intake")
}

type instanceMetadata struct {
	LastUpdated time.Time
	instanceID  string
	metadata    metadata
}

type inventorychecksImpl struct {
	util.InventoryPayload

	m sync.Mutex
	// data is a map of instanceID to metadata
	data map[string]instanceMetadata

	log     log.Component
	conf    config.Component
	coll    option.Option[collector.Component]
	sources option.Option[*sources.LogSources]

	// Current approach
	hostname string

	// NEW: Add cluster detection
	isClusterAgent bool
	clustername    string
	clusterID      string
	clusterHandler ClusterHandlerInterface // Typed cluster checks handler
}

type dependencies struct {
	fx.In

	Log        log.Component
	Config     config.Component
	Serializer serializer.MetricSerializer
	Coll       option.Option[collector.Component]
	LogAgent   option.Option[logagent.Component] `optional:"true"`
	Hostname   hostnameinterface.Component
}

type provides struct {
	fx.Out

	Comp          inventorychecks.Component
	Provider      runnerimpl.Provider
	FlareProvider flaretypes.Provider
	Endpoint      api.AgentEndpointProvider
}

func newInventoryChecksProvider(deps dependencies) provides {
	// Detect if we're running on cluster agent
	isClusterAgent := flavor.GetFlavor() == flavor.ClusterAgent

	ic := &inventorychecksImpl{
		conf:           deps.Config,
		log:            deps.Log,
		coll:           deps.Coll,
		sources:        option.None[*sources.LogSources](),
		isClusterAgent: isClusterAgent,
		data:           map[string]instanceMetadata{},
	}

	if isClusterAgent {
		// Use cluster agent approach (mirror cluster agent metadata)
		hname, _ := deps.Hostname.Get(context.Background())
		ic.clustername = clustername.GetClusterName(context.Background(), hname)
		clusterID, err := getClusterID()
		if err != nil {
			ic.log.Warnf("Failed to get cluster ID for inventory checks: %v", err)
			ic.clusterID = ""
		} else {
			ic.clusterID = clusterID
		}
		ic.log.Infof("Inventorychecks running on cluster agent - cluster: %s, ID: %s", ic.clustername, ic.clusterID)
	} else {
		// Use regular agent approach
		hname, _ := deps.Hostname.Get(context.Background())
		ic.hostname = hname
		ic.log.Infof("Inventorychecks running on node agent - hostname: %s", ic.hostname)
	}
	ic.InventoryPayload = util.CreateInventoryPayload(deps.Config, deps.Log, deps.Serializer, ic.getPayloadWithConfigs, "checks.json")

	// We want to be notified when the collector add or removed a check.
	// TODO: (component) - This entire metadata provider should be part of the collector. Once the collector is a
	// component we can migrate it there and remove the entire logic to emit event from the collector.

	if coll, isSet := ic.coll.Get(); isSet {
		coll.AddEventReceiver(func(_ checkid.ID, _ collector.EventType) { ic.Refresh() })
	}

	// Only set up logs sources for regular agents, not cluster agents
	if !isClusterAgent {
		if logAgent, isSet := deps.LogAgent.Get(); isSet {
			ic.sources.Set(logAgent.GetSources())
		}
	}

	// Set the expvar callback to the current inventorycheck
	// This should be removed when migrated to collector component
	if icExpvar := expvar.Get("inventories"); icExpvar == nil {
		expvar.Publish("inventories", expvar.Func(func() interface{} {
			return ic.getPayload(false)
		}))
	}

	return provides{
		Comp:          ic,
		Provider:      ic.MetadataProvider(),
		FlareProvider: ic.FlareProvider(),
		Endpoint:      api.NewAgentEndpointProvider(ic.WritePayloadAsJSON, "/metadata/inventory-checks", "GET"),
	}
}

// Set sets a metadata value for one check instance
func (ic *inventorychecksImpl) Set(instanceID string, key string, value interface{}) {
	if !ic.Enabled || instanceID == "" {
		return
	}

	ic.log.Debugf("setting check metadata for check %s, '%s': '%s'", instanceID, key, value)

	ic.m.Lock()
	defer ic.m.Unlock()

	entry, found := ic.data[instanceID]
	if !found {
		entry = instanceMetadata{
			instanceID: instanceID,
			metadata:   map[string]interface{}{},
		}
		ic.data[instanceID] = entry
	}

	if !reflect.DeepEqual(entry.metadata[key], value) {
		entry.LastUpdated = time.Now()
		entry.metadata[key] = value

		ic.Refresh()
	}
}

func (ic *inventorychecksImpl) GetInstanceMetadata(instanceID string) map[string]interface{} {
	ic.m.Lock()
	defer ic.m.Unlock()

	res := map[string]interface{}{}
	if instance, found := ic.data[instanceID]; found {
		maps.Copy(res, instance.metadata)
	}
	return res
}

func (ic *inventorychecksImpl) getPayloadWithConfigs() marshaler.JSONMarshaler {
	return ic.getPayload(true)
}

func (ic *inventorychecksImpl) getPayload(withConfigs bool) marshaler.JSONMarshaler {
	ic.m.Lock()
	defer ic.m.Unlock()

	payloadData := make(checksMetadata)
	invChecksEnabled := ic.conf.GetBool("inventories_checks_configuration_enabled")
	withConfigs = withConfigs && invChecksEnabled

	// For cluster agents, only collect cluster checks (skip local checks)
	if !ic.isClusterAgent {
		if coll, isSet := ic.coll.Get(); isSet {
			foundInCollector := map[string]struct{}{}

			coll.MapOverChecks(func(checks []check.Info) {
				for _, c := range checks {
					cm := check.GetMetadata(c, withConfigs)

					if checkData, found := ic.data[string(c.ID())]; found {
						maps.Copy(cm, checkData.metadata)
					}

					checkName := c.String()
					payloadData[checkName] = append(payloadData[checkName], cm)

					instanceID := string(c.ID())
					foundInCollector[instanceID] = struct{}{}
				}
			})

			// if metadata were added for a check not in the collector we clear the cache. This can happen when a check
			// submit metadata after being unscheduled but before exiting its last run.
			for instanceID := range ic.data {
				if _, found := foundInCollector[instanceID]; !found {
					delete(ic.data, instanceID)
				}
			}
		}
	}

	// Collect cluster check metadata (only for cluster agents)
	if ic.isClusterAgent && invChecksEnabled {
		ic.collectClusterCheckMetadata(payloadData)
	}

	logsMetadata := make(map[string][]metadata)
	if sources, isSet := ic.sources.Get(); isSet && invChecksEnabled {
		if sources != nil {
			for _, logSource := range sources.GetSources() {
				if _, found := logsMetadata[logSource.Name]; !found {
					logsMetadata[logSource.Name] = []metadata{}
				}

				parsedJSON, err := logSource.Config.PublicJSON()
				if err != nil {
					ic.log.Debugf("could not parse log configuration for source metadata %s: %v", logSource.Name, err)
					continue
				}

				tags := logSource.Config.Tags
				if tags == nil {
					tags = []string{}
				}
				logsMetadata[logSource.Name] = append(logsMetadata[logSource.Name], metadata{
					"config": string(parsedJSON),
					"state": map[string]string{
						"error":  logSource.Status.GetError(),
						"status": logSource.Status.String(),
					},
					"service":                  logSource.Config.Service,
					"source":                   logSource.Config.Source,
					"integration_name":         logSource.Config.IntegrationName,
					"integration_source":       logSource.Config.IntegrationSource,
					"integration_source_index": logSource.Config.IntegrationSourceIndex,
					"tags":                     tags,
				})
			}
		}
	}

	jmxMetadata := ic.getJMXChecksMetadata()
	for checkName, checks := range jmxMetadata {
		if _, ok := payloadData[checkName]; !ok {
			payloadData[checkName] = []metadata{}
		}
		payloadData[checkName] = append(payloadData[checkName], checks...)
	}

	payload := &Payload{
		IsClusterCheck: ic.isClusterAgent,
		Timestamp:      time.Now().UnixNano(),
		Metadata:       payloadData,
		LogsMetadata:   logsMetadata,
		UUID:           uuid.GetUUID(),
	}

	// Populate fields based on agent type and cluster checks availability
	if ic.isClusterAgent {
		// Only generate cluster agent payload if cluster checks are actually available
		if ic.clusterHandler != nil {
			payload.Clustername = ic.clustername
			payload.ClusterID = ic.clusterID
			ic.log.Debugf("Generated cluster check inventory payload for cluster %s", ic.clustername)
		} else {
			// No cluster checks handler available - don't generate cluster payload
			ic.log.Debugf("Cluster checks handler not available, skipping inventory payload generation")
			return nil
		}
	} else {
		payload.Hostname = ic.hostname
		ic.log.Debugf("Generated node check inventory payload for host %s", ic.hostname)
	}

	return payload
}

func (ic *inventorychecksImpl) WritePayloadAsJSON(w http.ResponseWriter, _ *http.Request) {
	// Check if cluster checks are available for cluster agents
	if ic.isClusterAgent {
		if ic.clusterHandler == nil {
			httputils.SetJSONError(w, fmt.Errorf("cluster checks handler not available"), 503)
			return
		}

		// Check if this is the leader cluster agent
		if ic.clusterHandler != nil && !isLeader(ic.clusterHandler) {
			httputils.SetJSONError(w, fmt.Errorf("cluster checks only available on leader cluster agent (currently follower)"), 503)
			return
		}
	}

	// GetAsJSON already return scrubbed data
	scrubbed, err := ic.GetAsJSON()
	if err != nil {
		httputils.SetJSONError(w, err, 500)
		return
	}
	w.Write(scrubbed)
}

// SetClusterHandler sets the cluster checks handler for collecting cluster check metadata (cluster agent only)
func (ic *inventorychecksImpl) SetClusterHandler(handler interface{}) {
	// Type assert and validate the handler at setup time
	if clusterHandler, ok := getClusterHandler(handler); ok {
		ic.clusterHandler = clusterHandler
		ic.log.Debug("Cluster checks handler set successfully")
	} else {
		ic.log.Warn("Invalid cluster checks handler type provided")
	}
}

// getClusterID returns the cluster ID using the same method as cluster agent
func getClusterID() (string, error) {
	// Use clustername utility to get cluster ID, same as cluster agent
	return clustername.GetClusterID()
}

// collectClusterCheckMetadata collects metadata from cluster checks dispatched to CLC runners
func (ic *inventorychecksImpl) collectClusterCheckMetadata(payloadData map[string][]metadata) {
	ic.log.Infof("collectClusterCheckMetadata called for cluster agent")

	// Try to get cluster check configurations via helper function
	configs, err := ic.getClusterCheckConfigs()
	if err != nil {
		ic.log.Warnf("Failed to get cluster check configurations: %v", err)
		return
	}

	ic.log.Infof("Found %d cluster check configurations from handler", len(configs))

	// Debug: Log all configs to understand what we have
	for i, config := range configs {
		ic.log.Infof("Config[%d]: Name=%s, ClusterCheck=%t, Provider=%s, Source=%s",
			i, config.Name, config.ClusterCheck, config.Provider, config.Source)
	}

	clusterConfigs := configs // All configs from GetAllClusterCheckConfigs are cluster checks

	// Convert cluster check configs to metadata format
	for _, config := range clusterConfigs {
		checkName := config.Name

		// Convert instances to string representation
		var instanceConfig string
		if len(config.Instances) > 0 {
			instanceConfig = string(config.Instances[0]) // Use first instance for metadata
		}

		cm := metadata{
			"config.hash":     config.Digest(),
			"config.provider": config.Provider,
			"config.source":   config.Source,
			"init_config":     string(config.InitConfig),
			"instance_config": instanceConfig,
		}

		payloadData[checkName] = append(payloadData[checkName], cm)
		ic.log.Tracef("Added cluster check metadata for %s: %+v", checkName, cm)
	}
}
