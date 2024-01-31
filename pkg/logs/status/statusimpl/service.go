// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

// Package statusimpl implements status methods required in logs agent modules
package statusimpl

import (
	"github.com/DataDog/datadog-agent/pkg/logs/status"
	"github.com/DataDog/datadog-agent/pkg/logs/status/statusinterface"
)

// service represents a status implementation that is required in order to decouple status methods
// from logs agent modules. This should be replaced when pkg/logs/status is converted to a component.
type service struct{}

// AddGlobalWarning keeps track of a warning message to display on the status.
func (ss *service) AddGlobalWarning(key string, warning string) {
	status.AddGlobalWarning(key, warning)
}

// RemoveGlobalWarning loses track of a warning message
// that does not need to be displayed on the status anymore.
func (ss *service) RemoveGlobalWarning(key string) {
	status.RemoveGlobalWarning(key)
}

// NewStatusImpl fetches the status and returns a service wrapping it
func NewStatusImpl() statusinterface.Component {
	return &service{}
}
