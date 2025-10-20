// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

//go:build test

// Package fx provides the fx module for the secrets mock component
package fx

import (
	"testing"

	"go.uber.org/fx"

	secrets "github.com/DataDog/datadog-agent/comp/core/secrets/def"
	secretsmock "github.com/DataDog/datadog-agent/comp/core/secrets/mock"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

// Module specifies the mock secrets module.
func MockModule() fxutil.Module {
	return fxutil.Component(
		fx.Provide(func(t testing.TB) secrets.Component {
			return secretsmock.New(t)
		}),
	)
}
