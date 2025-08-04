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
	"sort"
	"strconv"
	"strings"

	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/listeners"
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
		autodiscoverySubnets := getSubnetsStatus(autodiscoveryVar)
		sort.Slice(autodiscoverySubnets, func(i1, i2 int) bool {
			return autodiscoverySubnets[i1].subnetIndex < autodiscoverySubnets[i2].subnetIndex
		})

		stats["autodiscoverySubnets"] = autodiscoverySubnets
	}

	// subnets configured in the snmp.d can not be ordered here as we can't retrieve the config here
	discoveryVar := expvar.Get("snmpDiscovery")

	if discoveryVar != nil {
		stats["discoverySubnets"] = getSubnetsStatus(discoveryVar)
	}
}

type subnetStatus struct {
	Subnet         string
	DeviceScanning string
	DevicesScanned int
	IpsCount       int
	DevicesFound   []string
	configHash     string
	subnetIndex    int
}

func getSubnetsStatus(discoveryVar expvar.Var) []subnetStatus {
	discoverySubnets := make(map[string]listeners.AutodiscoveryStatus)
	discoveryJSON := []byte(discoveryVar.String())
	json.Unmarshal(discoveryJSON, &discoverySubnets) //nolint:errcheck

	discoverySubnetsStatus := []subnetStatus{}

	for subnetKey, autodiscoveryStatus := range discoverySubnets {
		subnetParts := strings.Split(subnetKey, "|")
		subnet := subnetParts[0]
		configHash := subnetParts[1]

		subnetIndex := 0
		if len(subnetParts) > 2 {
			indexString := subnetParts[2]
			index, err := strconv.Atoi(indexString)
			if err == nil {
				subnetIndex = index
			}
		}

		_, ipNet, _ := net.ParseCIDR(subnet)

		ones, bits := ipNet.Mask.Size()
		ipsTotalCount := 1 << (bits - ones)
		discoverySubnetsStatus = append(discoverySubnetsStatus, subnetStatus{subnet,
			autodiscoveryStatus.CurrentDevice,
			autodiscoveryStatus.DevicesScannedCount,
			ipsTotalCount,
			autodiscoveryStatus.DevicesFoundList,
			configHash,
			subnetIndex,
		})
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
