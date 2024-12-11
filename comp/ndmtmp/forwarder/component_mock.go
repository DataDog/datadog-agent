// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

// Package forwarder exposes the event platform forwarder for netflow.
package forwarder

import "github.com/DataDog/datadog-agent/comp/forwarder/eventplatform/eventplatformimpl"

// MockComponent is the type for mock components.
// It is a gomock-generated mock of EventPlatformForwarder.
type MockComponent interface {
	Component
	EXPECT() *eventplatformimpl.MockEventPlatformForwarderMockRecorder
}
