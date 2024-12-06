// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build test

// Package mock contains the implementation of the mock for the tagger component.
package mock

import (
	"testing"

	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

// Module is a module containing the mock, useful for testing
func Module() fxutil.Module {
	return fxutil.Component(
		fxutil.ProvideComponentConstructor(New),
	)
}

// SetupFakeTagger calls fxutil.Test to create a mock tagger for testing
func SetupFakeTagger(t testing.TB) Mock {
	return fxutil.Test[Mock](t, Module())
}
