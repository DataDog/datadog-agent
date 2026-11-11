// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build windows

package daemon

import (
	agentconfig "github.com/DataDog/datadog-agent/comp/core/config"
)

// NewRestrictedLocalAPI returns a no-op LocalAPI on Windows for now: the
// restricted dependency endpoint is unix-socket based and not implemented for
// the named pipe transport yet.
func NewRestrictedLocalAPI(_ Daemon, _ agentconfig.Reader) (LocalAPI, error) {
	return nil, nil
}
