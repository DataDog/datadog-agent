// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build windows

package daemon

import (
	"context"
	"fmt"
	"strconv"
	"time"

	installerErrors "github.com/DataDog/datadog-agent/pkg/fleet/installer/errors"
	windowsuser "github.com/DataDog/datadog-agent/pkg/fleet/installer/packages/user/windows"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// agentUserCheckTimeout bounds the Agent user check. Its SID and gMSA lookups query the
// domain controller, and refreshState runs it with the daemon lock held, so an unreachable
// domain controller must not block the daemon for as long as Windows takes to give up.
const agentUserCheckTimeout = 10 * time.Second

// agentUserErrorCode collects the outcome of the Agent user check that remote
// updates depend on, as the installer error code an update would fail with.
// Returns "ok" when the check passes.
//
// This only collects the outcome, it enforces nothing.
func agentUserErrorCode(ctx context.Context) string {
	ctx, cancel := context.WithTimeout(ctx, agentUserCheckTimeout)
	defer cancel()

	// The check is a chain of blocking Win32 calls. It takes ctx and stops between calls
	// once it is done, but a call already in flight cannot be interrupted — so run it off
	// this goroutine to be sure we return on time. The buffer lets it exit either way.
	done := make(chan error, 1)
	go func() {
		user, err := windowsuser.GetAgentUserFromService()
		if err == nil {
			err = windowsuser.ValidateAgentUserRemoteUpdatePrerequisites(ctx, user)
		}
		done <- err
	}()

	var err error
	select {
	case err = <-done:
	case <-ctx.Done():
		err = installerErrors.Wrap(installerErrors.ErrTimeout,
			fmt.Errorf("the Agent user check did not complete within %s: %w", agentUserCheckTimeout, ctx.Err()))
	}
	if err == nil {
		return "ok"
	}
	log.Warnf("Agent user is not usable for a remote update: %v", err)
	return strconv.FormatUint(uint64(installerErrors.GetCode(err)), 10)
}
