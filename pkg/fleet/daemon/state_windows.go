// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build windows

package daemon

import (
	"strconv"

	installerErrors "github.com/DataDog/datadog-agent/pkg/fleet/installer/errors"
	windowsuser "github.com/DataDog/datadog-agent/pkg/fleet/installer/packages/user/windows"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// agentUserErrorCode collects the outcome of the Agent user check that remote
// updates depend on, as the installer error code an update would fail with.
// Returns "ok" when the check passes.
//
// This only collects the outcome, it enforces nothing.
func agentUserErrorCode() string {
	user, err := windowsuser.GetAgentUserFromService()
	if err == nil {
		err = windowsuser.ValidateAgentUserRemoteUpdatePrerequisites(user)
	}
	if err == nil {
		return "ok"
	}
	log.Warnf("Agent user is not usable for a remote update: %v", err)
	return strconv.FormatUint(uint64(installerErrors.GetCode(err)), 10)
}
