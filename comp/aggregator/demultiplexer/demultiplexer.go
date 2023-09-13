// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

package demultiplexer

import (
	"context"

	"github.com/DataDog/datadog-agent/comp/core/log"
	"github.com/DataDog/datadog-agent/comp/forwarder/defaultforwarder"
	"github.com/DataDog/datadog-agent/pkg/aggregator"
	"github.com/DataDog/datadog-agent/pkg/util/hostname"
	"go.uber.org/fx"
)

type dependencies struct {
	fx.In
	Log             log.Component
	SharedForwarder defaultforwarder.Component

	Params Params
}

func newDemultiplexer(deps dependencies) (Component, error) {
	hostnameDetected, err := hostname.Get(context.TODO())
	if err != nil {
		return nil, deps.Log.Errorf("Error while getting hostname, exiting: %v", err)
	}

	demux := aggregator.InitAndStartAgentDemultiplexer(
		deps.Log,
		deps.SharedForwarder,
		deps.Params.Options,
		hostnameDetected)

	return demux, nil
}
