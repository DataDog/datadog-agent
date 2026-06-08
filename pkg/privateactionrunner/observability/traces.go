// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package observability

const (
	// Name of the service used for tracing the private action runner
	ParService = "private-action-runner"

	// Name of the sub-services that we want to trace
	RshellService = "rshell"

	// ActionRunOperation is the operation name for the span that covers a
	// private action execution. Paired with the action FQN as the resource name.
	ActionRunOperation = "action.run"
)
