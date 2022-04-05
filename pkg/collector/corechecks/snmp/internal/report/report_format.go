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

func formatValue(value valuestore.ResultValue, format string) valuestore.ResultValue {
	switch value.Value.(type) {
	case []byte:
		val := value.Value.([]byte)
		if format == "mac_address" {
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
