// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package fx expose the secrets comp to FX
package fx

import (
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"

	secretsimpl "github.com/DataDog/datadog-agent/comp/core/secrets/impl"
)

// Module specifies the secrets module.
func Module() fxutil.Module {
	return fxutil.Component(
		fxutil.ProvideComponentConstructor(
			secretsimpl.NewComponent,
		),
	)
}
