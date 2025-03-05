// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

//go:build test

// Package fx provides the fx module for the tagger component
package fx

import (
	"testing"

	taggerimpl "github.com/DataDog/datadog-agent/comp/core/tagger/impl"
	taggermock "github.com/DataDog/datadog-agent/comp/core/tagger/mock"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

// Module is a module containing the mock, useful for testing
func Module() fxutil.Module {
	return fxutil.Component(
		fxutil.ProvideComponentConstructor(taggerimpl.New),
	)
}

// SetupFakeTagger calls fxutil.Test to create a mock tagger for testing
func SetupFakeTagger(t testing.TB) taggermock.Mock {
	return fxutil.Test[taggermock.Mock](t, Module())
}
