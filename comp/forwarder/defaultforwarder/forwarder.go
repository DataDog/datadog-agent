// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

package defaultforwarder

import (
	"github.com/DataDog/datadog-agent/pkg/forwarder"
	"go.uber.org/fx"
)

type dependencies struct {
	fx.In

	Params Params
}

func newForwarder(dep dependencies) Component {
	if dep.Params.UseNoopForwarder {
		return forwarder.NoopForwarder{}
	}
	return forwarder.NewDefaultForwarder(dep.Params.Options)
}
