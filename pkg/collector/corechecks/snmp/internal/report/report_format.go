// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2022-present Datadog, Inc.

package report

import (
	"encoding/hex"
	"fmt"
	"net"
	"strings"

	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/snmp/internal/valuestore"
)

func formatValue(value valuestore.ResultValue, format string) (valuestore.ResultValue, error) {
	switch value.Value.(type) {
	case []byte:
		val := value.Value.([]byte)
		switch format {
		case "mac_address":
			// Format mac address from OctetString to IEEE 802.1a canonical format e.g. `82:a5:6e:a5:c8:01`
			value.Value = formatColonSepBytes(val)
		case "ip_address":
			if len(val) == 0 {
				value.Value = ""
			} else {
				value.Value = net.IP(val).String()
			}
		default:
			return valuestore.ResultValue{}, fmt.Errorf("unknown format `%s` (value type `%T`)", format, value.Value)
		}
	default:
		return valuestore.ResultValue{}, fmt.Errorf("value type `%T` not supported (format `%s`)", value.Value, format)
	}
	return value, nil
}

func formatColonSepBytes(val []byte) string {
	octetsList := make([]string, 0, 11)
	for _, b := range val {
		octetsList = append(octetsList, hex.EncodeToString([]byte{b}))
	}
	return strings.Join(octetsList, ":")
}
