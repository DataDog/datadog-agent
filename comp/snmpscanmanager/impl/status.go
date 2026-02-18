// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package snmpscanmanagerimpl

import (
	"embed"
	"io"
	"slices"

	"github.com/DataDog/datadog-agent/comp/core/status"
)

//go:embed status_templates
var templatesFS embed.FS

// Name returns the name
func (m *snmpScanManagerImpl) Name() string {
	return "SNMP"
}

// Section returns the section
func (m *snmpScanManagerImpl) Section() string {
	return "SNMP"
}

// JSON populates the status map
func (m *snmpScanManagerImpl) JSON(_ bool, stats map[string]interface{}) error {
	m.populateStatus(stats)

	return nil
}

// Text renders the text output
func (m *snmpScanManagerImpl) Text(_ bool, buffer io.Writer) error {
	return status.RenderText(templatesFS, "snmpscanmanager.tmpl", buffer, m.getStatusInfo())
}

// HTML renders the html output
func (m *snmpScanManagerImpl) HTML(_ bool, buffer io.Writer) error {
	return status.RenderHTML(templatesFS, "snmpscanmanagerHTML.tmpl", buffer, m.getStatusInfo())
}

func (m *snmpScanManagerImpl) populateStatus(stats map[string]interface{}) {
	deviceScans := m.cloneDeviceScans()

	pendingScansCount := len(m.scanQueue)

	successScansCount := 0
	failedScansIPs := make([]string, 0)
	for _, scan := range deviceScans {
		if scan.isSuccess() {
			successScansCount++
		}
		if scan.isFailed() {
			failedScansIPs = append(failedScansIPs, scan.DeviceIP)
		}
	}
	slices.Sort(failedScansIPs)

	stats["pendingScanCount"] = pendingScansCount
	stats["successScanCount"] = successScansCount
	stats["failedScanIPs"] = failedScansIPs
}

func (m *snmpScanManagerImpl) getStatusInfo() map[string]interface{} {
	stats := make(map[string]interface{})

	m.populateStatus(stats)

	return stats
}
