// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package traps

import (
	"encoding/json"
	"fmt"
	"strings"
	"unicode"

	"github.com/gosnmp/gosnmp"

	"github.com/DataDog/datadog-agent/pkg/util/log"
)

const ddsource string = "snmp-traps"

// Formatter is an interface to extract and format raw SNMP Traps
type Formatter interface {
	FormatPacket(packet *SnmpPacket) ([]byte, error)
}

// JSONFormatter is a Formatter implementation that transforms Traps into JSON
type JSONFormatter struct {
	oidResolver OIDResolver
	namespace   string
}

type trapVariable struct {
	OID     string      `json:"oid"`
	VarType string      `json:"type"`
	Value   interface{} `json:"value"`
}

const (
	sysUpTimeInstanceOID = "1.3.6.1.2.1.1.3.0"
	snmpTrapOID          = "1.3.6.1.6.3.1.1.4.1.0"
)

// NewJSONFormatter creates a new JSONFormatter instance with an optional OIDResolver variable.
func NewJSONFormatter(oidResolver OIDResolver, namespace string) (JSONFormatter, error) {
	if oidResolver == nil {
		return JSONFormatter{}, fmt.Errorf("NewJSONFormatter called with a nil OIDResolver")
	}
	return JSONFormatter{oidResolver, namespace}, nil
}

// FormatPacket converts a raw SNMP trap packet to a FormattedSnmpPacket containing the JSON data and the tags to attach
//
//	{
//		"trap": {
//	   "ddsource": "snmp-traps",
//	   "ddtags": "namespace:default,snmp_device:10.0.0.2,...",
//	   "timestamp": 123456789,
//	   "snmpTrapName": "...",
//	   "snmpTrapOID": "1.3.6.1.5.3.....",
//	   "snmpTrapMIB": "...",
//	   "uptime": "12345",
//	   "genericTrap": "5", # v1 only
//	   "specificTrap": "0",  # v1 only
//	   "variables": [
//	     {
//	       "oid": "1.3.4.1....",
//	       "type": "integer",
//	       "value": 12
//	     },
//	     ...
//	   ],
//	  }
//	}
func (f JSONFormatter) FormatPacket(packet *SnmpPacket) ([]byte, error) {
	payload := make(map[string]interface{})
	var formattedTrap map[string]interface{}
	var err error
	if packet.Content.Version == gosnmp.Version1 {
		formattedTrap = f.formatV1Trap(packet.Content)
	} else {
		formattedTrap, err = f.formatTrap(packet.Content)
		if err != nil {
			return nil, err
		}
	}
	formattedTrap["ddsource"] = ddsource
	formattedTrap["ddtags"] = strings.Join(f.getTags(packet), ",")
	formattedTrap["timestamp"] = packet.Timestamp
	payload["trap"] = formattedTrap
	return json.Marshal(payload)
}

// GetTags returns a list of tags associated to an SNMP trap packet.
func (f JSONFormatter) getTags(packet *SnmpPacket) []string {
	return []string{
		"snmp_version:" + formatVersion(packet.Content),
		"device_namespace:" + f.namespace,
		"snmp_device:" + packet.Addr.IP.String(),
	}
}

func (f JSONFormatter) formatV1Trap(packet *gosnmp.SnmpPacket) map[string]interface{} {
	data := make(map[string]interface{})
	data["uptime"] = uint32(packet.Timestamp)
	enterpriseOid := NormalizeOID(packet.Enterprise)
	genericTrap := packet.GenericTrap
	specificTrap := packet.SpecificTrap
	var trapOID string
	if genericTrap == 6 {
		// Vendor-specific trap
		trapOID = fmt.Sprintf("%s.0.%d", enterpriseOid, specificTrap)
	} else {
		// Generic trap
		trapOID = fmt.Sprintf("%s.%d", genericTrapOid, genericTrap+1)
	}
	data["snmpTrapOID"] = trapOID
	trapMetadata, err := f.oidResolver.GetTrapMetadata(trapOID)
	if err != nil {
		log.Debugf("unable to resolve OID: %s", err)
	} else {
		data["snmpTrapName"] = trapMetadata.Name
		data["snmpTrapMIB"] = trapMetadata.MIBName
	}
	data["enterpriseOID"] = enterpriseOid
	data["genericTrap"] = genericTrap
	data["specificTrap"] = specificTrap
	parsedVariables, enrichedValues := f.parseVariables(trapOID, packet.Variables)
	data["variables"] = parsedVariables
	for key, value := range enrichedValues {
		data[key] = value
	}
	return data
}

