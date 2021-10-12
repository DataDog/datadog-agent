package gosnmplib

import (
	"encoding/json"
	"fmt"

	"github.com/cihub/seelog"
	"github.com/gosnmp/gosnmp"

	"github.com/DataDog/datadog-agent/pkg/util/log"
)

type debugVariable struct {
	Oid      string      `json:"oid"`
	Type     string      `json:"type"`
	Value    interface{} `json:"value"`
	ParseErr string      `json:"parse_err,omitempty"`
}

// PacketAsStringIfLoglevel used to format gosnmp.SnmpPacket for debug logging
func PacketAsStringIfLoglevel(packet *gosnmp.SnmpPacket, logLevel seelog.LogLevel) string {
	if packet == nil {
		return ""
	}
	if curLogLevel, err := log.GetLogLevel(); err != nil || curLogLevel <= logLevel {
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
		}
		return fmt.Sprintf("error=%s(code:%d, idx:%d), values=%s", packet.Error, packet.Error, packet.ErrorIndex, jsonPayload)
	}
	return ""
}
