// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package daemon

import (
	"context"
	"time"

	"github.com/DataDog/datadog-agent/pkg/procmgr/coat"
)

type procmgrSnapshotCollector interface {
	Collect(ctx context.Context) coat.Snapshot
}

func (d *daemonImpl) ddotProcessState(ctx context.Context) string {
	if d.procmgrCollector == nil {
		return coat.ProcessStateUnknown
	}
	ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	return d.procmgrCollector.Collect(ctx).ServiceProcessState(coat.ServiceIDDDOT)
}
