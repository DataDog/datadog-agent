// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package host implements a component to generate the 'host' metadata payload (also known as "v5").
package host

import (
	"context"

	"github.com/shirou/gopsutil/v3/host"
)

// team: agent-shared-components

// Component is the component type.
type Component interface {
	GetPayloadAsJSON(ctx context.Context) ([]byte, error)
	GetInformation() *host.InfoStat
}
