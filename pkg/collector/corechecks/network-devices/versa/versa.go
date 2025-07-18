// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

// Package versa implements NDM Versa corecheck
package versa

import (
	"fmt"
	"strings"
	"time"

	yaml "gopkg.in/yaml.v2"

	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/integration"
	"github.com/DataDog/datadog-agent/pkg/aggregator/sender"
	"github.com/DataDog/datadog-agent/pkg/collector/check"
	core "github.com/DataDog/datadog-agent/pkg/collector/corechecks"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/network-devices/versa/client"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/network-devices/versa/payload"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/network-devices/versa/report"
	devicemetadata "github.com/DataDog/datadog-agent/pkg/networkdevice/metadata"
	"github.com/DataDog/datadog-agent/pkg/snmp/utils"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/datadog-agent/pkg/util/option"
)

const (
	// CheckName is the name of the check
	CheckName            = "versa"
	defaultCheckInterval = 1 * time.Minute
	defaultDirectorPort  = 9182
)

// Configuration for the Versa check
type checkCfg struct {
	// add versa specific fields
	Name                            string   `yaml:"name"` // TODO: remove this field, only added it for testing
	DirectorEndpoint                string   `yaml:"director_endpoint"`
	DirectorPort                    int      `yaml:"director_port"`
	AnalyticsEndpoint               string   `yaml:"analytics_endpoint"`
	Username                        string   `yaml:"username"`
	Password                        string   `yaml:"password"`
	MaxAttempts                     int      `yaml:"max_attempts"`
	MaxPages                        int      `yaml:"max_pages"`
	MaxCount                        int      `yaml:"max_count"`
	LookbackTimeWindowMinutes       int      `yaml:"lookback_time_window_minutes"`
	UseHTTP                         bool     `yaml:"use_http"`
	Insecure                        bool     `yaml:"insecure"`
	CAFile                          string   `yaml:"ca_file"`
	Namespace                       string   `yaml:"namespace"`
	IncludedTenants                 []string `yaml:"included_tenants"`
	ExcludedTenants                 []string `yaml:"excluded_tenants"`
	SendDeviceMetadata              *bool    `yaml:"send_device_metadata"`
	SendInterfaceMetadata           *bool    `yaml:"send_interface_metadata"`
	MinCollectionInterval           int      `yaml:"min_collection_interval"`
	CollectHardwareMetrics          *bool    `yaml:"collect_hardware_metrics"`
	CollectInterfaceMetrics         *bool    `yaml:"collect_interface_metrics"`
	CollectTunnelMetrics            *bool    `yaml:"collect_tunnel_metrics"`
	CollectControlConnectionMetrics *bool    `yaml:"collect_control_connection_metrics"`
	CollectOMPPeerMetrics           *bool    `yaml:"collect_omp_peer_metrics"`
	CollectDeviceCountersMetrics    *bool    `yaml:"collect_device_counters_metrics"`
	CollectBFDSessionStatus         *bool    `yaml:"collect_bfd_session_status"`
	CollectHardwareStatus           *bool    `yaml:"collect_hardware_status"`
	CollectCloudApplicationsMetrics *bool    `yaml:"collect_cloud_applications_metrics"`
	CollectBGPNeighborStates        *bool    `yaml:"collect_bgp_neighbor_states"`
	CollectSLAMetrics               *bool    `yaml:"collect_sla_metrics"`
	CollectLinkMetrics              *bool    `yaml:"collect_link_metrics"`
}

// VersaCheck contains the fields for the Versa check
type VersaCheck struct {
	core.CheckBase
	interval      time.Duration
	config        checkCfg
	metricsSender *report.Sender
}

