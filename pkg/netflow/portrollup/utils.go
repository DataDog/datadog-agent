// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2022-present Datadog, Inc.

package portrollup

import "strconv"

// PortToString convert port to string
func PortToString(port int32) string {
	if port >= 0 {
		return strconv.Itoa(int(port))
	}
	if port == EphemeralPort {
		return "*"
	}
	// this should never happen since port is either zero/positive or -1 (ephemeral port), no other value is currently supported
	return "invalid"
}
