// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

// Package client implements a Versa API client
package client

import (
	"fmt"
)

// parseSLAMetrics parses the raw AaData response into SLAMetrics structs
func parseSLAMetrics(data [][]interface{}) ([]SLAMetrics, error) {
	var rows []SLAMetrics
	for _, row := range data {
		m := SLAMetrics{}
		if len(row) != 12 {
			return nil, fmt.Errorf("expected 12 columns, got %d", len(row))
		}
		// Type assertions for each value
		var ok bool
		if m.DrillKey, ok = row[0].(string); !ok {
			return nil, fmt.Errorf("expected string for CombinedKey")
		}
		if m.LocalSite, ok = row[1].(string); !ok {
			return nil, fmt.Errorf("expected string for LocalSite")
		}
		if m.RemoteSite, ok = row[2].(string); !ok {
			return nil, fmt.Errorf("expected string for RemoteSite")
		}
		if m.LocalAccessCircuit, ok = row[3].(string); !ok {
			return nil, fmt.Errorf("expected string for LocalCircuit")
		}
		if m.RemoteAccessCircuit, ok = row[4].(string); !ok {
			return nil, fmt.Errorf("expected string for RemoteCircuit")
		}
		if m.ForwardingClass, ok = row[5].(string); !ok {
			return nil, fmt.Errorf("expected string for ForwardingClass")
		}

		// Floats from index 6–11
		floatFields := []*float64{
			&m.Delay, &m.FwdDelayVar, &m.RevDelayVar,
			&m.FwdLossRatio, &m.RevLossRatio, &m.PDULossRatio,
		}
		for i, ptr := range floatFields {
			if val, ok := row[i+6].(float64); ok {
				*ptr = val
			} else {
				return nil, fmt.Errorf("expected float64 at index %d", i+6)
			}
		}
		rows = append(rows, m)
	}
	return rows, nil
}

// parseApplicationsByApplianceMetrics parses the raw AaData response into ApplicationsByApplianceMetrics structs
func parseApplicationsByApplianceMetrics(data [][]interface{}) ([]ApplicationsByApplianceMetrics, error) {
	var rows []ApplicationsByApplianceMetrics
	for _, row := range data {
		m := ApplicationsByApplianceMetrics{}
		if len(row) != 9 {
			return nil, fmt.Errorf("expected 9 columns, got %d", len(row))
		}
		// Type assertions for each value
		var ok bool
		if m.DrillKey, ok = row[0].(string); !ok {
			return nil, fmt.Errorf("expected string for DrillKey")
		}
		if m.Site, ok = row[1].(string); !ok {
			return nil, fmt.Errorf("expected string for Site")
		}
		if m.AppID, ok = row[2].(string); !ok {
			return nil, fmt.Errorf("expected string for AppId")
		}

		// Floats from index 3–8
		floatFields := []*float64{
			&m.Sessions, &m.VolumeTx, &m.VolumeRx,
			&m.BandwidthTx, &m.BandwidthRx, &m.Bandwidth,
		}
		for i, ptr := range floatFields {
			if val, ok := row[i+3].(float64); ok {
				*ptr = val
			} else {
				return nil, fmt.Errorf("expected float64 at index %d", i+3)
			}
		}
		rows = append(rows, m)
	}
	return rows, nil
}

// parseLinkStatusMetrics parses the raw AaData response into LinkStatusMetrics structs
func parseLinkStatusMetrics(data [][]interface{}) ([]LinkStatusMetrics, error) {
	var rows []LinkStatusMetrics
	for _, row := range data {
		m := LinkStatusMetrics{}
		if len(row) != 4 {
			return nil, fmt.Errorf("missing columns in row: got %d columns, expected 4", len(row))
		}
		// Type assertions for each value
		var ok bool
		if m.DrillKey, ok = row[0].(string); !ok {
			return nil, fmt.Errorf("expected string for DrillKey")
		}
		if m.Site, ok = row[1].(string); !ok {
			return nil, fmt.Errorf("expected string for Site")
		}
		if m.AccessCircuit, ok = row[2].(string); !ok {
			return nil, fmt.Errorf("expected string for AccessCircuit")
		}
		if m.Availability, ok = row[3].(float64); !ok {
			return nil, fmt.Errorf("expected float64 for Availability")
		}
		rows = append(rows, m)
	}
	return rows, nil
}

