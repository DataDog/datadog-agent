// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

//go:build test

// Package mock provides a mock for the local API client component.
package mock

import (
	"testing"

	localapiclient "github.com/DataDog/datadog-agent/comp/updater/localapiclient/def"
	"github.com/DataDog/datadog-agent/pkg/fleet/daemon"
)

// Mock returns a mock for the local API client component.
func Mock(_ *testing.T) localapiclient.Component {
	return daemon.NewLocalAPIClient()
}
