// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

// Package defaultforwarder implements a component to send payloads to the backend
package defaultforwarder

import (
	defaultforwarderdef "github.com/DataDog/datadog-agent/comp/forwarder/defaultforwarder/def"
	defaultforwarderimpl "github.com/DataDog/datadog-agent/comp/forwarder/defaultforwarder/impl"
)

// Forwarder is the interface for the default forwarder.
//
// Deprecated: use comp/forwarder/defaultforwarder/def instead.
type Forwarder = defaultforwarderdef.Forwarder

// Options contains the options for the default forwarder.
//
// Deprecated: use comp/forwarder/defaultforwarder/impl instead.
type Options = defaultforwarderimpl.Options

// DefaultForwarder is the default implementation of the forwarder.
//
// Deprecated: use comp/forwarder/defaultforwarder/impl instead.
type DefaultForwarder = defaultforwarderimpl.DefaultForwarder

// NewOptions creates forwarder options from the given configuration.
//
// Deprecated: use comp/forwarder/defaultforwarder/impl instead.
var NewOptions = defaultforwarderimpl.NewOptions

// NewDefaultForwarder creates a new default forwarder.
//
// Deprecated: use comp/forwarder/defaultforwarder/impl instead.
var NewDefaultForwarder = defaultforwarderimpl.NewDefaultForwarder

// Stopped represents the internal state of an unstarted Forwarder.
//
// Deprecated: use comp/forwarder/defaultforwarder/impl instead.
const Stopped = defaultforwarderimpl.Stopped

// Started represents the internal state of a started Forwarder.
//
// Deprecated: use comp/forwarder/defaultforwarder/impl instead.
const Started = defaultforwarderimpl.Started
