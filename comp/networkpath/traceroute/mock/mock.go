// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build test

// Package mock provides a mock for the traceroute component
package mock

import (
	"context"
	"testing"

	traceroute "github.com/DataDog/datadog-agent/comp/networkpath/traceroute/def"
	"github.com/DataDog/datadog-agent/pkg/networkpath/payload"
	"github.com/DataDog/datadog-agent/pkg/networkpath/traceroute/config"
)

// Mock returns a mock for traceroute component.
func Mock(_t testing.TB) traceroute.Component {
	return &mock{}
}

type mock struct{}

func (m *mock) Run(_ctx context.Context, _cfg config.Config) (payload.NetworkPath, error) {
	return payload.NetworkPath{}, nil
}
