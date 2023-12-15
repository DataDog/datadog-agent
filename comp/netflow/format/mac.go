// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2022-present Datadog, Inc.

package format

import (
	"encoding/binary"
	"net"
)

// MacAddress formats a mac address as "xx:xx:xx:xx:xx:xx"
func MacAddress(fieldValue uint64) string {
	mac := make([]byte, 8)
	binary.BigEndian.PutUint64(mac, fieldValue)
	return net.HardwareAddr(mac[2:]).String()
}
