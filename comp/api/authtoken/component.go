// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

// Package authtoken implements the creation and access to the auth_token used to communicate between Agent processes.
// This component offers two implementations: one to create and fetch the auth_token and another that doesn't create the
// auth_token file but can fetch it it's available.
package authtoken

import (
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
	"github.com/DataDog/datadog-agent/pkg/util/optional"
	"go.uber.org/fx"
)

// team: agent-shared-components

// Component is the component type.
type Component interface {
	Get() string
}

// NoneModule return a None optional type for authtoken.Component.
//
// This helper allows code that needs a disabled Optional type for authtoken to get it. The helper is split from
// the implementation to avoid linking with the dependencies from sysprobeconfig.
func NoneModule() fxutil.Module {
	return fxutil.Component(fx.Provide(func() optional.Option[Component] {
		return optional.NewNoneOption[Component]()
	}))
}
