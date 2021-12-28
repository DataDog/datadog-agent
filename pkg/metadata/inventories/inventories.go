// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package inventories

import (
	"context"
	"encoding/json"
	"reflect"
	"strings"
	"sync"
	"time"

	"github.com/DataDog/datadog-agent/pkg/autodiscovery/integration"
	"github.com/DataDog/datadog-agent/pkg/collector/check"
	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/util/flavor"
	"github.com/DataDog/datadog-agent/pkg/util/scrubber"
	"github.com/DataDog/datadog-agent/pkg/version"
)

type schedulerInterface interface {
	TriggerAndResetCollectorTimer(name string, delay time.Duration)
}

// AutoConfigInterface is an interface for the MapOverLoadedConfigs method of autodiscovery
type AutoConfigInterface interface {
	MapOverLoadedConfigs(func(map[string]integration.Config))
}

// CollectorInterface is an interface for the GetAllInstanceIDs method of the collector
type CollectorInterface interface {
	GetAllInstanceIDs(checkName string) []check.ID
}

type checkMetadataCacheEntry struct {
	LastUpdated           time.Time
	CheckInstanceMetadata CheckInstanceMetadata
}

var (
	checkMetadata      = make(map[string]*checkMetadataCacheEntry) // by check ID
	checkMetadataMutex = &sync.Mutex{}
	agentMetadata      = make(AgentMetadata)
	agentMetadataMutex = &sync.Mutex{}

	agentStartupTime = timeNow()

	lastPayload         *Payload
	lastGetPayload      = timeNow()
	lastGetPayloadMutex = &sync.Mutex{}

	metadataUpdatedC = make(chan interface{}, 1)
)

var (
	// For testing purposes
	timeNow   = time.Now
	timeSince = time.Since
)

// AgentMetadataName is an enum type containing all defined keys for
// SetAgentMetadata.
type AgentMetadataName string

// Constants for the metadata names; these are defined in
// pkg/metadata/inventories/README.md and any additions should
// be updated there as well.
const (
	AgentCloudProvider                 AgentMetadataName = "cloud_provider"
	AgentHostnameSource                AgentMetadataName = "hostname_source"
	AgentVersion                       AgentMetadataName = "agent_version"
	AgentFlavor                        AgentMetadataName = "flavor"
	AgentConfigAPMDDURL                AgentMetadataName = "config_apm_dd_url"
	AgentConfigDDURL                   AgentMetadataName = "config_dd_url"
	AgentConfigSite                    AgentMetadataName = "config_site"
	AgentConfigLogsDDURL               AgentMetadataName = "config_logs_dd_url"
	AgentConfigLogsSocks5ProxyAddress  AgentMetadataName = "config_logs_socks5_proxy_address"
	AgentConfigNoProxy                 AgentMetadataName = "config_no_proxy"
	AgentConfigProcessDDURL            AgentMetadataName = "config_process_dd_url"
	AgentConfigProxyHTTP               AgentMetadataName = "config_proxy_http"
	AgentConfigProxyHTTPS              AgentMetadataName = "config_proxy_https"
	AgentInstallMethodInstallerVersion AgentMetadataName = "install_method_installer_version"
	AgentInstallMethodTool             AgentMetadataName = "install_method_tool"
	AgentInstallMethodToolVersion      AgentMetadataName = "install_method_tool_version"
	AgentLogsTransport                 AgentMetadataName = "logs_transport"
	AgentCWSEnabled                    AgentMetadataName = "feature_cws_enabled"
	AgentProcessEnabled                AgentMetadataName = "feature_process_enabled"
	AgentNetworksEnabled               AgentMetadataName = "feature_networks_enabled"
	AgentLogsEnabled                   AgentMetadataName = "feature_logs_enabled"
	AgentCSPMEnabled                   AgentMetadataName = "feature_cspm_enabled"
	AgentAPMEnabled                    AgentMetadataName = "feature_apm_enabled"
)

