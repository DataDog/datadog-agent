// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package multiplefx

import (
	multipleimpl "github.com/DataDog/datadog-agent/comp/multiple/impl"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

func Module() fxutil.Module {
	return fxutil.Component(
		fxutil.ProvideComponentConstructor(
			multipleimpl.NewComponent,
		),
	)
}
