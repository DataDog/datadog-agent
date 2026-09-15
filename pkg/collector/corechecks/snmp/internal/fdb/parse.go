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

func parseQBridgeIndex(index string) (fdbID uint32, mac string, ok bool) {
	parts := strings.Split(index, ".")
	if len(parts) < 7 {
		return 0, "", false
	}
	macParts := parts[len(parts)-6:]
	mac, ok = macFromOIDParts(macParts)
	if !ok {
		return 0, "", false
	}
	idParts := parts[:len(parts)-6]
	// Some agents encode MacAddress with an explicit length sub-id of 6
	// ({fdbId}.6.{6 octets}) instead of the common implied form ({fdbId}.{6 octets}).
	if n := len(idParts); n >= 2 && idParts[n-1] == "6" {
		idParts = idParts[:n-1]
	}
	if len(idParts) != 1 {
		return 0, "", false
	}
	id, err := strconv.ParseUint(idParts[0], 10, 32)
	if err != nil {
		return 0, "", false
	}
	return uint32(id), mac, true
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
