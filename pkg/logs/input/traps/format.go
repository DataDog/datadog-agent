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
	if len(p.Variables) < 2 {
		return nil, fmt.Errorf("expected at least 2 variables, got %d", len(p.Variables))
	}

	data := make(map[string]interface{})

	// TODO sender_ip

	trap, err := getTrap(p)
	if err != nil {
		return nil, err
	}
	data["trap"] = trap

	return json.Marshal(data)
}

func normalizeOID(value string) string {
	return strings.TrimLeft(value, ".")
}

func getTrap(p *traps.SnmpPacket) (map[string]interface{}, error) {
	data := make(map[string]interface{})

	uptime, err := getUptime(p)
	if err != nil {
		return nil, err
	}
	data["uptime"] = uptime

	oid, err := getTrapOID(p)
	if err != nil {
		return nil, err
	}
	data["oid"] = oid

	data["objects"] = getObjects(p)

	return data, nil
}

func getUptime(p *traps.SnmpPacket) (uint32, error) {
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

func getTrapOID(p *traps.SnmpPacket) (string, error) {
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

func getObjects(p *traps.SnmpPacket) []map[string]interface{} {
	var objects []map[string]interface{}

	for _, v := range p.Variables[2:] {
		obj := make(map[string]interface{})
		obj["name"] = normalizeOID(v.Name)
		obj["type"] = formatType(v)
		obj["value"] = formatValue(v)
		objects = append(objects, obj)
	}

	return objects
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
		return "count"
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
