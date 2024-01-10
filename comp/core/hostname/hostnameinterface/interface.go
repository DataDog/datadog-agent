// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

// Package hostnameinterface describes hostname methods required in logs agent modules
package hostnameinterface

import (
	"context"
)

type HostnameInterface interface {
	// Get returns the host name for the agent.
	Get(context.Context) (string, error)
}
