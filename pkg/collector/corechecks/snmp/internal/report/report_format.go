// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2022-present Datadog, Inc.

package report

import (
	"encoding/hex"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/snmp/internal/valuestore"
	"strings"
)

func formatValueWithDisplayHint(value valuestore.ResultValue, displayHint string) valuestore.ResultValue {
	switch value.Value.(type) {
	case []byte:
		val := value.Value.([]byte)
		// TODO: Implement generic DISPLAY-HINT formatting:
		//   https://www.webnms.com/snmp/help/snmpapi/snmpv3/using_mibs_in_applns/tcs_overview.html
		//   https://www.itu.int/wftp3/Public/t/fl/ietf/rfc/rfc2579/SNMPv2-TC.html
		//   https://linux.die.net/man/7/snmpv2-tm
		if displayHint == "1x:" {
			// Display Hint is "1x:" to indicate the value must consist of a one-byte hex string or two-hex digits, such as 01 or AB.
			value.Value = formatColonSepBytes(val)
		}
	}
	return value
}

func formatColonSepBytes(val []byte) string {
	octetsList := make([]string, 0, 11)
	for _, b := range val {
		octetsList = append(octetsList, hex.EncodeToString([]byte{b}))
	}
	return strings.Join(octetsList, ":")
}
