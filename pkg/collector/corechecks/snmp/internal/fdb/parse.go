// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package fdb

import (
	"fmt"
	"strconv"
	"strings"
)

func parseQBridgeIndex(index string) (mac string, ok bool) {
	parts := strings.Split(index, ".")
	if len(parts) == 8 && parts[1] == "6" {
		return macFromOIDParts(parts[2:])
	}
	if len(parts) != 7 {
		return "", false
	}
	return macFromOIDParts(parts[1:])
}

func parseBridgeIndex(index string) (mac string, ok bool) {
	parts := strings.Split(index, ".")
	if len(parts) != 6 {
		return "", false
	}
	return macFromOIDParts(parts)
}

func macFromOIDParts(parts []string) (string, bool) {
	if len(parts) != 6 {
		return "", false
	}
	octets := make([]byte, 6)
	for i, p := range parts {
		n, err := strconv.Atoi(p)
		if err != nil || n < 0 || n > 255 {
			return "", false
		}
		octets[i] = byte(n)
	}
	return formatMAC(octets), true
}

func formatMAC(octets []byte) string {
	return fmt.Sprintf("%02x:%02x:%02x:%02x:%02x:%02x", octets[0], octets[1], octets[2], octets[3], octets[4], octets[5])
}

func isZeroMAC(mac string) bool {
	return mac == "00:00:00:00:00:00"
}

func isBroadcastMAC(mac string) bool {
	return mac == "ff:ff:ff:ff:ff:ff"
}

func isMulticastMAC(mac string) bool {
	parts := strings.Split(mac, ":")
	if len(parts) != 6 {
		return false
	}
	n, err := strconv.ParseUint(parts[0], 16, 8)
	if err != nil {
		return false
	}
	return n&1 == 1
}