func (f JSONFormatter) formatTrap(packet *gosnmp.SnmpPacket) (map[string]interface{}, error) {
	/*
		An SNMP v2 or v3 trap packet consists in the following variables (PDUs):
		{sysUpTime.0, snmpTrapOID.0, additionalDataVariables...}
		See: https://tools.ietf.org/html/rfc3416#section-4.2.6
	*/
	variables := packet.Variables
	if len(variables) < 2 {
		return nil, fmt.Errorf("expected at least 2 variables, got %d", len(variables))
	}

	data := make(map[string]interface{})

	uptime, err := parseSysUpTime(variables[0])
	if err != nil {
		return nil, err
	}
	data["uptime"] = uptime

	trapOID, err := parseSnmpTrapOID(variables[1])
	if err != nil {
		return nil, err
	}
	data["snmpTrapOID"] = trapOID

	trapMetadata, err := f.oidResolver.GetTrapMetadata(trapOID)
	if err != nil {
		log.Debugf("unable to resolve OID: %s", err)
	} else {
		data["snmpTrapName"] = trapMetadata.Name
		data["snmpTrapMIB"] = trapMetadata.MIBName
	}

	parsedVariables, enrichedValues := f.parseVariables(trapOID, variables[2:])
	data["variables"] = parsedVariables
	for key, value := range enrichedValues {
		data[key] = value
	}
	return data, nil
}

// NormalizeOID convert an OID from the absolute form ".1.2.3..." to a relative form "1.2.3..."
func NormalizeOID(value string) string {
	// OIDs can be formatted as ".1.2.3..." ("absolute form") or "1.2.3..." ("relative form").
	// Convert everything to relative form, like we do in the Python check.
	return strings.TrimLeft(value, ".")
}

// IsValidOID returns true if a looks like a valid OID.
// An OID is made of digits and dots, but OIDs do not end with a dot and there are always
// digits between dots.
func IsValidOID(value string) bool {
	var previousChar rune
	for _, char := range value {
		if char != '.' && !unicode.IsDigit(char) {
			return false
		}
		if char == '.' && previousChar == '.' {
			return false
		}
		previousChar = char
	}
	return previousChar != '.'
}

// enrichEnum checks to see if the variable has a mapping in an enum and
// returns the mapping if it exists, otherwise returns the value unchanged
func enrichEnum(variable trapVariable, varMetadata VariableMetadata) interface{} {
	// if we find a mapping set it and return
	i, ok := variable.Value.(int)
	if !ok {
		log.Warnf("unable to enrich variable %q %s with integer enum, received value was not int, was %T", varMetadata.Name, variable.OID, variable.Value)
		return variable.Value
	}
	if value, ok := varMetadata.Enumeration[i]; ok {
		return value
	}

	// if no mapping is found or type is not integer
	log.Debugf("unable to find enum mapping for value %d variable %q", i, varMetadata.Name)
	return variable.Value
}

// enrichBits checks to see if the variable has a mapping in bits and
// returns the mapping if it exists, otherwise returns the value unchanged
func enrichBits(variable trapVariable, varMetadata VariableMetadata) interface{} {
	// do bitwise search
	bytes, ok := variable.Value.([]byte)
	if !ok {
		log.Warnf("unable to enrich variable %q %s with BITS mapping, received value was not []byte, was %T", varMetadata.Name, variable.OID, variable.Value)
		return variable.Value
	}
	enabledValues := make([]interface{}, 0)
	for i, b := range bytes {
		for j := 0; j < 8; j++ {
			position := j + i*8 // position is the index in the current byte plus 8 * the position in the byte array
			enabled, err := isBitEnabled(uint8(b), j)
			if err != nil {
				log.Debugf("unable to determine status at position %d: %s", position, err.Error())
				continue
			}
			if enabled {
				if value, ok := varMetadata.Bits[position]; !ok {
					log.Debugf("unable to find enum mapping for value %d variable %q", i, varMetadata.Name)
					enabledValues = append(enabledValues, position)
				} else {
					enabledValues = append(enabledValues, value)
				}
			}
		}
	}

	return enabledValues
}