// SetAgentMetadata updates the agent metadata value in the cache
func SetAgentMetadata(name AgentMetadataName, value interface{}) {
	agentMetadataMutex.Lock()
	defer agentMetadataMutex.Unlock()

	if !reflect.DeepEqual(agentMetadata[string(name)], value) {
		agentMetadata[string(name)] = value

		select {
		case metadataUpdatedC <- nil:
		default: // To make sure this call is not blocking
		}
	}
}

// SetCheckMetadata updates a metadata value for one check instance in the cache.
func SetCheckMetadata(checkID, key string, value interface{}) {
	checkMetadataMutex.Lock()
	defer checkMetadataMutex.Unlock()

	entry, found := checkMetadata[checkID]
	if !found {
		entry = &checkMetadataCacheEntry{
			CheckInstanceMetadata: make(CheckInstanceMetadata),
		}
		checkMetadata[checkID] = entry
	}

	if !reflect.DeepEqual(entry.CheckInstanceMetadata[key], value) {
		entry.LastUpdated = timeNow()
		entry.CheckInstanceMetadata[key] = value

		select {
		case metadataUpdatedC <- nil:
		default: // To make sure this call is not blocking
		}
	}
}

func createCheckInstanceMetadata(checkID, configProvider string) *CheckInstanceMetadata {
	const transientFields = 3

	var checkInstanceMetadata CheckInstanceMetadata
	var lastUpdated time.Time

	if entry, found := checkMetadata[checkID]; found {
		checkInstanceMetadata = make(CheckInstanceMetadata, len(entry.CheckInstanceMetadata)+transientFields)
		for k, v := range entry.CheckInstanceMetadata {
			checkInstanceMetadata[k] = v
		}
		lastUpdated = entry.LastUpdated
	} else {
		checkInstanceMetadata = make(CheckInstanceMetadata, transientFields)
		lastUpdated = agentStartupTime
	}

	checkInstanceMetadata["last_updated"] = lastUpdated.UnixNano()
	checkInstanceMetadata["config.hash"] = checkID
	checkInstanceMetadata["config.provider"] = configProvider

	return &checkInstanceMetadata
}

// CreatePayload fills and returns the inventory metadata payload
func CreatePayload(ctx context.Context, hostname string, ac AutoConfigInterface, coll CollectorInterface) *Payload {
	checkMetadataMutex.Lock()
	defer checkMetadataMutex.Unlock()

	// Collect check metadata for the payload
	payloadCheckMeta := make(CheckMetadata)

	foundInCollector := map[string]struct{}{}
	if ac != nil {
		ac.MapOverLoadedConfigs(func(loadedConfigs map[string]integration.Config) {
			for _, config := range loadedConfigs {
				payloadCheckMeta[config.Name] = make([]*CheckInstanceMetadata, 0)
				instanceIDs := coll.GetAllInstanceIDs(config.Name)
				for _, id := range instanceIDs {
					checkInstanceMetadata := createCheckInstanceMetadata(string(id), config.Provider)
					payloadCheckMeta[config.Name] = append(payloadCheckMeta[config.Name], checkInstanceMetadata)
					foundInCollector[string(id)] = struct{}{}
				}
			}
		})
	}
	// if metadata were added for a check not in the collector we still need
	// to add them to the payloadCheckMeta (this happens when using the
	// 'check' command)
	for id := range checkMetadata {
		if _, found := foundInCollector[id]; !found {
			// id should be "check_name:check_hash"
			parts := strings.SplitN(id, ":", 2)
			payloadCheckMeta[parts[0]] = append(payloadCheckMeta[parts[0]], createCheckInstanceMetadata(id, ""))
		}
	}

	agentMetadataMutex.Lock()
	defer agentMetadataMutex.Unlock()

	// Create a static copy of agentMetadata for the payload
	payloadAgentMeta := make(AgentMetadata)
	for k, v := range agentMetadata {
		payloadAgentMeta[k] = v
	}

	return &Payload{
		Hostname:      hostname,
		Timestamp:     timeNow().UnixNano(),
		CheckMetadata: &payloadCheckMeta,
		AgentMetadata: &payloadAgentMeta,
	}
}

