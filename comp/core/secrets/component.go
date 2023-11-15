// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package secrets
package secrets

import (
	"io"

	flaretypes "github.com/DataDog/datadog-agent/comp/core/flare/types"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
	"go.uber.org/fx"
)

type dependencies struct {
	fx.In
}

type provides struct {
	fx.Out

	Comp          Component
	FlareProvider flaretypes.Provider
}

// team: agent-shared-components

// Component is the component type.
type Component interface {
	Assign(command string, arguments []string, timeout, maxSize int, groupExecPerm, removeLinebreak bool)
	GetDebugInfo(w io.Writer)
	Decrypt(data []byte, origin string) ([]byte, error)
}

// Module defines the fx options for this component.
var Module = fxutil.Component(
	fx.Provide(newSecretResolverProvider),
)
