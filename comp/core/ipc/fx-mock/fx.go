// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

// Package fx provides the fx module for the ipc component
package fx

import (
	"testing"

	ipc "github.com/DataDog/datadog-agent/comp/core/ipc/def"
	"github.com/DataDog/datadog-agent/comp/core/ipc/mock"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

// Module defines the fx options for this component
func Module() fxutil.Module {
	return fxutil.Component(
		fxutil.ProvideComponentConstructor(newMock),
		fxutil.ProvideOptional[ipc.Component](),
	)
}

// Requires defines the dependencies for the ipc mock component
type Requires struct {
	T testing.TB
}

// newMock creates a new mock ipc component
func newMock(reqs Requires) ipc.Component {
	return mock.Mock(reqs.T)
}
