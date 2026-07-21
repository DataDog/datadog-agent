// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2022-present Datadog, Inc.

package format

import "fmt"

var dscpMap = map[uint32]string{
	0:  "CS0",
	1:  "LE",
	8:  "CS1",
	10: "AF11",
	12: "AF12",
	14: "AF13",
	16: "CS2",
	18: "AF21",
	20: "AF22",
	22: "AF23",
	24: "CS3",
	26: "AF31",
	28: "AF32",
	30: "AF33",
	32: "CS4",
	34: "AF41",
	36: "AF42",
	38: "AF43",
	40: "CS5",
	44: "VOICE-ADMIT",
	45: "NQB",
	46: "EF",
	48: "CS6",
	56: "CS7",
}

func DSCPFromTOS(tos uint32) uint32 {
	return tos >> 2
}

func DSCPNameFromTOS(tos uint32) string {
	dscp := DSCPFromTOS(tos)
	if name, ok := dscpMap[dscp]; ok {
		return name
	}
	return fmt.Sprintf("DSCP-%d", dscp)
}
