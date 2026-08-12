// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !windows

package statusapi

import (
	"path/filepath"

	"github.com/DataDog/datadog-agent/pkg/fleet/installer/paths"
)

const socketName = "installer-status.sock"

// Endpoint returns the unix socket the status API listens on. Both sides derive it
// from here so they cannot drift apart.
func Endpoint() string {
	return filepath.Join(paths.RunPath, socketName)
}
