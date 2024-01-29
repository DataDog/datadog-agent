// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

package eventplatformimpl

// Params defines the parameters for the event platform forwarder.
type Params struct {
	UseNoopEventPlatformForwarder bool
	UseEventPlatformForwarder     bool
}

// NewDefaultParams returns the default parameters for the event platform forwarder.
func NewDefaultParams() Params {
	return Params{UseEventPlatformForwarder: true, UseNoopEventPlatformForwarder: false}
}
