// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package status defines the Trace Agent status component.
package status

import pbcore "github.com/DataDog/datadog-agent/pkg/proto/pbgo/core"

// team: agent-apm

// Component is the status interface.
type Component interface {
	pbcore.StatusProviderServer
}
