// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

// Package ciscosdwan implements NDM Cisco SD-WAN corecheck
package ciscosdwan

import (
	"context"
	"fmt"
	"time"

	"go.yaml.in/yaml/v3"

	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/integration"
	"github.com/DataDog/datadog-agent/pkg/aggregator/sender"
	"github.com/DataDog/datadog-agent/pkg/collector/check"
	core "github.com/DataDog/datadog-agent/pkg/collector/corechecks"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/network-devices/cisco-sdwan/client"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/network-devices/cisco-sdwan/payload"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/network-devices/cisco-sdwan/report"
	"github.com/DataDog/datadog-agent/pkg/snmp/utils"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/datadog-agent/pkg/util/option"
)

const (
	// CheckName is the name of the check
	CheckName            = "cisco_sdwan"
	defaultCheckInterval = 1 * time.Minute
)

// Configuration for the Cisco SD-WAN check
type checkCfg struct {
	VManageEndpoint                 string  `yaml:"vmanage_endpoint"`
	Username                        string  `yaml:"username"`
	Password                        string  `yaml:"password"`
	Namespace                       string  `yaml:"namespace"`
	MaxAttempts                     int     `yaml:"max_attempts"`
	MaxPages                        int     `yaml:"max_pages"`
	MaxCount                        int     `yaml:"max_count"`
	LookbackTimeWindowMinutes       int     `yaml:"lookback_time_window_minutes"`
	MaxRequestsPerSecond            float64 `yaml:"max_requests_per_second"`
	RateLimitBurst                  int     `yaml:"rate_limit_burst"`
	UseHTTP                         bool    `yaml:"use_http"`
	Insecure                        bool    `yaml:"insecure"`
	CAFile                          string  `yaml:"ca_file"`
	SendNDMMetadata                 *bool   `yaml:"send_ndm_metadata"`
	MinCollectionInterval           int     `yaml:"min_collection_interval"`
	CollectHardwareMetrics          *bool   `yaml:"collect_hardware_metrics"`
	CollectInterfaceMetrics         *bool   `yaml:"collect_interface_metrics"`
	CollectTunnelMetrics            *bool   `yaml:"collect_tunnel_metrics"`
	CollectControlConnectionMetrics *bool   `yaml:"collect_control_connection_metrics"`
	CollectOMPPeerMetrics           *bool   `yaml:"collect_omp_peer_metrics"`
	CollectDeviceCountersMetrics    *bool   `yaml:"collect_device_counters_metrics"`
	CollectBFDSessionStatus         *bool   `yaml:"collect_bfd_session_status"`
	CollectHardwareStatus           *bool   `yaml:"collect_hardware_status"`
	CollectCloudApplicationsMetrics *bool   `yaml:"collect_cloud_applications_metrics"`
	CollectBGPNeighborStates        *bool   `yaml:"collect_bgp_neighbor_states"`
}

// newClient creates the Cisco SD-WAN API client.
// Useful for mocking
var newClient = client.NewClient

// CiscoSdwanCheck contains the field for the CiscoSdwanCheck
type CiscoSdwanCheck struct {
	core.CheckBase
	interval      time.Duration
	config        checkCfg
	metricsSender *report.SDWanSender
	// ctx is cancelled when the check is stopped or unscheduled, to interrupt API requests
	ctx    context.Context
	cancel context.CancelFunc
}

