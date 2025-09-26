//go:build !windows

// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

// Package fx provides the fx module for the publishermetadatacache component
package fx

import (
	publishermetadatacache "github.com/DataDog/datadog-agent/comp/publishermetadatacache/def"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
	"go.uber.org/fx"
)

// Module provides a stub module for non-Windows platforms
func Module() fxutil.Module {
	return fxutil.Component(
		fx.Provide(func() publishermetadatacache.Component {
			return &stubComponent{}
		}),
	)
}

// stubComponent implements the Component interface for non-Windows platforms
type stubComponent struct{}
