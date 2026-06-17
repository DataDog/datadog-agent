// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package eventplatform

import (
	sdsscanner "github.com/DataDog/datadog-agent/comp/core/sdsscanner/def"
	"github.com/DataDog/datadog-agent/pkg/logs/message"
)

// TransformerDependencies bundles the forwarder dependencies that are made
// available to an EventTransformer. A transformer keeps whatever it needs and
// ignores the rest; any field may be nil when the corresponding dependency is
// not wired into the running binary.
type TransformerDependencies struct {
	Scanners sdsscanner.Component
}

// EventTransformer mutates the messages of the pipeline it is attached to
// before the event platform forwarder sends them. It is optional: when a
// pipeline has no transformer, its messages are forwarded unchanged.
type EventTransformer interface {
	// Init is called once, before any call to Transform, with all the forwarder
	// dependencies. The transformer picks and stores whatever it needs.
	// Returning an error disables the transformer.
	Init(deps TransformerDependencies) error

	// Transform mutates the message in place. Returning an error drops the
	// message instead of forwarding it.
	Transform(e *message.Message) error
}
