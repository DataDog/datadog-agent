// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2022-present Datadog, Inc.

package format

var tcpFlagsMapping = map[uint32]string{
	1:  "FIN",
	2:  "SYN",
	4:  "RST",
	8:  "PSH",
	16: "ACK",
	32: "URG",
}

// TCPFlags formats a TCP bitmask as a set of strings.
func TCPFlags(flags uint32) []string {
	var strFlags []string
	flag := uint32(1)
	for {
		if flag > 32 {
			break
		}
		strFlag, ok := tcpFlagsMapping[flag]
		if !ok {
			continue
		}
		if flag&flags != 0 {
			strFlags = append(strFlags, strFlag)
		}
		flag <<= 1
	}
	return strFlags
}
