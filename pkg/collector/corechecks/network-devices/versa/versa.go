// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

// Package versa implements NDM Versa corecheck
package versa

import (
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	yaml "go.yaml.in/yaml/v3"

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
	// minTimeseriesLookbackMinutes is the shortest lookback that spans two buckets on a
	// tenant with 15 minute buckets.
	minTimeseriesLookbackMinutes = 30
	// CheckName is the name of the check
	CheckName            = "versa"
	defaultCheckInterval = 1 * time.Minute
	defaultDirectorPort  = 9182
)

// Request setting overrides for a single Versa endpoint, unset values inherit
// the instance-wide max_count, max_pages and client_timeout
type endpointOverrideCfg struct {
	MaxCount int `yaml:"max_count"`
	MaxPages int `yaml:"max_pages"`
	Timeout  int `yaml:"timeout"`
}

// deviceEntry is a single included_devices entry. It accepts either a bare appliance name:
//
//	included_devices:
//	  - branch-1
//
// or a mapping that also carries tags to attach to that device's metadata:
//
//	included_devices:
//	  - name: branch-1
//	    tags:
//	      env: prod
type deviceEntry struct {
	Name string            `yaml:"name"`
	Tags map[string]string `yaml:"tags"`
}

// UnmarshalYAML accepts both the plain-string and the mapping form of a device entry, so
// that configurations written against the string-only form keep working unchanged.
func (d *deviceEntry) UnmarshalYAML(unmarshal func(interface{}) error) error {
	var name string
	if err := unmarshal(&name); err == nil {
		d.Name = name
		return nil
	}

	// deviceEntryFields mirrors deviceEntry without inheriting this UnmarshalYAML method,
	// so unmarshalling the mapping form does not recurse back into it.
	type deviceEntryFields deviceEntry
	var fields deviceEntryFields
	if err := unmarshal(&fields); err != nil {
		return fmt.Errorf("included_devices entries must be a device name or a mapping with a name and tags: %w", err)
	}
	if fields.Name == "" {
		return errors.New("included_devices entry is missing a name")
	}

	*d = deviceEntry(fields)
	return nil
}

// Configuration for the Versa check
type checkCfg struct {
	DirectorEndpoint                      string        `yaml:"director_endpoint"`
	DirectorPort                          int           `yaml:"director_port"`
	AnalyticsEndpoint                     string        `yaml:"analytics_endpoint"`
	Username                              string        `yaml:"username"`
	Password                              string        `yaml:"password"`
	AuthMethod                            string        `yaml:"auth_method"`
	MaxAttempts                           int           `yaml:"max_attempts"`
	MaxPages                              int           `yaml:"max_pages"`
	MaxCount                              int           `yaml:"max_count"`
	ClientTimeout                         int           `yaml:"client_timeout"`
	LookbackTimeWindowMinutes             int           `yaml:"lookback_time_window_minutes"`
	AnalyticsQueryType                    string        `yaml:"analytics_query_type"`
	AnalyticsGap                          string        `yaml:"analytics_gap"`
	UseHTTP                               bool          `yaml:"use_http"`
	Insecure                              bool          `yaml:"insecure"`
	CAFile                                string        `yaml:"ca_file"`
	Namespace                             string        `yaml:"namespace"`
	IncludedTenants                       []string      `yaml:"included_tenants"`
	ExcludedTenants                       []string      `yaml:"excluded_tenants"`
	IncludedDevices                       []deviceEntry `yaml:"included_devices"`
	ExcludedDevices                       []string      `yaml:"excluded_devices"`
	SendDeviceMetadata                    *bool         `yaml:"send_device_metadata"`
	SendInterfaceMetadata                 *bool         `yaml:"send_interface_metadata"`
	MinCollectionInterval                 int           `yaml:"min_collection_interval"`
	CollectHardwareMetrics                *bool         `yaml:"collect_hardware_metrics"`
	CollectDirectorInterfaceMetrics       *bool         `yaml:"collect_director_interface_metrics"`
	CollectTunnelMetrics                  *bool         `yaml:"collect_tunnel_metrics"`
	CollectSLAMetrics                     *bool         `yaml:"collect_sla_metrics"`
	CollectLinkMetrics                    *bool         `yaml:"collect_link_metrics"`
	CollectApplicationsByApplianceMetrics *bool         `yaml:"collect_applications_by_appliance_metrics"`
	CollectTopUserMetrics                 *bool         `yaml:"collect_top_user_metrics"`
	CollectQoSMetrics                     *bool         `yaml:"collect_qos_metrics"`
	CollectCircuitQoSMetrics              *bool         `yaml:"collect_circuit_qos_metrics"`
	CollectDIAMetrics                     *bool         `yaml:"collect_dia_metrics"`
	CollectSiteMetrics                    *bool         `yaml:"collect_site_metrics"`
	CollectInterfaceMetrics               *bool         `yaml:"collect_interface_metrics"`
	SendInterfaceMetadataFromAnalytics    *bool         `yaml:"send_interface_metadata_from_analytics"`
	ClientID                              string        `yaml:"client_id"`
	ClientSecret                          string        `yaml:"client_secret"`

	EndpointOverrides map[string]endpointOverrideCfg `yaml:"endpoint_overrides"`
}

