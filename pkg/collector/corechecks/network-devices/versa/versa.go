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

	yaml "go.yaml.in/yaml/v2"

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
	DirectorEndpoint                      string   `yaml:"director_endpoint"`
	DirectorPort                          int      `yaml:"director_port"`
	AnalyticsEndpoint                     string   `yaml:"analytics_endpoint"`
	Username                              string   `yaml:"username"`
	Password                              string   `yaml:"password"`
	AuthMethod                            string   `yaml:"auth_method"`
	MaxAttempts                           int      `yaml:"max_attempts"`
	MaxPages                              int      `yaml:"max_pages"`
	MaxCount                              int      `yaml:"max_count"`
	LookbackTimeWindowMinutes             int      `yaml:"lookback_time_window_minutes"`
	UseHTTP                               bool     `yaml:"use_http"`
	Insecure                              bool     `yaml:"insecure"`
	CAFile                                string   `yaml:"ca_file"`
	Namespace                             string   `yaml:"namespace"`
	IncludedTenants                       []string `yaml:"included_tenants"`
	ExcludedTenants                       []string `yaml:"excluded_tenants"`
	SendDeviceMetadata                    *bool    `yaml:"send_device_metadata"`
	SendInterfaceMetadata                 *bool    `yaml:"send_interface_metadata"`
	MinCollectionInterval                 int      `yaml:"min_collection_interval"`
	CollectHardwareMetrics                *bool    `yaml:"collect_hardware_metrics"`
	CollectDirectorInterfaceMetrics       *bool    `yaml:"collect_director_interface_metrics"`
	CollectTunnelMetrics                  *bool    `yaml:"collect_tunnel_metrics"`
	CollectSLAMetrics                     *bool    `yaml:"collect_sla_metrics"`
	CollectLinkMetrics                    *bool    `yaml:"collect_link_metrics"`
	CollectApplicationsByApplianceMetrics *bool    `yaml:"collect_applications_by_appliance_metrics"`
	CollectTopUserMetrics                 *bool    `yaml:"collect_top_user_metrics"`
	CollectQoSMetrics                     *bool    `yaml:"collect_qos_metrics"`
	CollectDIAMetrics                     *bool    `yaml:"collect_dia_metrics"`
	CollectSiteMetrics                    *bool    `yaml:"collect_site_metrics"`
	CollectInterfaceMetrics               *bool    `yaml:"collect_interface_metrics"`
	SendInterfaceMetadataFromAnalytics    *bool    `yaml:"send_interface_metadata_from_analytics"`
	ClientID                              string   `yaml:"client_id"`
	ClientSecret                          string   `yaml:"client_secret"`
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
	log.Infof("Running Versa check")

	clientOptions, err := v.buildClientOptions()
	if err != nil {
		return err
	}

	authConfig := client.AuthConfig{
		Method:       v.config.AuthMethod,
		Username:     v.config.Username,
		Password:     v.config.Password,
		ClientID:     v.config.ClientID,
		ClientSecret: v.config.ClientSecret,
	}

	c, err := client.NewClient(v.config.DirectorEndpoint, v.config.DirectorPort, v.config.AnalyticsEndpoint, v.config.UseHTTP, authConfig, clientOptions...)
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
	needsDeviceMapping := *v.config.SendInterfaceMetadata || *v.config.CollectDirectorInterfaceMetrics ||
		*v.config.CollectSLAMetrics || *v.config.CollectLinkMetrics || *v.config.CollectSiteMetrics ||
		*v.config.CollectApplicationsByApplianceMetrics || *v.config.CollectTopUserMetrics ||
		*v.config.CollectTunnelMetrics || *v.config.CollectQoSMetrics || *v.config.CollectDIAMetrics ||
		*v.config.CollectInterfaceMetrics

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

		// Grab interfaces if we need interface metadata
		// Note: collect_interface_metrics depends on this data, but requires explicit send_interface_metadata enablement
		if *v.config.SendInterfaceMetadata {
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

	if *v.config.CollectDirectorInterfaceMetrics {
		// Validate that interface metadata collection is enabled since we depend on interface data
		if !*v.config.SendInterfaceMetadata {
			log.Errorf("collect_director_interface_metrics requires send_interface_metadata to be enabled. " +
				"Director interface metrics collection depends on interface data from the GetInterfaces API call, " +
				"which is only made when send_interface_metadata is enabled. Skipping director interface metrics collection.")
		} else {

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

		// Collect site metrics if enabled
		if *v.config.CollectSiteMetrics {
			siteMetrics, err := c.GetSiteMetrics(org.Name)
			if err != nil {
				log.Errorf("error getting site metrics from organization %s: %v", org.Name, err)
			} else {
				v.metricsSender.SendSiteMetrics(siteMetrics, deviceNameToIDMap)
			}
		}

		// Collect applications by appliance metrics if enabled
		if *v.config.CollectApplicationsByApplianceMetrics {
			appsByApplianceMetrics, err := c.GetApplicationsByAppliance(org.Name)
			if err != nil {
				log.Errorf("error getting applications by appliance metrics from organization %s: %v", org.Name, err)
			} else {
				v.metricsSender.SendApplicationsByApplianceMetrics(appsByApplianceMetrics, deviceNameToIDMap)
			}
		}

		// Collect top user metrics if enabled
		if *v.config.CollectTopUserMetrics {
			topUserMetrics, err := c.GetTopUsers(org.Name)
			if err != nil {
				log.Errorf("error getting top user metrics from organization %s: %v", org.Name, err)
			} else {
				v.metricsSender.SendTopUserMetrics(topUserMetrics, deviceNameToIDMap)
			}
		}

		// Collect tunnel metrics if enabled
		if *v.config.CollectTunnelMetrics {
			tunnelMetrics, err := c.GetTunnelMetrics(org.Name)
			if err != nil {
				log.Warnf("error getting tunnel metrics for tenant %s from Versa client: %v", org.Name, err)
				continue
			}
			v.metricsSender.SendTunnelMetrics(tunnelMetrics, deviceNameToIDMap)
		}

		// Collect QoS metrics if enabled
		if *v.config.CollectQoSMetrics {
			qosMetrics, err := c.GetPathQoSMetrics(org.Name)
			if err != nil {
				log.Errorf("error getting QoS metrics from organization %s: %v", org.Name, err)
			} else {
				v.metricsSender.SendPathQoSMetrics(qosMetrics, deviceNameToIDMap)
			}
		}

		// Collect DIA metrics if enabled
		if *v.config.CollectDIAMetrics {
			diaMetrics, err := c.GetDIAMetrics(org.Name)
			if err != nil {
				log.Errorf("error getting DIA metrics from organization %s: %v", org.Name, err)
			} else {
				v.metricsSender.SendDIAMetrics(diaMetrics, deviceNameToIDMap)
			}
		}

		// Collect interface metrics if enabled (from analytics)
		if *v.config.CollectInterfaceMetrics {
			analyticsInterfaceMetrics, err := c.GetAnalyticsInterfaces(org.Name)
			if err != nil {
				log.Errorf("error getting analytics interface metrics from organization %s: %v", org.Name, err)
				continue
			}

			if len(analyticsInterfaceMetrics) > 0 {
				// Send metrics
				v.metricsSender.SendAnalyticsInterfaceMetrics(analyticsInterfaceMetrics, deviceNameToIDMap)

				// TODO: I'd really like to do something like this for larger customers where the director
				// call is too large, but we need to find or agree on something to use for interface status
				// today, nothing exists
				// If explicitly configured to send interface metadata from analytics and we haven't
				// already sent interface metadata, create and send it from analytics data
				// if *v.config.SendInterfaceMetadataFromAnalytics && !(*v.config.SendInterfaceMetadata) && !(*v.config.CollectInterfaceMetrics) {
				// 	orgInterfaceMetadata, err := payload.GetInterfaceMetadataFromAnalytics(v.config.Namespace, deviceNameToIDMap, analyticsInterfaceMetrics)
				// 	if err != nil {
				// 		log.Errorf("error creating interface metadata from analytics for organization %s: %v", org.Name, err)
				// 	} else if len(orgInterfaceMetadata) > 0 {
				// 		log.Tracef("sending interface metadata from analytics for organization %s: %+v", org.Name, orgInterfaceMetadata)
				// 		v.metricsSender.SendMetadata(nil, orgInterfaceMetadata, nil)
				// 	}
				// }
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
	instanceConfig.SendDeviceMetadata = boolPointer(true)
	instanceConfig.SendInterfaceMetadata = boolPointer(false)
	instanceConfig.CollectDirectorInterfaceMetrics = boolPointer(false)

	instanceConfig.CollectSLAMetrics = boolPointer(false)
	instanceConfig.CollectTunnelMetrics = boolPointer(false)
	instanceConfig.CollectLinkMetrics = boolPointer(false)
	instanceConfig.CollectApplicationsByApplianceMetrics = boolPointer(false)
	instanceConfig.CollectTopUserMetrics = boolPointer(false)
	instanceConfig.CollectQoSMetrics = boolPointer(false)
	instanceConfig.CollectDIAMetrics = boolPointer(false)
	instanceConfig.CollectSiteMetrics = boolPointer(false)
	instanceConfig.CollectInterfaceMetrics = boolPointer(false)
	instanceConfig.SendInterfaceMetadataFromAnalytics = boolPointer(false)

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
