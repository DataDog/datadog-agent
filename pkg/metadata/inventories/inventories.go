// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package inventories

import (
	"context"
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
	"sync"
	"time"

	"github.com/DataDog/datadog-agent/pkg/collector/check"
	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/util/flavor"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/datadog-agent/pkg/util/scrubber"
	"github.com/DataDog/datadog-agent/pkg/version"
)

type schedulerInterface interface {
	TriggerAndResetCollectorTimer(name string, delay time.Duration)
}

// CollectorInterface is an interface for the MapOverChecks method of the collector
type CollectorInterface interface {
	MapOverChecks(func([]check.Info))
}

type checkMetadataCacheEntry struct {
	LastUpdated           time.Time
	CheckInstanceMetadata CheckInstanceMetadata
}

var (
	checkMetadata = make(map[string]*checkMetadataCacheEntry) // by check ID
	agentMetadata = make(AgentMetadata)
	hostMetadata  = make(AgentMetadata)

	inventoryMutex = &sync.Mutex{}

	lastPayload    *Payload
	lastGetPayload = timeNow()

	metadataUpdatedC = make(chan interface{}, 1)

	// The inventory payload might be generated once per minute. We don't want to
	logCount  = 0
	logErrorf = log.Errorf
	logInfof  = log.Infof
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
	AgentOTLPEnabled                   AgentMetadataName = "feature_otlp_enabled"
	AgentProcessEnabled                AgentMetadataName = "feature_process_enabled"
	AgentProcessesContainerEnabled     AgentMetadataName = "feature_processes_container_enabled"
	AgentNetworksEnabled               AgentMetadataName = "feature_networks_enabled"
	AgentNetworksHTTPEnabled           AgentMetadataName = "feature_networks_http_enabled"
	AgentNetworksHTTPSEnabled          AgentMetadataName = "feature_networks_https_enabled"
	AgentNetworksGoTLSEnabled          AgentMetadataName = "feature_networks_gotls_enabled"
	AgentNetworksJavaTLSEnabled        AgentMetadataName = "feature_networks_javatls_enabled"
	AgentLogsEnabled                   AgentMetadataName = "feature_logs_enabled"
	AgentCSPMEnabled                   AgentMetadataName = "feature_cspm_enabled"
	AgentAPMEnabled                    AgentMetadataName = "feature_apm_enabled"

	// Those are reserved fields for the agentMetadata payload.
	agentProvidedConf AgentMetadataName = "provided_configuration"
	agentFullConf     AgentMetadataName = "full_configuration"

	// key for the host metadata cache. See host_metadata.go
	HostOSVersion AgentMetadataName = "os_version"
)

// Refresh signals that some data has been updated and a new payload should be sent (ex: when configuration is changed
// by the user, new checks starts, etc). This will trigger a new payload to be sent while still respecting
// 'inventories_min_interval'.
func Refresh() {
	select {
	case metadataUpdatedC <- nil:
	default: // To make sure this call is not blocking
	}
}

// SetAgentMetadata updates the agent metadata value in the cache
func SetAgentMetadata(name AgentMetadataName, value interface{}) {
	if !config.Datadog.GetBool("inventories_enabled") {
		return
	}

	inventoryMutex.Lock()
	defer inventoryMutex.Unlock()

	if !reflect.DeepEqual(agentMetadata[string(name)], value) {
		agentMetadata[string(name)] = value

		Refresh()
	}
}

// SetHostMetadata updates the host metadata value in the cache
func SetHostMetadata(name AgentMetadataName, value interface{}) {
	if !config.Datadog.GetBool("inventories_enabled") {
		return
	}

	inventoryMutex.Lock()
	defer inventoryMutex.Unlock()

	if !reflect.DeepEqual(hostMetadata[string(name)], value) {
		hostMetadata[string(name)] = value

		Refresh()
	}
}

// SetCheckMetadata updates a metadata value for one check instance in the cache.
func SetCheckMetadata(checkID, key string, value interface{}) {
	if checkID == "" || !config.Datadog.GetBool("inventories_enabled") {
		return
	}

	inventoryMutex.Lock()
	defer inventoryMutex.Unlock()

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

		Refresh()
	}
}

// RemoveCheckMetadata removes metadata for a check a trigger a new payload. This need to be called when a check is
// unscheduled.
func RemoveCheckMetadata(checkID string) {
	if !config.Datadog.GetBool("inventories_enabled") {
		return
	}

	inventoryMutex.Lock()
	defer inventoryMutex.Unlock()

	delete(checkMetadata, checkID)

	Refresh()
}

