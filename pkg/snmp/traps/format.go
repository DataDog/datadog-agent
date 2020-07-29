// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://wwt.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

package traps

import (
	"fmt"
	"strings"

	"github.com/soniah/gosnmp"
)

const (
	sysUpTimeOID         = ".1.3.6.1.2.1.1.3"
	sysUpTimeInstanceOID = ".1.3.6.1.2.1.1.3.0"
	snmpTrapOID          = ".1.3.6.1.6.3.1.1.4.1"
	snmpTrapInstanceOID  = ".1.3.6.1.6.3.1.1.4.1.0"
)

// NOTE: This module is used by the traps logs input.

// FormatJSON converts an SNMP trap packet to a JSON-serializable object.
func FormatJSON(p *SnmpPacket) (map[string]interface{}, error) {
	return formatTrapPDUs(p.Content.Variables)
}

// GetTags returns a list of tags associated to an SNMP trap packet.
func GetTags(p *SnmpPacket) []string {
	tags := []string{
		fmt.Sprintf("community:%s", p.Content.Community),
		fmt.Sprintf("device_ip:%s", p.Addr.IP.String()),
		fmt.Sprintf("device_port:%d", p.Addr.Port),
	}
	if version := formatVersion(p); version != "" {
		tags = append(tags, fmt.Sprintf("snmp_version:%s", version))
	}
	return tags
}

func formatVersion(p *SnmpPacket) string {
	switch p.Content.Version {
	case gosnmp.Version2c:
		return "2"
	default:
		return "" // Unsupported.
	}
}

func formatTrapPDUs(vars []gosnmp.SnmpPDU) (map[string]interface{}, error) {
	/*
		An SNMPv2 trap packet consists in the following variables (PDUs):
		{sysUpTime.0, snmpTrapOID.0, additionalDataVariables...}
		See: https://tools.ietf.org/html/rfc3416#section-4.2.6
	*/
	if len(vars) < 2 {
		return nil, fmt.Errorf("expected at least 2 variables, got %d", len(vars))
	}

	data := make(map[string]interface{})

	uptime, err := parseSysUpTime(vars[0])
	if err != nil {
		return nil, err
	}
	data["uptime"] = uptime

	trapOID, err := parseSnmpTrapOID(vars[1])
	if err != nil {
		return nil, err
	}
	data["oid"] = trapOID

	data["variables"] = parseVariables(vars[2:])

	return data, nil
}

func normalizeOID(value string) string {
	return strings.TrimLeft(value, ".")
}

func parseSysUpTime(v gosnmp.SnmpPDU) (uint32, error) {
	if v.Name != sysUpTimeOID && v.Name != sysUpTimeInstanceOID {
		return 0, fmt.Errorf("expected OID %s or %s, got %s", sysUpTimeOID, sysUpTimeInstanceOID, v.Name)
	}

	value, ok := v.Value.(uint32)
	if !ok {
		return 0, fmt.Errorf("expected uptime to be uint32 (got %v of type %T)", v.Value, v.Value)
	}

	return value, nil
}

func parseSnmpTrapOID(v gosnmp.SnmpPDU) (string, error) {
	if v.Name != snmpTrapOID && v.Name != snmpTrapInstanceOID {
		return "", fmt.Errorf("expected OID %s or %s, got %s", snmpTrapOID, snmpTrapInstanceOID, v.Name)
	}

	value := ""
	switch v.Value.(type) {
	case string:
		value = v.Value.(string)
	case []byte:
		value = string(v.Value.([]byte))
	default:
		return "", fmt.Errorf("expected snmpTrapOID to be a string (got %v of type %T)", v.Value, v.Value)
	}

	return normalizeOID(value), nil
}

func parseVariables(vars []gosnmp.SnmpPDU) []map[string]interface{} {
	var variables []map[string]interface{}

	for _, v := range vars {
		variable := make(map[string]interface{})
		variable["oid"] = normalizeOID(v.Name)
		variable["type"] = formatType(v)
		variable["value"] = formatValue(v)
		variables = append(variables, variable)
	}

	return variables
}

func formatType(v gosnmp.SnmpPDU) string {
	switch v.Type {
	case gosnmp.Integer, gosnmp.Uinteger32:
		return "integer"
	case gosnmp.OctetString:
		return "string"
	case gosnmp.ObjectIdentifier:
		return "oid"
	case gosnmp.Counter32:
		return "counter32"
	case gosnmp.Counter64:
		return "counter64"
	case gosnmp.Gauge32:
		return "gauge32"
	default:
		return "other"
	}
}

func formatValue(v gosnmp.SnmpPDU) interface{} {
	switch v.Value.(type) {
	case []byte:
		return string(v.Value.([]byte))
	default:
		return v.Value
	}
}