// Run executes the check
func (c *CiscoSdwanCheck) Run() error {
	clientOptions, err := c.buildClientOptions()
	if err != nil {
		return err
	}

	// Create Cisco SD-WAN API client
	client, err := newClient(c.config.VManageEndpoint, c.config.Username, c.config.Password, c.config.UseHTTP, clientOptions...)
	if err != nil {
		return err
	}

	devices, err := client.GetDevices(c.ctx)
	if err != nil {
		log.Warnf("Error getting devices from Cisco SD-WAN API: %s", err)
	}
	vEdgeInterfaces, err := client.GetVEdgeInterfaces(c.ctx)
	if err != nil {
		log.Warnf("Error getting vEdge interfaces from Cisco SD-WAN API: %s", err)
	}
	cEdgeInterfaces, err := client.GetCEdgeInterfaces(c.ctx)
	if err != nil {
		log.Warnf("Error getting cEdge interfaces from Cisco SD-WAN API: %s", err)
	}

	devicesMetadata := payload.GetDevicesMetadata(c.config.Namespace, devices)
	interfaces := payload.ConvertInterfaces(vEdgeInterfaces, cEdgeInterfaces)
	interfacesMetadata, interfacesMap := payload.GetInterfacesMetadata(c.config.Namespace, interfaces)
	ipAddressesMetadata := payload.GetIPAddressesMetadata(c.config.Namespace, interfaces)

	deviceTags := payload.GetDevicesTags(c.config.Namespace, devices)
	c.metricsSender.SetDeviceTags(deviceTags)

	if *c.config.CollectHardwareMetrics {
		deviceStats, err := client.GetDeviceHardwareMetrics(c.ctx)
		if err != nil {
			log.Warnf("Error getting device metrics from Cisco SD-WAN API: %s", err)
		}

		uptimes := payload.GetDevicesUptime(devices)
		deviceStatus := payload.GetDevicesStatus(devices)

		c.metricsSender.SendDeviceMetrics(deviceStats)
		c.metricsSender.SendUptimeMetrics(uptimes)
		c.metricsSender.SendDeviceStatusMetrics(deviceStatus)
	}

	if *c.config.CollectInterfaceMetrics {
		interfaceStats, err := client.GetInterfacesMetrics(c.ctx)
		if err != nil {
			log.Warnf("Error getting interface metrics from Cisco SD-WAN API: %s", err)
		}
		c.metricsSender.SendInterfaceMetrics(interfaceStats, interfacesMap)
	}

	if *c.config.CollectTunnelMetrics {
		appRouteStats, err := client.GetApplicationAwareRoutingMetrics(c.ctx)
		if err != nil {
			log.Warnf("Error getting application-aware routing metrics from Cisco SD-WAN API: %s", err)
		}
		c.metricsSender.SendAppRouteMetrics(appRouteStats)
	}

	if *c.config.CollectControlConnectionMetrics {
		controlConnectionsState, err := client.GetControlConnectionsState(c.ctx)
		if err != nil {
			log.Warnf("Error getting control-connection states from Cisco SD-WAN API: %s", err)
		}
		c.metricsSender.SendControlConnectionMetrics(controlConnectionsState)
	}

	if *c.config.CollectOMPPeerMetrics {
		ompPeersState, err := client.GetOMPPeersState(c.ctx)
		if err != nil {
			log.Warnf("Error getting OMP peer states from Cisco SD-WAN API: %s", err)
		}
		c.metricsSender.SendOMPPeerMetrics(ompPeersState)
	}

	if *c.config.CollectDeviceCountersMetrics {
		deviceCounters, err := client.GetDevicesCounters(c.ctx)
		if err != nil {
			log.Warnf("Error getting device counters from Cisco SD-WAN API: %s", err)
		}
		c.metricsSender.SendDeviceCountersMetrics(deviceCounters)
	}

	// Disabled  by default
	if *c.config.CollectBFDSessionStatus {
		bfdSessionsState, err := client.GetBFDSessionsState(c.ctx)
		if err != nil {
			log.Warnf("Error getting BFD session states from Cisco SD-WAN API: %s", err)
		}
		c.metricsSender.SendBFDSessionMetrics(bfdSessionsState)
	}

	// Disabled  by default
	if *c.config.CollectHardwareStatus {
		hardwareStates, err := client.GetHardwareStates(c.ctx)
		if err != nil {
			log.Warnf("Error getting hardware states from Cisco SD-WAN API: %s", err)
		}
		c.metricsSender.SendHardwareMetrics(hardwareStates)
	}

	// Disabled  by default
	if *c.config.CollectCloudApplicationsMetrics {
		cloudApplications, err := client.GetCloudExpressMetrics(c.ctx)
		if err != nil {
			log.Warnf("Error getting cloud application metrics from Cisco SD-WAN API: %s", err)
		}
		c.metricsSender.SendCloudApplicationMetrics(cloudApplications)
	}

	// Disabled  by default
	if *c.config.CollectBGPNeighborStates {
		bgpNeighbors, err := client.GetBGPNeighbors(c.ctx)
		if err != nil {
			log.Warnf("Error getting BGP neighbors from Cisco SD-WAN API: %s", err)
		}
		c.metricsSender.SendBGPNeighborMetrics(bgpNeighbors)
	}

	if *c.config.SendNDMMetadata {
		c.metricsSender.SendMetadata(devicesMetadata, interfacesMetadata, ipAddressesMetadata)
	}

	// Commit
	c.metricsSender.Commit()

	return nil
}