func createCheckInstanceMetadata(checkID, configProvider, initConfig, instanceConfig string, withConfigs bool) *CheckInstanceMetadata {
	checkInstanceMetadata := CheckInstanceMetadata{}

	if entry, found := checkMetadata[checkID]; found {
		for k, v := range entry.CheckInstanceMetadata {
			checkInstanceMetadata[k] = v
		}
	}

	checkInstanceMetadata["config.hash"] = checkID
	checkInstanceMetadata["config.provider"] = configProvider

	if withConfigs && config.Datadog.GetBool("inventories_checks_configuration_enabled") {
		if instanceScrubbed, err := scrubber.ScrubString(instanceConfig); err != nil {
			log.Errorf("Could not scrub instance configuration for check id %s: %s", checkID, err)
		} else {
			checkInstanceMetadata["instance_config"] = strings.TrimSpace(instanceScrubbed)
		}

		if initScrubbed, err := scrubber.ScrubString(initConfig); err != nil {
			log.Errorf("Could not scrub init configuration for check id %s: %s", checkID, err)
		} else {
			checkInstanceMetadata["init_config"] = strings.TrimSpace(initScrubbed)
		}
	}

	return &checkInstanceMetadata
}

// createPayload fills and returns the inventory metadata payload
func createPayload(ctx context.Context, hostname string, coll CollectorInterface, withConfigs bool) *Payload {
	// setLogLevel select the correct log level for the inventory payload currently being created. We send a new payload
	// every 1 to 10 min (depending on new metadata being registered). We don't want to log the same error again and again.
	// We log once every 12 times on normal log level and on debug the rest of the time. The metadata in this paylaod should
	// not change often, so with 1 paylaod every 10 minutes we would log once every 2h.
	if logCount%12 == 0 {
		logErrorf = log.Errorf
		logInfof = log.Infof
	} else {
		logErrorf = func(format string, params ...interface{}) error {
			err := fmt.Errorf(format, params...)
			log.Debugf(err.Error())
			return err
		}
		logInfof = log.Debugf
	}
	logCount++

	// Collect check metadata for the payload
	payloadCheckMeta := make(CheckMetadata)

	foundInCollector := map[string]struct{}{}

	if coll != nil {
		coll.MapOverChecks(func(checks []check.Info) {
			for _, c := range checks {
				checkName := c.String()
				cm := createCheckInstanceMetadata(
					string(c.ID()),
					strings.Split(c.ConfigSource(), ":")[0],
					c.InitConfig(),
					c.InstanceConfig(),
					withConfigs,
				)

				if _, found := payloadCheckMeta[checkName]; !found {
					payloadCheckMeta[checkName] = make([]*CheckInstanceMetadata, 0)
				}
				payloadCheckMeta[checkName] = append(payloadCheckMeta[checkName], cm)
				foundInCollector[string(c.ID())] = struct{}{}
			}
		})
	}

	// if metadata were added for a check not in the collector we clear the cache. This can happen when a check
	// submit metadata after being unscheduled but before exiting its last run.
	for id := range checkMetadata {
		if _, found := foundInCollector[id]; !found {
			delete(checkMetadata, id)
		}
	}

	// Create a static copy of agentMetadata for the payload
	payloadAgentMeta := make(AgentMetadata)
	for k, v := range agentMetadata {
		payloadAgentMeta[k] = v
	}

	if withConfigs {
		if fullConf, err := getFullAgentConfiguration(); err == nil {
			payloadAgentMeta[string(agentFullConf)] = fullConf
		}
		if providedConf, err := getProvidedAgentConfiguration(); err == nil {
			payloadAgentMeta[string(agentProvidedConf)] = providedConf
		}
	}

	return &Payload{
		Hostname:      hostname,
		Timestamp:     timeNow().UnixNano(),
		CheckMetadata: &payloadCheckMeta,
		AgentMetadata: &payloadAgentMeta,
		HostMetadata:  getHostMetadata(),
	}
}

// GetCheckMetadata returns metadata for a check instance
func GetCheckMetadata(c check.Check) *CheckInstanceMetadata {
	if !config.Datadog.GetBool("inventories_enabled") {
		return nil
	}

	inventoryMutex.Lock()
	defer inventoryMutex.Unlock()

	checkID := string(c.ID())
	if _, found := checkMetadata[checkID]; found {
		return createCheckInstanceMetadata(
			checkID,
			strings.Split(c.ConfigSource(), ":")[0],
			c.InitConfig(),
			c.InstanceConfig(),
			false,
		)
	}
	return nil
}

