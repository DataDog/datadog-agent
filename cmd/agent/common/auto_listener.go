// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2017 Datadog, Inc.

// +build !docker

package common

import (
	"github.com/DataDog/datadog-agent/pkg/config"
)

// AutoAddListeners checks if the listener auto is selected and
// adds the docker listener if the host support it
// no effect if the listeners already contain the docker one
// Note: auto listener isn't a real listener but a way to automatically starts other listeners
// TODO: support more listeners (kubelet, ...)
func AutoAddListeners(listeners []config.Listeners) []config.Listeners {
	return listeners
}
