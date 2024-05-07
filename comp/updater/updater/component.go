// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package updater is the updater component.
package updater

import (
	"github.com/DataDog/datadog-agent/pkg/fleet/daemon"
)

// team: fleet

// Component is the interface for the updater component.
type Component interface {
	daemon.Daemon
}