// Configure the Cisco SD-WAN check
func (c *CiscoSdwanCheck) Configure(senderManager sender.SenderManager, integrationConfigDigest uint64, rawInstance integration.Data, rawInitConfig integration.Data, source string, provider string) error {
	// Must be called before c.CommonConfigure
	c.BuildID(integrationConfigDigest, rawInstance, rawInitConfig)

	err := c.CommonConfigure(senderManager, rawInitConfig, rawInstance, source, provider)
	if err != nil {
		return err
	}

	sender, err := c.GetSender()
	if err != nil {
		return err
	}

	var instanceConfig checkCfg

	// Set defaults before unmarshalling
	instanceConfig.CollectHardwareMetrics = boolPointer(true)
	instanceConfig.CollectInterfaceMetrics = boolPointer(true)
	instanceConfig.CollectTunnelMetrics = boolPointer(true)
	instanceConfig.CollectControlConnectionMetrics = boolPointer(true)
	instanceConfig.CollectOMPPeerMetrics = boolPointer(true)
	instanceConfig.CollectDeviceCountersMetrics = boolPointer(true)
	instanceConfig.SendNDMMetadata = boolPointer(true)

	instanceConfig.CollectBFDSessionStatus = boolPointer(false)
	instanceConfig.CollectHardwareStatus = boolPointer(false)
	instanceConfig.CollectCloudApplicationsMetrics = boolPointer(false)
	instanceConfig.CollectBGPNeighborStates = boolPointer(false)

	err = yaml.Unmarshal(rawInstance, &instanceConfig)
	if err != nil {
		return err
	}
	c.config = instanceConfig

	if c.config.Namespace == "" {
		c.config.Namespace = "default"
	} else {
		namespace, err := utils.NormalizeNamespace(c.config.Namespace)
		if err != nil {
			return err
		}
		c.config.Namespace = namespace
	}

	if c.config.MinCollectionInterval != 0 {
		c.interval = time.Second * time.Duration(c.config.MinCollectionInterval)
	}

	if c.config.MaxRequestsPerSecond < 0 {
		return fmt.Errorf("max_requests_per_second must be positive, got %v", c.config.MaxRequestsPerSecond)
	}
	if c.config.MaxRequestsPerSecond > 0 && c.config.MaxRequestsPerSecond*c.interval.Seconds() < 1 {
		return fmt.Errorf("max_requests_per_second must allow at least one request per check interval (%s), got %v", c.interval, c.config.MaxRequestsPerSecond)
	}

	c.metricsSender = report.NewSDWanSender(sender, c.config.Namespace)

	return nil
}

func (c *CiscoSdwanCheck) buildClientOptions() ([]client.ClientOptions, error) {
	var clientOptions []client.ClientOptions

	if c.config.Insecure || c.config.CAFile != "" {
		options, err := client.WithTLSConfig(c.config.Insecure, c.config.CAFile)
		if err != nil {
			return nil, err
		}

		clientOptions = append(clientOptions, options)
	}

	if c.config.MaxAttempts > 0 {
		clientOptions = append(clientOptions, client.WithMaxAttempts(c.config.MaxAttempts))
	}

	// Retrying a request must not outlast a check run
	clientOptions = append(clientOptions, client.WithMaxRetryDuration(c.interval))

	if c.config.MaxPages > 0 {
		clientOptions = append(clientOptions, client.WithMaxPages(c.config.MaxPages))
	}

	if c.config.MaxCount > 0 {
		clientOptions = append(clientOptions, client.WithMaxCount(c.config.MaxCount))
	}

	if c.config.LookbackTimeWindowMinutes > 0 {
		clientOptions = append(clientOptions, client.WithLookback(time.Minute*time.Duration(c.config.LookbackTimeWindowMinutes)))
	}

	if c.config.MaxRequestsPerSecond > 0 {
		// A burst of 0 would never allow any request, default to strict pacing
		burst := c.config.RateLimitBurst
		if burst <= 0 {
			burst = 1
		}
		// A rate limiter wait must not outlast a check run
		clientOptions = append(clientOptions, client.WithRateLimit(c.config.MaxRequestsPerSecond, burst, c.interval))
	}

	return clientOptions, nil
}

// Interval returns the scheduling time for the check
func (c *CiscoSdwanCheck) Interval() time.Duration {
	return c.interval
}

// Stop interrupts the API requests of a running check
func (c *CiscoSdwanCheck) Stop() {
	c.cancel()
}

// Cancel interrupts the API requests of a running check when it is unscheduled
func (c *CiscoSdwanCheck) Cancel() {
	c.cancel()
}

// IsHASupported returns true if the check supports HA
func (c *CiscoSdwanCheck) IsHASupported() bool {
	return true
}

func boolPointer(b bool) *bool {
	return &b
}

// Factory creates a new check factory
func Factory() option.Option[func() check.Check] {
	return option.New(newCheck)
}

func newCheck() check.Check {
	ctx, cancel := context.WithCancel(context.Background())
	return &CiscoSdwanCheck{
		CheckBase: core.NewCheckBase(CheckName),
		interval:  defaultCheckInterval,
		ctx:       ctx,
		cancel:    cancel,
	}
}
