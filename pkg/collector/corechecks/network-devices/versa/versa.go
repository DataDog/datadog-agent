// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

// Package versa implements NDM Versa corecheck
package versa

import (
	"time"

	yaml "gopkg.in/yaml.v2"

	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/integration"
	"github.com/DataDog/datadog-agent/pkg/aggregator/sender"
	"github.com/DataDog/datadog-agent/pkg/collector/check"
	core "github.com/DataDog/datadog-agent/pkg/collector/corechecks"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/network-devices/versa/report"
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
	Name                            string `yaml:"name"` // TODO: remove this field, only added it for testing
	Namespace                       string `yaml:"namespace"`
	SendNDMMetadata                 *bool  `yaml:"send_ndm_metadata"`
	MinCollectionInterval           int    `yaml:"min_collection_interval"`
	CollectHardwareMetrics          *bool  `yaml:"collect_hardware_metrics"`
	CollectInterfaceMetrics         *bool  `yaml:"collect_interface_metrics"`
	CollectTunnelMetrics            *bool  `yaml:"collect_tunnel_metrics"`
	CollectControlConnectionMetrics *bool  `yaml:"collect_control_connection_metrics"`
	CollectOMPPeerMetrics           *bool  `yaml:"collect_omp_peer_metrics"`
	CollectDeviceCountersMetrics    *bool  `yaml:"collect_device_counters_metrics"`
	CollectBFDSessionStatus         *bool  `yaml:"collect_bfd_session_status"`
	CollectHardwareStatus           *bool  `yaml:"collect_hardware_status"`
	CollectCloudApplicationsMetrics *bool  `yaml:"collect_cloud_applications_metrics"`
	CollectBGPNeighborStates        *bool  `yaml:"collect_bgp_neighbor_states"`
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

	// Commit
	//v.metricsSender.Commit()

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