// Run executes the check
func (v *VersaCheck) Run() error {
	log.Infof("Running Versa check for instance: %s", v.config.Name)

	clientOptions, err := v.buildClientOptions()
	if err != nil {
		return err
	}

	c, err := client.NewClient(v.config.DirectorEndpoint, v.config.DirectorPort, v.config.AnalyticsEndpoint, v.config.Username, v.config.Password, v.config.UseHTTP, clientOptions...)
	if err != nil {
		return fmt.Errorf("error creating Versa client: %w", err)
	}

	// Get all the organizations, so we can get the appliances
	// for each. We should only need to get the top level organizations
	// which act as tenants. Appliances of child tennants should be part
	// of the parent tenant.
	organizations, err := c.GetOrganizations()
	if err != nil {
		return fmt.Errorf("error getting organizations from Versa client: %w", err)
	}
	log.Tracef("Unfiltered organizations: %v", organizations)
	organizations = filterOrganizations(organizations, v.config.IncludedTenants, v.config.ExcludedTenants)
	log.Tracef("Filtered organizations: %v", organizations)

	// Get director status (independent of organizations)
	directorStatus, err := c.GetDirectorStatus()
	if err != nil {
		return fmt.Errorf("error getting director status from Versa client: %w", err)
	}

	// Process each organization and collect all required data
	var appliances []client.Appliance
	var interfaces []client.Interface

	// Determine if we need appliances for device mapping
	needsDeviceMapping := *v.config.SendInterfaceMetadata || *v.config.CollectInterfaceMetrics ||
		*v.config.CollectSLAMetrics || *v.config.CollectLinkMetrics

	for _, org := range organizations {
		log.Tracef("Processing organization: %s", org.Name)

		// Gather appliances if we need device metadata, hardware metrics, or device mapping
		if *v.config.SendDeviceMetadata || *v.config.CollectHardwareMetrics || needsDeviceMapping {
			orgAppliances, err := c.GetChildAppliancesDetail(org.Name)
			if err != nil {
				log.Errorf("error getting appliances from organization %s: %v", org.Name, err)
			} else {
				for _, appliance := range orgAppliances {
					log.Tracef("Processing appliance: %+v", appliance)
				}
				appliances = append(appliances, orgAppliances...)
			}
		}

		// Grab interfaces if we need interface metadata or interface metrics
		if *v.config.SendInterfaceMetadata || *v.config.CollectInterfaceMetrics {
			orgInterfaces, err := c.GetInterfaces(org.Name)
			if err != nil {
				// not getting interfaces shouldn't stop the rest of the check
				log.Errorf("error getting interfaces from organization %s: %v", org.Name, err)
			} else {
				interfaces = append(interfaces, orgInterfaces...)
			}
		}
	}

	// Convert Versa objects to device metadata
	// If we collected appliances for any reason, always send device metadata since we already have it
	var deviceMetadata []devicemetadata.DeviceMetadata
	if len(appliances) > 0 {
		deviceMetadata = make([]devicemetadata.DeviceMetadata, 0, len(appliances)+1)
		deviceMetadata = append(deviceMetadata, payload.GetDeviceMetadataFromAppliances(v.config.Namespace, appliances)...)

		directorDeviceMetadata, err := payload.GetDeviceMetadataFromDirector(v.config.Namespace, directorStatus)
		if err != nil {
			log.Errorf("error getting director device metadata: %v", err)
		} else {
			deviceMetadata = append(deviceMetadata, directorDeviceMetadata)
		}
	}

	// Send the tags to the metrics sender
	deviceTags := payload.GetApplianceDevicesTags(v.config.Namespace, appliances)
	directorDeviceTags, err := payload.GetDirectorDeviceTags(v.config.Namespace, directorStatus)
	if err != nil {
		log.Warnf("error getting director device tags, director metrics will contain default tags: %v", err)
	}
	// TODO: is there any chance that the director IP overlaps with an appliance IP?
	for ip, tags := range directorDeviceTags {
		deviceTags[ip] = append(deviceTags[ip], tags...)
	}
	v.metricsSender.SetDeviceTagsMap(deviceTags)

	deviceNameToIDMap := generateDeviceNameToIPMap(deviceMetadata)

	var interfaceMetadata []devicemetadata.InterfaceMetadata
	if *v.config.SendInterfaceMetadata {
		var err error
		interfaceMetadata, err = payload.GetInterfaceMetadata(v.config.Namespace, deviceNameToIDMap, interfaces)
		if err != nil {
			if len(interfaceMetadata) == 0 {
				log.Errorf("failed to parse all interface metadata: %v", err)
			} else {
				log.Errorf("partial failure in parsing interface metadata: %v", err)
			}
		}

		log.Tracef("interfaces are as follows: %+v", interfaceMetadata)
	}

	// Send the metadata to the metrics sender
	if len(deviceMetadata) > 0 || len(interfaceMetadata) > 0 {
		v.metricsSender.SendMetadata(deviceMetadata, interfaceMetadata, nil)
	}

	// Send interface status metrics
	if *v.config.SendInterfaceMetadata {
		v.metricsSender.SendInterfaceStatus(interfaces, deviceNameToIDMap)
	}

	// Send hardware metrics to the metrics sender
	if *v.config.CollectHardwareMetrics {
		uptimes := payload.GetDevicesUptime(appliances)
		deviceStatus := payload.GetDevicesStatus(appliances)

		v.metricsSender.SendDeviceMetrics(appliances)
		v.metricsSender.SendUptimeMetrics(uptimes)
		v.metricsSender.SendDeviceStatusMetrics(deviceStatus)

		// Director metrics
		v.metricsSender.SendDirectorDeviceMetrics(directorStatus)
		v.metricsSender.SendDirectorUptimeMetrics(directorStatus)
		v.metricsSender.SendDirectorStatus(directorStatus)
	}

	if *v.config.CollectInterfaceMetrics {
		type deviceID struct {
			ApplianceName string
			TenantName    string
		}
		deviceWithInterfaceMap := make(map[string]deviceID)
		for _, iface := range interfaces {
			deviceWithInterfaceMap[iface.TenantName+":"+iface.DeviceName] = deviceID{
				ApplianceName: iface.DeviceName,
				TenantName:    iface.TenantName,
			}
		}

		// Collect interface metrics for each device
		interfaceMetricsByDevice := make(map[string][]client.InterfaceMetrics)

		for _, id := range deviceWithInterfaceMap {
			interfaceMetrics, err := c.GetInterfaceMetrics(id.ApplianceName, id.TenantName)
			if err != nil {
				log.Errorf("error getting interface metrics for device %s in tenant %s: %v", id.ApplianceName, id.TenantName, err)
				continue
			}

			// Get device IP from the deviceNameToIDMap
			if deviceIP, ok := deviceNameToIDMap[id.ApplianceName]; ok {
				interfaceMetricsByDevice[deviceIP] = interfaceMetrics
			} else {
				log.Errorf("device IP not found for device %s, skipping interface metrics", id.ApplianceName)
			}
		}

		// Send interface metrics
		v.metricsSender.SendInterfaceMetrics(interfaceMetricsByDevice)
	}

	// Now collect organization-specific metrics that need deviceNameToIDMap
	for _, org := range organizations {
		// Collect SLA metrics if enabled
		if *v.config.CollectSLAMetrics {
			slaMetrics, err := c.GetSLAMetrics(org.Name)
			if err != nil {
				log.Errorf("error getting SLA metrics from organization %s: %v", org.Name, err)
			} else {
				v.metricsSender.SendSLAMetrics(slaMetrics, deviceNameToIDMap)
			}
		}

		// Collect link metrics if enabled
		if *v.config.CollectLinkMetrics {
			linkStatusMetrics, err := c.GetLinkStatusMetrics(org.Name)
			if err != nil {
				log.Errorf("error getting link status metrics from organization %s: %v", org.Name, err)
			} else {
				v.metricsSender.SendLinkStatusMetrics(linkStatusMetrics, deviceNameToIDMap)
			}

			linkUsageMetrics, err := c.GetLinkUsageMetrics(org.Name)
			if err != nil {
				log.Errorf("error getting link usage metrics from organization %s: %v", org.Name, err)
			} else {
				v.metricsSender.SendLinkUsageMetrics(linkUsageMetrics, deviceNameToIDMap)
			}
		}
	}

	// Commit
	v.metricsSender.Commit()

	return nil
}

