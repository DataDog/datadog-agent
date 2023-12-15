// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2022-present Datadog, Inc.

package format

// Direction remaps direction from 0 or 1 to respectively ingress or egress.
func Direction(direction uint32) string {
	if direction == 1 {
		return "egress"
	}
	return "ingress"
}
