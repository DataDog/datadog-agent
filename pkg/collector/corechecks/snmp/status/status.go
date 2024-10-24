// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

// Package status contains the SNMP Profiles status provider
package status

import (
	"embed"
	"encoding/json"
	"expvar"
	"io"
	"net"
	"strings"

	"github.com/DataDog/datadog-agent/comp/core/status"
)

//go:embed status_templates
var templatesFS embed.FS

// Provider provides the functionality to populate the status output
type Provider struct{}

// Name returns the name
func (Provider) Name() string {
	return "SNMP"
}

// Section return the section
func (Provider) Section() string {
	return "SNMP"
}

func (p Provider) getStatusInfo() map[string]interface{} {
	stats := make(map[string]interface{})

	p.populateStatus(stats)

	return stats
}

func (Provider) populateStatus(stats map[string]interface{}) {
	profiles := make(map[string]string)

	snmpProfileErrorsVar := expvar.Get("snmpProfileErrors")
	if snmpProfileErrorsVar != nil {
		snmpProfileErrorsJSON := []byte(snmpProfileErrorsVar.String())
		json.Unmarshal(snmpProfileErrorsJSON, &profiles) //nolint:errcheck
		stats["snmpProfiles"] = profiles
	}

	autodiscoveryVar := expvar.Get("snmpAutodiscovery")

	if autodiscoveryVar != nil {
		stats["autodiscoverySubnets"] = getSubnetsStatus(autodiscoveryVar)
	}

	discoveryVar := expvar.Get("snmpDiscovery")

	if discoveryVar != nil {
		stats["discoverySubnets"] = getSubnetsStatus(discoveryVar)
	}
}

type subnetStatus struct {
	Subnet         string
	ConfigHash     string
	DeviceScanning string
	DevicesScanned int
	IpsCount       int
	DevicesFound   string
}

func getSubnetsStatus(discoveryVar expvar.Var) map[string]subnetStatus {
	discoverySubnets := make(map[string]map[string]interface{})
	discoveryJSON := []byte(discoveryVar.String())
	json.Unmarshal(discoveryJSON, &discoverySubnets) //nolint:errcheck

	devicesScannedInSubnet := discoverySubnets["devicesScannedInSubnet"]
	devicesFoundInSubnet := discoverySubnets["devicesFoundInSubnet"]
	deviceScanningInSubnet := discoverySubnets["deviceScanningInSubnet"]

	discoverySubnetsStatus := make(map[string]subnetStatus)

	for subnetKey, devicesScanned := range devicesScannedInSubnet {
		subnet, configHash := strings.Split(subnetKey, "|")[0], strings.Split(subnetKey, "|")[1]
		_, ipNet, _ := net.ParseCIDR(subnet)

		ones, bits := ipNet.Mask.Size()
		ipsCount := 1 << (bits - ones)
		devicesScannedCount := int(devicesScanned.(float64))

		discoverySubnetsStatus[subnetKey] = subnetStatus{subnet, configHash, deviceScanningInSubnet[subnetKey].(string), devicesScannedCount, ipsCount, ""}
	}

	for subnetKey, devicesFound := range devicesFoundInSubnet {
		devices := devicesFound.(string)
		devicesList := strings.Split(devices, "|")

		status, statusFound := discoverySubnetsStatus[subnetKey]
		if statusFound {
			status.DevicesFound = strings.Join(devicesList, ", ")
			discoverySubnetsStatus[subnetKey] = status
		}
	}

	return discoverySubnetsStatus
}

// JSON populates the status map
func (p Provider) JSON(_ bool, stats map[string]interface{}) error {
	p.populateStatus(stats)

	return nil
}

// Text renders the text output
func (p Provider) Text(_ bool, buffer io.Writer) error {
	return status.RenderText(templatesFS, "snmp.tmpl", buffer, p.getStatusInfo())
}

// HTML renders the html output
func (p Provider) HTML(_ bool, buffer io.Writer) error {
	return status.RenderHTML(templatesFS, "snmpHTML.tmpl", buffer, p.getStatusInfo())
}
