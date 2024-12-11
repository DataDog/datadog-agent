// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package classicimpl

import (
	"go.uber.org/fx"

	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

type dependencies struct {
	fx.In
}

type provides struct {
	fx.Out
}

type implementation struct{}

func newClassic() Component {
	return &implementation{}
}

func Module() fxutil.Module {
	return fxutil.Component(
		fx.Provide(newClassic))
}
