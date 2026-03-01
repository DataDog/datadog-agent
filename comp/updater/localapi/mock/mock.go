// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build test

// Package mock provides a mock for the updater local api component.
package mock

import (
	"context"
	"testing"

	localapi "github.com/DataDog/datadog-agent/comp/updater/localapi/def"
)

type mockLocalAPI struct{}

func (m *mockLocalAPI) Start(_ context.Context) error { return nil }
func (m *mockLocalAPI) Stop(_ context.Context) error  { return nil }

// Mock returns a mock for the local api component.
func Mock(_ *testing.T) localapi.Component {
	return &mockLocalAPI{}
}
