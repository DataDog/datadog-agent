// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package mock

import (
	eventplatform "github.com/DataDog/datadog-agent/comp/forwarder/eventplatform/def"
	eventplatformnoop "github.com/DataDog/datadog-agent/comp/forwarder/eventplatform/impl-noop"
)

// NewMock returns a new mock component.
func NewMock() eventplatform.Component {
	return eventplatformnoop.NewComponent()
}
