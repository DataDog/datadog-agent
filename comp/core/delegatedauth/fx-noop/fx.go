// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

// Package fx exposes the delegatedauth noop comp to FX
package fx

import (
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"

	delegatedauthnoopimpl "github.com/DataDog/datadog-agent/comp/core/delegatedauth/noop-impl"
)

// Module specifies the delegatedauth noop module.
func Module() fxutil.Module {
	return fxutil.Component(
		fxutil.ProvideComponentConstructor(
			delegatedauthnoopimpl.NewComponent,
		),
	)
}