// GetPayload returns a new inventory metadata payload and updates lastGetPayload
func GetPayload(ctx context.Context, hostname string, coll CollectorInterface, withConfigs bool) *Payload {
	if !config.Datadog.GetBool("inventories_enabled") {
		return nil
	}

	inventoryMutex.Lock()
	defer inventoryMutex.Unlock()

	p := createPayload(ctx, hostname, coll, withConfigs)
	if withConfigs {
		lastGetPayload = timeNow()
		lastPayload = p
	}
	return p
}

// GetLastPayload returns the last payload created by the inventories metadata collector as JSON.
func GetLastPayload() ([]byte, error) {
	inventoryMutex.Lock()
	defer inventoryMutex.Unlock()

	if lastPayload == nil {
		return []byte("no inventories metadata payload was created yet"), nil
	}
	return json.MarshalIndent(lastPayload, "", "    ")
}

// StartMetadataUpdatedGoroutine starts a routine that listens to the metadataUpdatedC
// signal to run the collector out of its regular interval.
func StartMetadataUpdatedGoroutine(sc schedulerInterface, minSendInterval time.Duration) error {
	if !config.Datadog.GetBool("inventories_enabled") {
		return nil
	}

	go func() {
		for {
			<-metadataUpdatedC

			inventoryMutex.Lock()
			delay := minSendInterval - timeSince(lastGetPayload)
			inventoryMutex.Unlock()
			if delay < 0 {
				delay = 0
			}
			sc.TriggerAndResetCollectorTimer("inventories", delay)
		}
	}()
	return nil
}

// InitializeData inits the inventories payload with basic and static information (agent version, flavor name, ...)
func InitializeData() {
	if !config.Datadog.GetBool("inventories_enabled") {
		return
	}

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

	cfgSlice := func(name string) []string {
		if cfg.IsSet(name) {
			ss := cfg.GetStringSlice(name)
			rv := make([]string, len(ss))
			for i, s := range ss {
				rv[i] = clean(s)
			}
			return rv
		}
		return []string{}
	}

	SetAgentMetadata(AgentConfigAPMDDURL, clean(cfg.GetString("apm_config.apm_dd_url")))
	SetAgentMetadata(AgentConfigDDURL, clean(cfg.GetString("dd_url")))
	SetAgentMetadata(AgentConfigSite, clean(cfg.GetString("dd_site")))
	SetAgentMetadata(AgentConfigLogsDDURL, clean(cfg.GetString("logs_config.logs_dd_url")))
	SetAgentMetadata(AgentConfigLogsSocks5ProxyAddress, clean(cfg.GetString("logs_config.socks5_proxy_address")))
	SetAgentMetadata(AgentConfigNoProxy, cfgSlice("proxy.no_proxy"))
	SetAgentMetadata(AgentConfigProcessDDURL, clean(cfg.GetString("process_config.process_dd_url")))
	SetAgentMetadata(AgentConfigProxyHTTP, clean(cfg.GetString("proxy.http")))
	SetAgentMetadata(AgentConfigProxyHTTPS, clean(cfg.GetString("proxy.https")))
	SetAgentMetadata(AgentCWSEnabled, config.Datadog.GetBool("runtime_security_config.enabled"))
	SetAgentMetadata(AgentProcessEnabled, config.Datadog.GetBool("process_config.process_collection.enabled"))
	SetAgentMetadata(AgentProcessesContainerEnabled, config.Datadog.GetBool("process_config.container_collection.enabled"))
	SetAgentMetadata(AgentNetworksEnabled, config.Datadog.GetBool("network_config.enabled"))
	SetAgentMetadata(AgentNetworksHTTPEnabled, config.Datadog.GetBool("network_config.enable_http_monitoring"))
	SetAgentMetadata(AgentNetworksHTTPSEnabled, config.Datadog.GetBool("network_config.enable_https_monitoring"))
	SetAgentMetadata(AgentNetworksGoTLSEnabled, config.Datadog.GetBool("system_probe_config.enable_go_tls_support"))
	SetAgentMetadata(AgentNetworksJavaTLSEnabled, config.Datadog.GetBool("system_probe_config.enable_java_tls_support"))
	SetAgentMetadata(AgentLogsEnabled, config.Datadog.GetBool("logs_enabled"))
	SetAgentMetadata(AgentCSPMEnabled, config.Datadog.GetBool("compliance_config.enabled"))
	SetAgentMetadata(AgentAPMEnabled, config.Datadog.GetBool("apm_config.enabled"))
	// NOTE: until otlp config stabilizes, we set AgentOTLPEnabled in cmd/agent/app/run.go
	// Also note we can't import OTLP here, as it would trigger an import loop - if we see another
	// case like that, we should move otlp.IsEnabled to pkg/config/otlp
}