// parseLinkUsageMetrics parses the raw AaData response into LinkUsageMetrics structs
func parseLinkUsageMetrics(data [][]interface{}) ([]LinkUsageMetrics, error) {
	var rows []LinkUsageMetrics
	for _, row := range data {
		m := LinkUsageMetrics{}
		if len(row) != 13 {
			return nil, fmt.Errorf("missing columns in row: got %d columns, expected 13", len(row))
		}
		// Type assertions for each value
		var ok bool
		if m.DrillKey, ok = row[0].(string); !ok {
			return nil, fmt.Errorf("expected string for DrillKey")
		}
		if m.Site, ok = row[1].(string); !ok {
			return nil, fmt.Errorf("expected string for Site")
		}
		if m.AccessCircuit, ok = row[2].(string); !ok {
			return nil, fmt.Errorf("expected string for AccessCircuit")
		}
		if m.UplinkBandwidth, ok = row[3].(string); !ok {
			return nil, fmt.Errorf("expected string for UplinkBandwidth")
		}
		if m.DownlinkBandwidth, ok = row[4].(string); !ok {
			return nil, fmt.Errorf("expected string for DownlinkBandwidth")
		}
		if m.Type, ok = row[5].(string); !ok {
			return nil, fmt.Errorf("expected string for Type")
		}
		if m.Media, ok = row[6].(string); !ok {
			return nil, fmt.Errorf("expected string for Media")
		}
		if m.IP, ok = row[7].(string); !ok {
			return nil, fmt.Errorf("expected string for IP")
		}
		if m.ISP, ok = row[8].(string); !ok {
			return nil, fmt.Errorf("expected string for ISP")
		}

		// Floats from index 9–12
		floatFields := []*float64{
			&m.VolumeTx, &m.VolumeRx, &m.BandwidthTx, &m.BandwidthRx,
		}
		for i, ptr := range floatFields {
			if val, ok := row[i+9].(float64); ok {
				*ptr = val
			} else {
				return nil, fmt.Errorf("expected float64 at index %d", i+9)
			}
		}
		rows = append(rows, m)
	}
	return rows, nil
}

// parseTopUserMetrics parses the raw AaData response into TopUser structs
// TODO: can I use a shared struct for the response for application metrics?
func parseTopUserMetrics(data [][]interface{}) ([]TopUserMetrics, error) {
	var rows []TopUserMetrics
	for _, row := range data {
		m := TopUserMetrics{}
		if len(row) != 9 {
			return nil, fmt.Errorf("expected 9 columns, got %d", len(row))
		}
		// Type assertions for each value
		var ok bool
		if m.DrillKey, ok = row[0].(string); !ok {
			return nil, fmt.Errorf("expected string for DrillKey")
		}
		if m.Site, ok = row[1].(string); !ok {
			return nil, fmt.Errorf("expected string for Site")
		}
		if m.User, ok = row[2].(string); !ok {
			return nil, fmt.Errorf("expected string for User")
		}

		// Floats from index 3–8
		floatFields := []*float64{
			&m.Sessions, &m.VolumeTx, &m.VolumeRx,
			&m.BandwidthTx, &m.BandwidthRx, &m.Bandwidth,
		}
		for i, ptr := range floatFields {
			if val, ok := row[i+3].(float64); ok {
				*ptr = val
			} else {
				return nil, fmt.Errorf("expected float64 at index %d", i+3)
			}
		}
		rows = append(rows, m)
	}
	return rows, nil
}

// parseTunnelMetrics parses the raw AaData response into TunnelMetrics structs
func parseTunnelMetrics(data [][]interface{}) ([]TunnelMetrics, error) {
	var rows []TunnelMetrics
	for _, row := range data {
		m := TunnelMetrics{}
		// Based on the new structure, we expect 7 columns
		if len(row) != 7 {
			return nil, fmt.Errorf("expected 7 columns, got %d", len(row))
		}
		// Type assertions for each value
		var ok bool
		if m.DrillKey, ok = row[0].(string); !ok {
			return nil, fmt.Errorf("expected string for DrillKey")
		}
		if m.Appliance, ok = row[1].(string); !ok {
			return nil, fmt.Errorf("expected string for Appliance")
		}
		if m.LocalIP, ok = row[2].(string); !ok {
			return nil, fmt.Errorf("expected string for LocalIP")
		}
		if m.RemoteIP, ok = row[3].(string); !ok {
			return nil, fmt.Errorf("expected string for RemoteIP")
		}
		if m.VpnProfName, ok = row[4].(string); !ok {
			return nil, fmt.Errorf("expected string for VpnProfName")
		}

		// Handle float metrics from indices 5-6
		if val, ok := row[5].(float64); ok {
			m.VolumeRx = val
		} else {
			return nil, fmt.Errorf("expected float64 for VolumeRx at index 5")
		}
		if val, ok := row[6].(float64); ok {
			m.VolumeTx = val
		} else {
			return nil, fmt.Errorf("expected float64 for VolumeTx at index 6")
		}
		rows = append(rows, m)
	}
	return rows, nil
}