// Configure the Versa check
func (v *VersaCheck) Configure(senderManager sender.SenderManager, integrationConfigDigest uint64, rawInstance integration.Data, rawInitConfig integration.Data, source string) error {

	// Must be called before v.CommonConfigure
	v.BuildID(integrationConfigDigest, rawInstance, rawInitConfig)

	err := v.CommonConfigure(senderManager, rawInitConfig, rawInstance, source)
	if err != nil {
		return err
	}

	sender, err := v.GetSender()
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
	instanceConfig.SendDeviceMetadata = boolPointer(true)
	instanceConfig.SendInterfaceMetadata = boolPointer(true)

	instanceConfig.CollectBFDSessionStatus = boolPointer(false)
	instanceConfig.CollectHardwareStatus = boolPointer(false)
	instanceConfig.CollectCloudApplicationsMetrics = boolPointer(false)
	instanceConfig.CollectBGPNeighborStates = boolPointer(false)
	instanceConfig.CollectSLAMetrics = boolPointer(false)
	instanceConfig.CollectLinkMetrics = boolPointer(false)

	err = yaml.Unmarshal(rawInstance, &instanceConfig)
	if err != nil {
		return err
	}
	v.config = instanceConfig

	if v.config.Namespace == "" {
		v.config.Namespace = "default"
	} else {
		namespace, err := utils.NormalizeNamespace(v.config.Namespace)
		if err != nil {
			return err
		}
		v.config.Namespace = namespace
	}

	if v.config.MinCollectionInterval != 0 {
		v.interval = time.Second * time.Duration(v.config.MinCollectionInterval)
	}

	if v.config.DirectorPort == 0 {
		v.config.DirectorPort = defaultDirectorPort
	}

	v.metricsSender = report.NewSender(sender, v.config.Namespace)

	return nil
}

