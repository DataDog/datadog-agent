// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

// Package forwarder exposes the event platform forwarder for netflow.
package forwarder

import (
	"testing"

	"github.com/DataDog/datadog-agent/comp/ndmtmp/aggregator"
	"github.com/DataDog/datadog-agent/pkg/epforwarder"
	"github.com/golang/mock/gomock"
)

func getForwarder(agg aggregator.Component) (Component, error) {
	return agg.GetEventPlatformForwarder()
}

func getMockForwarder(t testing.TB) MockComponent {
	ctrl := gomock.NewController(t)
	return epforwarder.NewMockEventPlatformForwarder(ctrl)
}
