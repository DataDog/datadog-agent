// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2022-present Datadog, Inc.

package format

import "github.com/DataDog/datadog-agent/comp/netflow/common"

// Direction remaps direction from 0 or 1 to respectively ingress or egress.
func Direction(direction uint32, flowType common.FlowType) string {
	// See https://www.cisco.com/en/US/technologies/tk648/tk362/technologies_white_paper09186a00800a3db9.html#:~:text=4%20is%20assumed.-,DIRECTION,-61
	if flowType != common.TypeNetFlow9 {
		return "undefined"
	}
	if direction == 1 {
		return "egress"
	}
	return "ingress"
}
