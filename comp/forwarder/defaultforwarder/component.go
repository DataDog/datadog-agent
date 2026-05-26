// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

// Package defaultforwarder provides backward compatibility shims for the defaultforwarder component.
// Deprecated: use sub-packages (def, impl, fx, mock, noop-impl) instead.
package defaultforwarder

import (
	defaultforwarderdef "github.com/DataDog/datadog-agent/comp/forwarder/defaultforwarder/def"
	defaultforwarderimpl "github.com/DataDog/datadog-agent/comp/forwarder/defaultforwarder/impl"
)

// Forwarder interface allows packages to send payload to the backend.
// Deprecated: use comp/forwarder/defaultforwarder/def.Forwarder instead.
type Forwarder = defaultforwarderdef.Forwarder

// DefaultForwarder is the default forwarder implementation.
// Deprecated: use comp/forwarder/defaultforwarder/impl.DefaultForwarder instead.
type DefaultForwarder = defaultforwarderimpl.DefaultForwarder

// Options contains the configuration options for the forwarder.
// Deprecated: use comp/forwarder/defaultforwarder/impl.Options instead.
type Options = defaultforwarderimpl.Options

// Stopped represents the internal state of an unstarted Forwarder.
// Deprecated: use comp/forwarder/defaultforwarder/impl.Stopped instead.
const Stopped = defaultforwarderimpl.Stopped

// Started represents the internal state of a started Forwarder.
// Deprecated: use comp/forwarder/defaultforwarder/impl.Started instead.
const Started = defaultforwarderimpl.Started

// NewOptions creates new Options.
// Deprecated: use comp/forwarder/defaultforwarder/impl.NewOptions instead.
var NewOptions = defaultforwarderimpl.NewOptions

// NewDefaultForwarder creates a new DefaultForwarder.
// Deprecated: use comp/forwarder/defaultforwarder/impl.NewDefaultForwarder instead.
var NewDefaultForwarder = defaultforwarderimpl.NewDefaultForwarder
