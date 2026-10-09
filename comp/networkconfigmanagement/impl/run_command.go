// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package networkconfigmanagementimpl

import (
	"context"
	"fmt"
	"strings"

	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	ncmconfig "github.com/DataDog/datadog-agent/pkg/networkconfigmanagement/config"
	"github.com/DataDog/datadog-agent/pkg/networkconfigmanagement/ioscmd"
	"github.com/DataDog/datadog-agent/pkg/networkconfigmanagement/types"
)

// RunCommand renders a block of IOS commands, sends it to a device over one of
// its connections (e.g. SSH credentials), and returns the response as a
// CommandResult.
func (n *networkDeviceConfigImpl) RunCommand(ctx context.Context, deviceID string, commands ioscmd.CommandBlock, credentialSet ncmconfig.CredentialSet) (*types.CommandResult, types.TypedError) {
	if credentialSet == ncmconfig.CredentialSetRollback {
		return nil, types.WrapErrorf(types.ErrCannotConnect, "invalid credential set specified: %q", credentialSet)
	}
	// Render (and thereby validate) before touching the device.
	lines, rerr := commands.Render()
	if rerr != nil {
		return nil, types.WrapError(types.ErrInvalidCommand, rerr)
	}
	if len(lines) == 0 {
		return nil, types.WrapErrorf(types.ErrInvalidCommand, "no commands to run")
	}
	var log log.Component = NewLogWrapper(n.log, fmt.Sprintf("ncm[%s]: ", deviceID))
	log.Infof("Run command requested for Device %q using credential set %s", deviceID, credentialSet)
	ctx = WithLogger(ctx, log)

	dc, err := n.devices.GetAndLock(ctx, deviceID)
	if err != nil {
		return nil, types.AsTypedError(err)
	}
	defer dc.UnlockOrLog(log)

	conn, cerr := n.connectAndEnsureProfile(ctx, dc, credentialSet)
	if cerr != nil {
		return nil, cerr
	}
	defer conn.Close()

	// The lines are sent as a single exec payload, the same way
	// PlainCommand.SetupCommands are.
	return conn.ExecuteCommand(ctx, strings.Join(lines, "\n"))
}
