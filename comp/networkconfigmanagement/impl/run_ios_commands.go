// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package networkconfigmanagementimpl

import (
	"context"
	"fmt"

	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	"github.com/DataDog/datadog-agent/pkg/networkconfigmanagement/ioscmd"
	ncmprofile "github.com/DataDog/datadog-agent/pkg/networkconfigmanagement/profile"
	"github.com/DataDog/datadog-agent/pkg/networkconfigmanagement/types"
)

// RunIOSCommands renders a block of IOS commands, runs them one at a time in
// a shell on a device, and returns a result for each line that ran. If a line
// fails, it returns the results so far along with an ErrCommandFailed error.
func (n *networkDeviceConfigImpl) RunIOSCommands(ctx context.Context, deviceID string, commands ioscmd.CommandBlock) ([]*types.CommandResult, types.TypedError) {
	// Render (and thereby validate) before touching the device.
	lines, rerr := commands.Render()
	if rerr != nil {
		return nil, types.WrapError(types.ErrInvalidCommand, rerr)
	}
	if len(lines) == 0 {
		return nil, types.WrapErrorf(types.ErrInvalidCommand, "no commands to run")
	}
	var log log.Component = NewLogWrapper(n.log, fmt.Sprintf("ncm[%s]: ", deviceID))
	log.Infof("Run command requested for Device %q", deviceID)
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

	if dc.profile.Name != ncmprofile.ProfileCiscoIOS {
		return nil, types.WrapErrorf(types.ErrCommandUnsupported, "device %s has profile %q, not %q", deviceID, dc.profile.Name, ncmprofile.ProfileCiscoIOS)
	}

	sh, serr := conn.StartShell(ctx)
	if serr != nil {
		return nil, serr
	}
	defer sh.Close()

	results, runErr := ioscmd.Run(ctx, sh, lines)
	var failed *types.CommandResult
	for _, result := range results {
		if err := redactResult(dc.profile, result); err != nil {
			return nil, types.WrapErrorf(types.ErrInternal, "unable to redact command output: %w", err)
		}
		if failed == nil && result.Error != "" {
			failed = result
		}
	}
	if runErr != nil {
		if failed != nil {
			// The run's error may embed raw output, so report the failure
			// from the redacted result instead.
			return results, types.WrapErrorf(types.ErrCommandFailed, "command %q failed: %w", failed.CommandStr, failed.FormattedError())
		}
		return results, types.WrapError(types.ErrCommandFailed, runErr)
	}
	return results, nil
}

// redactResult applies the profile's redaction rules to a result's output and
// error.
func redactResult(prof *ncmprofile.NCMProfile, result *types.CommandResult) error {
	output, err := prof.RedactOutput([]byte(result.Output))
	if err != nil {
		return err
	}
	result.Output = string(output)
	if result.Error != "" {
		msg, err := prof.RedactOutput([]byte(result.Error))
		if err != nil {
			return err
		}
		result.Error = string(msg)
	}
	return nil
}
