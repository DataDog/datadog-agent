package gosnmplib

import (
	"encoding/json"
	"fmt"

	"github.com/gosnmp/gosnmp"

	"github.com/DataDog/datadog-agent/pkg/util/log"
)

type debugVariable struct {
	Oid      string      `json:"oid"`
	Type     string      `json:"type"`
	Value    interface{} `json:"value"`
	ParseErr string      `json:"parse_err,omitempty"`
}

// PacketAsString used to format gosnmp.SnmpPacket for debug/trace logging
func PacketAsString(packet *gosnmp.SnmpPacket) string {
	if packet == nil {
		return ""
	}
	var debugVariables []debugVariable
	for _, pduVariable := range packet.Variables {
		var parseError string
		name := pduVariable.Name
		value := fmt.Sprintf("%v", pduVariable.Value)
		_, resValue, err := GetValueFromPDU(pduVariable)
		if err == nil {
			resValueStr, err := resValue.ToString()
			if err == nil {
				value = resValueStr
			}
		}
		if err != nil {
			parseError = fmt.Sprintf("`%s`", err)
		}
		debugVar := debugVariable{Oid: name, Type: fmt.Sprintf("%v", pduVariable.Type), Value: value, ParseErr: parseError}
		debugVariables = append(debugVariables, debugVar)
	}

	jsonPayload, err := json.Marshal(debugVariables)
	if err != nil {
		log.Debugf("error marshaling debugVar: %s", err)
		jsonPayload = []byte(``)
	}
	return fmt.Sprintf("error=%s(code:%d, idx:%d), values=%s", packet.Error, packet.Error, packet.ErrorIndex, jsonPayload)
}
