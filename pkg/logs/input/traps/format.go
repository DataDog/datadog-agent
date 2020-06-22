// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://wwt.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

package traps

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/DataDog/datadog-agent/pkg/snmp/traps"
	"github.com/soniah/gosnmp"
)

const (
	sysUpTimeInstance = ".1.3.6.1.2.1.1.3.0"
	snmpTrapOID       = ".1.3.6.1.6.3.1.1.4.1.0"
)

// FormatPacketJSON converts an SNMP trap packet to a binary JSON log message content.
func FormatPacketJSON(p *traps.SnmpPacket) ([]byte, error) {
	// A trap PDU is composed of a list of variables with the following contents:
	// [sysUpTime.0, snmpTrapOID.0, additionalVariables...]
	// See: https://tools.ietf.org/html/rfc3416#section-4.2.6

	if len(p.Content.Variables) < 2 {
		return nil, fmt.Errorf("expected at least 2 variables, got %d", len(p.Content.Variables))
	}

	trap, err := parseTrap(p)
	if err != nil {
		return nil, err
	}

	return json.Marshal(trap)
}

// FormatPacketTags returns a list of tags associated to an SNMP trap packet.
func FormatPacketTags(p *traps.SnmpPacket) []string {
	tags := []string{
		fmt.Sprintf("device_port:%d", p.Addr.Port),
		fmt.Sprintf("device_ip:%s", p.Addr.IP.String()),
		fmt.Sprintf("snmp_version:%s", traps.VersionAsString(p.Content.Version)),
	}
	if p.Content.Community != "" {
		tags = append(tags, fmt.Sprintf("community:%s", p.Content.Community))
	}
	return tags
}

func normalizeOID(value string) string {
	return strings.TrimLeft(value, ".")
}

func parseTrap(p *traps.SnmpPacket) (map[string]interface{}, error) {
	data := make(map[string]interface{})

	uptime, err := parseSysUpTime(p.Content)
	if err != nil {
		return nil, err
	}
	data["uptime"] = uptime

	trapOID, err := parseSnmpTrapOID(p.Content)
	if err != nil {
		return nil, err
	}
	data["oid"] = trapOID

	data["variables"] = parseVariables(p.Content)

	return data, nil
}

func parseSysUpTime(p *gosnmp.SnmpPacket) (uint32, error) {
	v := p.Variables[0]

	if v.Type != gosnmp.TimeTicks {
		return 0, fmt.Errorf("expected %v, got %v", gosnmp.TimeTicks, v.Type)
	}

	if v.Name != sysUpTimeInstance {
		return 0, fmt.Errorf("expected OID %s, got %s", sysUpTimeInstance, v.Name)
	}

	value, ok := v.Value.(uint32)
	if !ok {
		return 0, fmt.Errorf("expected uptime to be uint32 (got %T)", v.Value)
	}

	// sysUpTimeInstance is given in hundreds of a second.
	return value * 100, nil
}

func parseSnmpTrapOID(p *gosnmp.SnmpPacket) (string, error) {
	v := p.Variables[1]

	if v.Type != gosnmp.ObjectIdentifier {
		return "", fmt.Errorf("expected %v, got %v", gosnmp.ObjectIdentifier, v.Type)
	}

	if v.Name != snmpTrapOID {
		return "", fmt.Errorf("expected OID %s, got %s", snmpTrapOID, v.Name)
	}

	value, ok := v.Value.(string)
	if !ok {
		return "", fmt.Errorf("expected snmpTrapOID to be a string (got %T)", v.Value)
	}

	return normalizeOID(value), nil
}

func parseVariables(p *gosnmp.SnmpPacket) []map[string]interface{} {
	var variables []map[string]interface{}

	for _, v := range p.Variables[2:] {
		variable := make(map[string]interface{})
		variable["name"] = normalizeOID(v.Name)
		variable["type"] = formatType(v)
		variable["value"] = formatValue(v)
		variables = append(variables, variable)
	}

	return variables
}

func formatType(v gosnmp.SnmpPDU) string {
	switch v.Type {
	case gosnmp.Integer, gosnmp.Uinteger32:
		return "int"
	case gosnmp.OctetString:
		return "string"
	case gosnmp.ObjectIdentifier:
		return "oid"
	case gosnmp.Counter32, gosnmp.Counter64:
		return "counter"
	case gosnmp.Gauge32:
		return "gauge"
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
