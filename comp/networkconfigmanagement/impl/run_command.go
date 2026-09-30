// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package networkconfigmanagementimpl

import (
	"context"
	"fmt"

	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	ncmconfig "github.com/DataDog/datadog-agent/pkg/networkconfigmanagement/config"
	"github.com/DataDog/datadog-agent/pkg/networkconfigmanagement/types"
)

// RunCommand sends a command to a device over one of its connections (e.g. SSH
// credentials) and returns the response as a CommandResult.
func (n *networkDeviceConfigImpl) RunCommand(ctx context.Context, deviceID string, command string, credentialSet ncmconfig.CredentialSet) (*types.CommandResult, types.TypedError) {
	if credentialSet == ncmconfig.CredentialSetRollback {
		return nil, types.WrapErrorf(types.ErrCannotConnect, "invalid credential set specified: %q", credentialSet)
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

	return conn.ExecuteCommand(ctx, command)
}
