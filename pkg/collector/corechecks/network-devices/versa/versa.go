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
)

// Configuration for the Versa check
type checkCfg struct {
	// add versa specific fields
	Name                            string   `yaml:"name"` // TODO: remove this field, only added it for testing
	DirectorEndpoint                string   `yaml:"director_endpoint"`
	AnalyticsEndpoint               string   `yaml:"analytics_endpoint"`
	Username                        string   `yaml:"username"`
	Password                        string   `yaml:"password"`
	UseHTTP                         bool     `yaml:"use_http"`
	Namespace                       string   `yaml:"namespace"`
	IncludedTenants                 []string `yaml:"included_tenants"`
	ExcludedTenants                 []string `yaml:"excluded_tenants"`
	SendNDMMetadata                 *bool    `yaml:"send_ndm_metadata"`
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

	c, err := client.NewClient(v.config.DirectorEndpoint, v.config.AnalyticsEndpoint, v.config.Username, v.config.Password, v.config.UseHTTP)
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

	// Gather appliances for each organization
	var appliances []client.Appliance
	for _, org := range organizations {
		orgAppliances, err := c.GetChildAppliancesDetail(org.Name)
		if err != nil {
			return fmt.Errorf("error getting appliances from Versa client: %w", err)
		}

		for _, appliance := range orgAppliances {
			log.Tracef("Processing appliance: %+v", appliance)
		}
		appliances = append(appliances, orgAppliances...)
	}

	// Get director status
	directorStatus, err := c.GetDirectorStatus()
	if err != nil {
		return fmt.Errorf("error getting director status from Versa client: %w", err)
	}

	// Convert Versa objects to device metadata
	deviceMetadata := make([]devicemetadata.DeviceMetadata, 0, len(appliances)+1)
	deviceMetadata = append(deviceMetadata, payload.GetDeviceMetadataFromAppliances(v.config.Namespace, appliances)...)

	directorDeviceMetadata, err := payload.GetDeviceMetadataFromDirector(v.config.Namespace, directorStatus)
	if err != nil {
		log.Errorf("error getting director device metadata: %v", err)
	} else {
		deviceMetadata = append(deviceMetadata, directorDeviceMetadata)
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

	// Send the metadata to the metrics sender
	if *v.config.SendNDMMetadata {
		v.metricsSender.SendMetadata(deviceMetadata, nil, nil)
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

	if *v.config.CollectSLAMetrics {
		deviceNameToIDMap := generateDeviceNameToIPMap(deviceMetadata)
		slaMetrics, err := c.GetSLAMetrics()
		if err != nil {
			log.Warnf("error getting SLA metrics from Versa client: %v", err)
		}
		v.metricsSender.SendSLAMetrics(slaMetrics, deviceNameToIDMap)
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
	instanceConfig.SendNDMMetadata = boolPointer(true)

	instanceConfig.CollectBFDSessionStatus = boolPointer(false)
	instanceConfig.CollectHardwareStatus = boolPointer(false)
	instanceConfig.CollectCloudApplicationsMetrics = boolPointer(false)
	instanceConfig.CollectBGPNeighborStates = boolPointer(false)
	instanceConfig.CollectSLAMetrics = boolPointer(false)

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

	v.metricsSender = report.NewSender(sender, v.config.Namespace)

	return nil
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