func (v *VersaCheck) buildClientOptions() ([]client.ClientOptions, error) {
	var clientOptions []client.ClientOptions

	if v.config.Insecure || v.config.CAFile != "" {
		options, err := client.WithTLSConfig(v.config.Insecure, v.config.CAFile)
		if err != nil {
			return nil, err
		}

		clientOptions = append(clientOptions, options)
	}

	if v.config.MaxAttempts > 0 {
		clientOptions = append(clientOptions, client.WithMaxAttempts(v.config.MaxAttempts))
	}

	if v.config.MaxPages > 0 {
		clientOptions = append(clientOptions, client.WithMaxPages(v.config.MaxPages))
	}

	if v.config.MaxCount > 0 {
		clientOptions = append(clientOptions, client.WithMaxCount(v.config.MaxCount))
	}

	if v.config.LookbackTimeWindowMinutes > 0 {
		clientOptions = append(clientOptions, client.WithLookback(v.config.LookbackTimeWindowMinutes))
	}

	return clientOptions, nil
}

// Interval returns the scheduling time for the check
func (v *VersaCheck) Interval() time.Duration {
	return v.interval
}

// IsHASupported returns true if the check supports HA
func (v *VersaCheck) IsHASupported() bool {
	// TODO: Is this true? I would think probably?
	return true
}

// filterOrganizations filters the list of organizations based on the included and excluded lists
// If an organization is on the included and excluded list, it will be excluded
func filterOrganizations(orgs []client.Organization, includedOrgs []string, excludedOrgs []string) []client.Organization {
	includedTenantsSet := make(map[string]struct{}, len(includedOrgs))
	for _, tenant := range includedOrgs {
		includedTenantsSet[strings.ToLower(tenant)] = struct{}{}
	}
	excludedTenantsSet := make(map[string]struct{}, len(excludedOrgs))
	for _, tenant := range excludedOrgs {
		excludedTenantsSet[strings.ToLower(tenant)] = struct{}{}
	}

	filteredOrgs := make([]client.Organization, 0, len(orgs))
	for _, org := range orgs {
		orgName := strings.ToLower(org.Name) // Normalize the organization name to lowercase
		// If includedTenants is not empty, only include tenants in the list
		if _, ok := includedTenantsSet[orgName]; len(includedOrgs) > 0 && !ok {
			continue
		}
		// If excludedTenants is not empty, exclude tenants in the list
		if _, ok := excludedTenantsSet[orgName]; len(excludedOrgs) > 0 && ok {
			continue
		}
		filteredOrgs = append(filteredOrgs, org)
	}

	return filteredOrgs
}

// TODO: should we convert the tags map to use ID instead of IP?
// generateDeviceNameToIPMap generates a map of device IP to device name to enrich the results from Analytics responses
func generateDeviceNameToIPMap(deviceMetadata []devicemetadata.DeviceMetadata) map[string]string {
	deviceNameToIPMap := make(map[string]string)
	for _, device := range deviceMetadata {
		if device.Name != "" {
			deviceNameToIPMap[device.Name] = device.IPAddress
		}
	}
	return deviceNameToIPMap
}

func boolPointer(b bool) *bool {
	return &b
}

// Factory creates a new check factory
func Factory() option.Option[func() check.Check] {
	return option.New(newCheck)
}

func newCheck() check.Check {
	return &VersaCheck{
		CheckBase: core.NewCheckBase(CheckName),
		interval:  defaultCheckInterval,
	}
}