// GetPayload returns a new inventory metadata payload and updates lastGetPayload
func GetPayload(ctx context.Context, hostname string, ac AutoConfigInterface, coll CollectorInterface) *Payload {
	lastGetPayloadMutex.Lock()
	defer lastGetPayloadMutex.Unlock()
	lastGetPayload = timeNow()

	lastPayload = CreatePayload(ctx, hostname, ac, coll)
	return lastPayload
}

// GetLastPayload returns the last payload created by the inventories metadata collector as JSON.
func GetLastPayload() ([]byte, error) {
	lastGetPayloadMutex.Lock()
	defer lastGetPayloadMutex.Unlock()

	if lastPayload == nil {
		return []byte("no inventories metadata payload was created yet"), nil
	}
	return json.MarshalIndent(lastPayload, "", "    ")
}

// StartMetadataUpdatedGoroutine starts a routine that listens to the metadataUpdatedC
// signal to run the collector out of its regular interval.
func StartMetadataUpdatedGoroutine(sc schedulerInterface, minSendInterval time.Duration) error {
	go func() {
		for {
			<-metadataUpdatedC
			lastGetPayloadMutex.Lock()
			delay := minSendInterval - timeSince(lastGetPayload)
			if delay < 0 {
				delay = 0
			}
			sc.TriggerAndResetCollectorTimer("inventories", delay)
			lastGetPayloadMutex.Unlock()
		}
	}()
	return nil
}

// InitializeData inits the inventories payload with basic and static information (agent version, flavor name, ...)
func InitializeData() {
	SetAgentMetadata(AgentVersion, version.AgentVersion)
	SetAgentMetadata(AgentFlavor, flavor.GetFlavor())

	initializeConfig(config.Datadog)
}

func initializeConfig(cfg config.Config) {
	clean := func(s string) string {
		// Errors come from internal use of a Reader interface.  Since we are
		// reading from a buffer, no errors are possible.
		cleanBytes, _ := scrubber.ScrubBytes([]byte(s))
		return string(cleanBytes)
	}

	cleanSlice := func(ss []string) []string {
		rv := make([]string, len(ss))
		for i, s := range ss {
			rv[i] = clean(s)
		}
		return rv
	}

	SetAgentMetadata(AgentConfigAPMDDURL, clean(cfg.GetString("apm_config.apm_dd_url")))
	SetAgentMetadata(AgentConfigDDURL, clean(cfg.GetString("dd_url")))
	SetAgentMetadata(AgentConfigSite, clean(cfg.GetString("dd_site")))
	SetAgentMetadata(AgentConfigLogsDDURL, clean(cfg.GetString("logs_config.logs_dd_url")))
	SetAgentMetadata(AgentConfigLogsSocks5ProxyAddress, clean(cfg.GetString("logs_config.socks5_proxy_address")))
	SetAgentMetadata(AgentConfigNoProxy, cleanSlice(cfg.GetStringSlice("proxy.no_proxy")))
	SetAgentMetadata(AgentConfigProcessDDURL, clean(cfg.GetString("process_config.process_dd_url")))
	SetAgentMetadata(AgentConfigProxyHTTP, clean(cfg.GetString("proxy.http")))
	SetAgentMetadata(AgentConfigProxyHTTPS, clean(cfg.GetString("proxy.https")))
	SetAgentMetadata(AgentCWSEnabled, config.Datadog.GetBool("runtime_security_config.enabled"))
	SetAgentMetadata(AgentProcessEnabled, config.Datadog.GetBool("process_config.enabled"))
	SetAgentMetadata(AgentNetworksEnabled, config.Datadog.GetBool("network_config.enabled"))
	SetAgentMetadata(AgentLogsEnabled, config.Datadog.GetBool("logs_enabled"))
	SetAgentMetadata(AgentCSPMEnabled, config.Datadog.GetBool("compliance_config.enabled"))
	SetAgentMetadata(AgentAPMEnabled, config.Datadog.GetBool("apm_config.enabled"))
}
