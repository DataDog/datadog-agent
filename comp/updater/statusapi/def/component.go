// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package statusapi is the installer read-only status api component.
package statusapi

import (
	"github.com/DataDog/datadog-agent/pkg/fleet/daemon"
)

// team: fleet windows-products

// Component is the interface for the installer status api component.
type Component interface {
	daemon.StatusAPI
}
