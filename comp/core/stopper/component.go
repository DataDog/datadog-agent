// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package stopper implements a component that will shutdown a running Fx App
// on receipt of SIGINT or SIGTERM or of an explicit stop signal.  Note that
// this will only work with apps (fxutil.Run) and not with one-shot commands
// (fxutil.OneShot).
//
// During the componentization process, this component also interfaces with
// cmd/agent/common/signals.
//
// This component is not automatically instantiated.  Any apps using `fxutil.Run`
// should instantiate the component to get the expected stopping behavior.  This
// decision may be revisited once cmd/agent/common/signals is removed, at which
// time there is no harm in always instantiating this component.
//
// The component registers for signals if BundlParams#StopOnSignals is true. In
// this case, it also ignores SIGPIPE.
package stopper

import (
	"go.uber.org/fx"
)

// team: agent-shared-components

const componentName = "comp/core/stopper"

// Component is the component type.
type Component interface {
	// Stop causes the running app to stop, asynchronously.  If the error is
	// nil, then it is a "normal" stop; otherwise, it finishes with the error.
	//
	// This call is asynchronous, and will return immediately with app shutdown
	// beginning in another goroutine.
	Stop(error)
}

// Module defines the fx options for this component.
var Module fx.Option = fx.Module(
	componentName,

	fx.Provide(newStopper),
)
