// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package networkconfigmanagementimpl

import (
	"context"
	"fmt"

	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	"github.com/DataDog/datadog-agent/pkg/networkconfigmanagement/types"
)

// RunCommand sends an arbitrary command to a device over its established
// connection (e.g. SSH) and returns the device's response as a CommandResult.
func (n *networkDeviceConfigImpl) RunCommand(ctx context.Context, deviceID string, command string) (*types.CommandResult, types.TypedError) {
	var log log.Component = NewLogWrapper(n.log, fmt.Sprintf("ncm[%s]: ", deviceID))
	log.Infof("Run command requested: Device %q: %q", deviceID, command)
	ctx = WithLogger(ctx, log)

	dc, err := n.devices.GetAndLock(ctx, deviceID)
	if err != nil {
		return nil, types.AsTypedError(err)
	}
	defer dc.UnlockOrLog(log)

	conn, cerr := n.connectAndEnsureProfile(ctx, dc)
	if cerr != nil {
		return nil, cerr
	}
	defer conn.Close()

	return conn.ExecuteCommand(ctx, command)
}