// VersaCheck contains the fields for the Versa check
type VersaCheck struct {
	core.CheckBase
	interval          time.Duration
	config            checkCfg
	endpointOverrides map[client.Endpoint]client.EndpointConfig
	metricsSender     *report.Sender
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
		*v.config.CollectTunnelMetrics || *v.config.CollectQoSMetrics || *v.config.CollectCircuitQoSMetrics ||
		*v.config.CollectDIAMetrics || *v.config.CollectInterfaceMetrics

	// Gather appliances if we need device metadata, hardware metrics, or device mapping
	collectAppliances := *v.config.SendDeviceMetadata || *v.config.CollectHardwareMetrics || needsDeviceMapping

	for _, org := range organizations {
		log.Tracef("Processing organization: %s", org.Name)

		if collectAppliances {
			orgAppliances, err := c.GetChildAppliancesDetail(org.Name)
			if err != nil {
				log.Errorf("error getting appliances from organization %s: %v", org.Name, err)
			} else {
				log.Tracef("Unfiltered appliances for organization %s: %d", org.Name, len(orgAppliances))
				orgAppliances = filterAppliances(orgAppliances, v.includedDeviceNames(), v.config.ExcludedDevices)
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

	// The set of devices that survived device filtering, or nil when filtering is disabled.
	// Everything downstream of appliance collection is filtered against this so that no
	// data is reported for a device the user did not opt in to.
	monitoredDevices := v.monitoredDeviceNames(appliances)

	// Drop interfaces belonging to appliances that device filtering excluded, so that
	// interface metadata and metrics stay consistent with the monitored device list.
	interfaces = filterInterfacesByDevice(interfaces, monitoredDevices, directorStatus.HAConfig.ClusterID)

	// Convert Versa objects to device metadata
	// If we collected appliances for any reason, always send device metadata since we already have it.
	// This is keyed off whether appliance collection ran rather than how many appliances came back, so
	// that the Director is still reported when device filtering leaves no appliances behind.
	var deviceMetadata []devicemetadata.DeviceMetadata
	if collectAppliances {
		deviceMetadata = make([]devicemetadata.DeviceMetadata, 0, len(appliances)+1)
		deviceMetadata = append(deviceMetadata, payload.GetDeviceMetadataFromAppliances(v.config.Namespace, appliances)...)

		directorDeviceMetadata, err := payload.GetDeviceMetadataFromDirector(v.config.Namespace, directorStatus)
		if err != nil {
			log.Errorf("error getting director device metadata: %v", err)
		} else {
			deviceMetadata = append(deviceMetadata, directorDeviceMetadata)
		}
	}

	// Attach any tags the user configured against individual devices in included_devices
	applyConfiguredDeviceTags(deviceMetadata, v.configuredDeviceTags())

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
				if deviceIP, ok := deviceNameToIDMap[strings.ToLower(id.ApplianceName)]; ok {
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
				slaMetrics = filterAnalyticsMetricsByDevice(slaMetrics, monitoredDevices, func(m client.SLAMetrics) string { return m.LocalSite })
				v.metricsSender.SendSLAMetrics(org.Name, slaMetrics, deviceNameToIDMap)
			}
		}

		// Collect link metrics if enabled
		if *v.config.CollectLinkMetrics {
			linkStatusMetrics, err := c.GetLinkStatusMetrics(org.Name)
			if err != nil {
				log.Errorf("error getting link status metrics from organization %s: %v", org.Name, err)
			} else {
				linkStatusMetrics = filterAnalyticsMetricsByDevice(linkStatusMetrics, monitoredDevices, func(m client.LinkStatusMetrics) string { return m.Site })
				v.metricsSender.SendLinkStatusMetrics(linkStatusMetrics, deviceNameToIDMap)
			}

			linkUsageMetrics, err := c.GetLinkUsageMetrics(org.Name)
			if err != nil {
				log.Errorf("error getting link usage metrics from organization %s: %v", org.Name, err)
			} else {
				linkUsageMetrics = filterAnalyticsMetricsByDevice(linkUsageMetrics, monitoredDevices, func(m client.LinkUsageMetrics) string { return m.Site })
				v.metricsSender.SendLinkUsageMetrics(linkUsageMetrics, deviceNameToIDMap)
			}
		}

		// Collect site metrics if enabled
		if *v.config.CollectSiteMetrics {
			siteMetrics, err := c.GetSiteMetrics(org.Name)
			if err != nil {
				log.Errorf("error getting site metrics from organization %s: %v", org.Name, err)
			} else {
				siteMetrics = filterAnalyticsMetricsByDevice(siteMetrics, monitoredDevices, func(m client.SiteMetrics) string { return m.Site })
				v.metricsSender.SendSiteMetrics(siteMetrics, deviceNameToIDMap)
			}
		}

		// Collect applications by appliance metrics if enabled
		if *v.config.CollectApplicationsByApplianceMetrics {
			appsByApplianceMetrics, err := c.GetApplicationsByAppliance(org.Name)
			if err != nil {
				log.Errorf("error getting applications by appliance metrics from organization %s: %v", org.Name, err)
			} else {
				appsByApplianceMetrics = filterAnalyticsMetricsByDevice(appsByApplianceMetrics, monitoredDevices, func(m client.ApplicationsByApplianceMetrics) string { return m.Site })
				v.metricsSender.SendApplicationsByApplianceMetrics(appsByApplianceMetrics, deviceNameToIDMap)
			}
		}

		// Collect top user metrics if enabled
		if *v.config.CollectTopUserMetrics {
			topUserMetrics, err := c.GetTopUsers(org.Name)
			if err != nil {
				log.Errorf("error getting top user metrics from organization %s: %v", org.Name, err)
			} else {
				topUserMetrics = filterAnalyticsMetricsByDevice(topUserMetrics, monitoredDevices, func(m client.TopUserMetrics) string { return m.Site })
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
			tunnelMetrics = filterAnalyticsMetricsByDevice(tunnelMetrics, monitoredDevices, func(m client.TunnelMetrics) string { return m.Appliance })
			v.metricsSender.SendTunnelMetrics(tunnelMetrics, deviceNameToIDMap)
		}

		// Collect per access circuit QoS metrics if enabled
		if *v.config.CollectCircuitQoSMetrics {
			circuitQoSMetrics, err := c.GetCircuitQoSMetrics(org.Name)
			if err != nil {
				log.Errorf("error getting circuit QoS metrics from organization %s: %v", org.Name, err)
			} else {
				circuitQoSMetrics = filterAnalyticsMetricsByDevice(circuitQoSMetrics, monitoredDevices, func(m client.CircuitQoSMetrics) string { return m.Site })
				v.metricsSender.SendCircuitQoSMetrics(org.Name, circuitQoSMetrics, deviceNameToIDMap)
			}
		}

		// Collect QoS metrics if enabled
		if *v.config.CollectQoSMetrics {
			qosMetrics, err := c.GetPathQoSMetrics(org.Name)
			if err != nil {
				log.Errorf("error getting QoS metrics from organization %s: %v", org.Name, err)
			} else {
				qosMetrics = filterAnalyticsMetricsByDevice(qosMetrics, monitoredDevices, func(m client.QoSMetrics) string { return m.LocalSiteName })
				v.metricsSender.SendPathQoSMetrics(org.Name, qosMetrics, deviceNameToIDMap)
			}
		}

		// Collect DIA metrics if enabled
		if *v.config.CollectDIAMetrics {
			diaMetrics, err := c.GetDIAMetrics(org.Name)
			if err != nil {
				log.Errorf("error getting DIA metrics from organization %s: %v", org.Name, err)
			} else {
				diaMetrics = filterAnalyticsMetricsByDevice(diaMetrics, monitoredDevices, func(m client.DIAMetrics) string { return m.Site })
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

			analyticsInterfaceMetrics = filterAnalyticsMetricsByDevice(analyticsInterfaceMetrics, monitoredDevices, func(m client.AnalyticsInterfaceMetrics) string { return m.Site })

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
func (v *VersaCheck) Configure(senderManager sender.SenderManager, integrationConfigDigest uint64, rawInstance integration.Data, rawInitConfig integration.Data, source string, provider string) error {

	// Must be called before v.CommonConfigure
	v.BuildID(integrationConfigDigest, rawInstance, rawInitConfig)

	err := v.CommonConfigure(senderManager, rawInitConfig, rawInstance, source, provider)
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
	instanceConfig.CollectCircuitQoSMetrics = boolPointer(false)
	instanceConfig.CollectDIAMetrics = boolPointer(false)
	instanceConfig.CollectSiteMetrics = boolPointer(false)
	instanceConfig.CollectInterfaceMetrics = boolPointer(false)
	instanceConfig.SendInterfaceMetadataFromAnalytics = boolPointer(false)

	err = yaml.Unmarshal(rawInstance, &instanceConfig)
	if err != nil {
		return err
	}
	v.config = instanceConfig

	v.endpointOverrides, err = parseEndpointOverrides(v.config.EndpointOverrides)
	if err != nil {
		return err
	}

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

	// Fail at configure time rather than passing an unknown query type to the Analytics
	// API, where it would surface as an opaque per-call failure on every run.
	switch v.config.AnalyticsQueryType {
	case "", client.QueryTypeTableData, client.QueryTypeTimeseries:
	default:
		return fmt.Errorf("invalid analytics_query_type %q, expected %q or %q",
			v.config.AnalyticsQueryType, client.QueryTypeTableData, client.QueryTypeTimeseries)
	}

	// Versa rejects a gap outside this set, 15MINUTE included. Warn rather than fail,
	// since the accepted set is tenant and version dependent and a hard error would block
	// a value that a given Director does accept.
	switch strings.ToUpper(v.config.AnalyticsGap) {
	case "", "1MINUTE", "1HOUR", "1DAY":
	default:
		log.Warnf("analytics_gap %q is outside the values Versa documents (1MINUTE, 1HOUR, 1DAY) and may be rejected", v.config.AnalyticsGap)
	}

	// Timeseries holds back the newest bucket of every entity, so a window that cannot
	// hold two buckets publishes nothing. The bucket size behind a 1MINUTE gap is tenant
	// dependent (often 15 minutes), so warn conservatively below 30 minutes.
	if v.config.AnalyticsQueryType == client.QueryTypeTimeseries &&
		v.config.LookbackTimeWindowMinutes > 0 && v.config.LookbackTimeWindowMinutes < minTimeseriesLookbackMinutes {
		log.Warnf("lookback_time_window_minutes %d may not span two analytics buckets; timeseries queries hold back the newest bucket and may publish nothing. Use at least %d (45 or more preferred)",
			v.config.LookbackTimeWindowMinutes, minTimeseriesLookbackMinutes)
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

	if v.config.ClientTimeout > 0 {
		clientOptions = append(clientOptions, client.WithTimeout(time.Duration(v.config.ClientTimeout)*time.Second))
	}

	for endpoint, config := range v.endpointOverrides {
		clientOptions = append(clientOptions, client.WithEndpointConfig(endpoint, config))
	}

	if v.config.LookbackTimeWindowMinutes > 0 {
		clientOptions = append(clientOptions, client.WithLookback(v.config.LookbackTimeWindowMinutes))
	}

	if v.config.AnalyticsQueryType != "" {
		clientOptions = append(clientOptions, client.WithAnalyticsQueryType(v.config.AnalyticsQueryType))
	}

	if v.config.AnalyticsGap != "" {
		clientOptions = append(clientOptions, client.WithAnalyticsGap(v.config.AnalyticsGap))
	}

	return clientOptions, nil
}

// parseEndpointOverrides validates the endpoint_overrides config and converts it to client endpoint configs
func parseEndpointOverrides(overrides map[string]endpointOverrideCfg) (map[client.Endpoint]client.EndpointConfig, error) {
	endpointConfigs := make(map[client.Endpoint]client.EndpointConfig, len(overrides))
	for name, override := range overrides {
		endpoint, err := client.ParseEndpoint(name)
		if err != nil {
			return nil, fmt.Errorf("invalid endpoint_overrides: %w", err)
		}
		endpointConfigs[endpoint] = client.EndpointConfig{
			MaxCount: override.MaxCount,
			MaxPages: override.MaxPages,
			Timeout:  time.Duration(override.Timeout) * time.Second,
		}
	}
	return endpointConfigs, nil
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

// filterAppliances filters appliances against the included and excluded device lists,
// matching case-insensitively on the appliance name. When includedDevices is non-empty
// it acts as an opt-in list: only the appliances it names are monitored. excludedDevices
// is applied afterwards, so an appliance named in both is excluded.
//
// Device filtering applies to appliances only. The Director is always collected, since it
// is the endpoint the check authenticates against; use collect_hardware_metrics and
// collect_director_interface_metrics to control what is reported for it.
func filterAppliances(appliances []client.Appliance, includedDevices []string, excludedDevices []string) []client.Appliance {
	if len(includedDevices) == 0 && len(excludedDevices) == 0 {
		return appliances
	}

	includedDevicesSet := make(map[string]struct{}, len(includedDevices))
	for _, device := range includedDevices {
		includedDevicesSet[strings.ToLower(device)] = struct{}{}
	}
	excludedDevicesSet := make(map[string]struct{}, len(excludedDevices))
	for _, device := range excludedDevices {
		excludedDevicesSet[strings.ToLower(device)] = struct{}{}
	}

	filteredAppliances := make([]client.Appliance, 0, len(appliances))
	for _, appliance := range appliances {
		applianceName := strings.ToLower(appliance.Name) // Normalize the appliance name to lowercase
		// If includedDevices is not empty, only include appliances in the list
		if _, ok := includedDevicesSet[applianceName]; len(includedDevices) > 0 && !ok {
			log.Debugf("Skipping appliance %q, not in included_devices", appliance.Name)
			continue
		}
		// If excludedDevices is not empty, exclude appliances in the list
		if _, ok := excludedDevicesSet[applianceName]; ok {
			log.Debugf("Skipping appliance %q, listed in excluded_devices", appliance.Name)
			continue
		}
		filteredAppliances = append(filteredAppliances, appliance)
	}

	return filteredAppliances
}

// includedDeviceNames returns the appliance names listed in included_devices, discarding
// any tags configured alongside them.
func (v *VersaCheck) includedDeviceNames() []string {
	includedDevices := make([]string, 0, len(v.config.IncludedDevices))
	for _, device := range v.config.IncludedDevices {
		includedDevices = append(includedDevices, device.Name)
	}

	return includedDevices
}

// configuredDeviceTags maps a lowercased appliance name to the tags configured for it in
// included_devices, rendered as the "key:value" strings the metadata payload expects.
func (v *VersaCheck) configuredDeviceTags() map[string][]string {
	configuredTags := make(map[string][]string)
	for _, device := range v.config.IncludedDevices {
		if len(device.Tags) == 0 {
			continue
		}

		tags := make([]string, 0, len(device.Tags))
		for key, value := range device.Tags {
			if key == "" {
				log.Warnf("Ignoring tag with an empty key configured for device %q", device.Name)
				continue
			}
			tags = append(tags, key+":"+value)
		}
		// Map iteration order is random, so sort to keep the payload stable across runs
		sort.Strings(tags)

		configuredTags[strings.ToLower(device.Name)] = tags
	}

	return configuredTags
}

// applyConfiguredDeviceTags appends the tags configured in included_devices to the metadata
// of the devices they were configured for, matching case-insensitively on the device name.
func applyConfiguredDeviceTags(deviceMetadata []devicemetadata.DeviceMetadata, configuredTags map[string][]string) {
	if len(configuredTags) == 0 {
		return
	}

	for i := range deviceMetadata {
		tags, ok := configuredTags[strings.ToLower(deviceMetadata[i].Name)]
		if !ok {
			continue
		}
		deviceMetadata[i].Tags = append(deviceMetadata[i].Tags, tags...)
	}
}

// monitoredDeviceNames returns the lowercased names of the appliances that survived device
// filtering, for filtering the data collected downstream of appliance collection. It returns
// nil when neither included_devices nor excluded_devices is configured, which callers treat
// as "no filtering", so that an unconfigured check reports everything as it did before.
//
// Note that this differs from an empty map, which means filtering is on and matched nothing.
func (v *VersaCheck) monitoredDeviceNames(appliances []client.Appliance) map[string]struct{} {
	if len(v.config.IncludedDevices) == 0 && len(v.config.ExcludedDevices) == 0 {
		return nil
	}

	monitoredDevices := make(map[string]struct{}, len(appliances))
	for _, appliance := range appliances {
		monitoredDevices[strings.ToLower(appliance.Name)] = struct{}{}
	}

	return monitoredDevices
}

// filterInterfacesByDevice keeps only the interfaces that belong to a monitored appliance, or
// to the Director, which device filtering does not apply to. A nil monitoredDevices means
// device filtering is disabled and every interface is kept.
func filterInterfacesByDevice(interfaces []client.Interface, monitoredDevices map[string]struct{}, directorName string) []client.Interface {
	if monitoredDevices == nil {
		return interfaces
	}

	allowedDevices := make(map[string]struct{}, len(monitoredDevices)+1)
	for deviceName := range monitoredDevices {
		allowedDevices[deviceName] = struct{}{}
	}
	if directorName != "" {
		allowedDevices[strings.ToLower(directorName)] = struct{}{}
	}

	filteredInterfaces := make([]client.Interface, 0, len(interfaces))
	for _, iface := range interfaces {
		if _, ok := allowedDevices[strings.ToLower(iface.DeviceName)]; !ok {
			continue
		}
		filteredInterfaces = append(filteredInterfaces, iface)
	}

	return filteredInterfaces
}

// filterAnalyticsMetricsByDevice keeps only the Analytics results belonging to a monitored
// device. Analytics queries are scoped to a tenant rather than a device, so every enabled
// Analytics family returns results for the whole organization and has to be narrowed here.
// deviceName extracts the appliance or site name a result is reported against, which varies
// per metric family. A nil monitoredDevices means device filtering is disabled and every
// result is kept.
//
// The Director is deliberately not exempt here, unlike in filterInterfacesByDevice: Analytics
// reports on appliances and sites, so a result matching the Director's name would be an
// appliance that happens to share it rather than the Director itself.
func filterAnalyticsMetricsByDevice[T any](metrics []T, monitoredDevices map[string]struct{}, deviceName func(T) string) []T {
	if monitoredDevices == nil {
		return metrics
	}

	filteredMetrics := make([]T, 0, len(metrics))
	for _, metric := range metrics {
		name := deviceName(metric)
		if _, ok := monitoredDevices[strings.ToLower(name)]; !ok {
			log.Debugf("Skipping analytics result for device %q, not a monitored device", name)
			continue
		}
		filteredMetrics = append(filteredMetrics, metric)
	}

	return filteredMetrics
}

// TODO: should we convert the tags map to use ID instead of IP?
// generateDeviceNameToIPMap generates a map of device IP to device name to enrich the results from Analytics responses
//
// Keys are lowercased, so every lookup must lowercase its device name too. Versa reports
// appliance names and Analytics site names with inconsistent casing for the same device, and
// device filtering already matches case-insensitively; keying this map case-sensitively meant a
// result could pass the filter and then fail the tag lookup, emitting a metric with no device_id.
func generateDeviceNameToIPMap(deviceMetadata []devicemetadata.DeviceMetadata) map[string]string {
	deviceNameToIPMap := make(map[string]string)
	for _, device := range deviceMetadata {
		if device.Name != "" {
			deviceNameToIPMap[strings.ToLower(device.Name)] = device.IPAddress
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