func parseSysUpTime(variable gosnmp.SnmpPDU) (uint32, error) {
	name := NormalizeOID(variable.Name)
	if name != sysUpTimeInstanceOID {
		return 0, fmt.Errorf("expected OID %s, got %s", sysUpTimeInstanceOID, name)
	}

	value, ok := variable.Value.(uint32)
	if !ok {
		return 0, fmt.Errorf("expected uptime to be uint32 (got %v of type %T)", variable.Value, variable.Value)
	}

	return value, nil
}

func parseSnmpTrapOID(variable gosnmp.SnmpPDU) (string, error) {
	name := NormalizeOID(variable.Name)
	if name != snmpTrapOID {
		return "", fmt.Errorf("expected OID %s, got %s", snmpTrapOID, name)
	}

	value := ""
	switch variable.Value.(type) {
	case string:
		value = variable.Value.(string)
	case []byte:
		value = string(variable.Value.([]byte))
	default:
		return "", fmt.Errorf("expected snmpTrapOID to be a string (got %v of type %T)", variable.Value, variable.Value)
	}

	return NormalizeOID(value), nil
}

func (f JSONFormatter) parseVariables(trapOID string, variables []gosnmp.SnmpPDU) ([]trapVariable, map[string]interface{}) {
	var parsedVariables []trapVariable
	enrichedValues := make(map[string]interface{})

	for _, variable := range variables {
		varOID := NormalizeOID(variable.Name)
		varType := formatType(variable)

		tv := trapVariable{
			OID:     varOID,
			VarType: varType,
			Value:   variable.Value,
		}

		varMetadata, err := f.oidResolver.GetVariableMetadata(trapOID, varOID)
		if err != nil {
			log.Debugf("unable to enrich variable: %s", err)
			tv.Value = formatValue(variable)
			parsedVariables = append(parsedVariables, tv)
			continue
		}

		if len(varMetadata.Enumeration) > 0 && len(varMetadata.Bits) > 0 {
			log.Errorf("Unable to enrich variable, trap variable %q has mappings for both integer enum and bits.", varMetadata.Name)
		} else if len(varMetadata.Enumeration) > 0 {
			enrichedValues[varMetadata.Name] = enrichEnum(tv, varMetadata)
		} else if len(varMetadata.Bits) > 0 {
			enrichedValues[varMetadata.Name] = enrichBits(tv, varMetadata)
		} else {
			// only format the value if it's not an enum type
			tv.Value = formatValue(variable)
			enrichedValues[varMetadata.Name] = tv.Value
		}

		parsedVariables = append(parsedVariables, tv)
	}

	return parsedVariables, enrichedValues
}

func formatType(variable gosnmp.SnmpPDU) string {
	switch variable.Type {
	case gosnmp.Integer, gosnmp.Uinteger32:
		return "integer"
	case gosnmp.OctetString, gosnmp.BitString:
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

func formatValue(variable gosnmp.SnmpPDU) interface{} {
	switch variable.Value.(type) {
	case []byte:
		return string(variable.Value.([]byte))
	case string:
		if variable.Type == gosnmp.ObjectIdentifier {
			return NormalizeOID(variable.Value.(string))
		}
		return variable.Value
	default:
		return variable.Value
	}
}

func formatVersion(packet *gosnmp.SnmpPacket) string {
	switch packet.Version {
	case gosnmp.Version3:
		return "3"
	case gosnmp.Version2c:
		return "2"
	case gosnmp.Version1:
		return "1"
	default:
		return "unknown"
	}
}

// isBitEnabled takes in a uint8 and returns true if
// the bit at the passed position is 1.
// Each byte is little endian meaning if
// you have the binary 10000000, passing position 0
// would return true and 7 would return false
func isBitEnabled(n uint8, pos int) (bool, error) {
	if pos < 0 || pos > 7 {
		return false, fmt.Errorf("invalid position %d, must be 0-7", pos)
	}
	val := n & uint8(1<<(7-pos))
	return val > 0, nil
}
