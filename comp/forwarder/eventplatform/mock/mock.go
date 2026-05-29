// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build test

// Package mock provides a mock event platform forwarder component.
package mock

import (
	"github.com/DataDog/datadog-agent/comp/core/hostname"
	compdef "github.com/DataDog/datadog-agent/comp/def"
	eventplatform "github.com/DataDog/datadog-agent/comp/forwarder/eventplatform/def"
	eventplatformimpl "github.com/DataDog/datadog-agent/comp/forwarder/eventplatform/impl"
	logscompression "github.com/DataDog/datadog-agent/comp/serializer/logscompression/def"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
	"github.com/DataDog/datadog-agent/pkg/util/option"
)

type mockRequires struct {
	Hostname    hostname.Component
	Compression logscompression.Component
}

type mockProvides struct {
	compdef.Out
	Comp eventplatform.Component
}

func newMockComponent(reqs mockRequires) mockProvides {
	return mockProvides{
		Comp: option.NewPtr[eventplatform.Forwarder](
			eventplatformimpl.NewNoopEventPlatformForwarder(reqs.Hostname, reqs.Compression),
		),
	}
}

// MockModule defines the fx options for the mock component.
func MockModule() fxutil.Module {
	return fxutil.Component(
		fxutil.ProvideComponentConstructor(newMockComponent),
	)
}
