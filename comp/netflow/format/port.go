// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2022-present Datadog, Inc.

package format

import "strconv"

// Port formats a port number. It's the same as strconv.Itoa, except that port
// -1 is mapped to the special string '*'.
func Port(port int32) string {
	if port >= 0 {
		return strconv.Itoa(int(port))
	}
	if port == -1 {
		return "*"
	}
	// this should never happen since port is either zero/positive or -1 (ephemeral port), no other value is currently supported
	return "invalid"
}
