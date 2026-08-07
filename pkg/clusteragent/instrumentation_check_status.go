// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package clusteragent

import "github.com/DataDog/datadog-agent/pkg/clusteragent/clusterchecks/types"

// InstrumentationCheckStatusReceiver accepts runtime check status reports from
// node Agents.
type InstrumentationCheckStatusReceiver interface {
	SubmitInstrumentationCheckStatus(types.InstrumentationCheckStatusRequest)
}
