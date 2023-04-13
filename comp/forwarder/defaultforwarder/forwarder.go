// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

package defaultforwarder

import (
	"go.uber.org/fx"
)

type dependencies struct {
	fx.In

	Params Params
}

func newForwarder(dep dependencies) Component {
	if dep.Params.UseNoopForwarder {
		return NoopForwarder{}
	}
	return NewDefaultForwarder(dep.Params.Options)
}

func newMockForwarder() Component {
	return NewDefaultForwarder(NewOptions(nil))
}
