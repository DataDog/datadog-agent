// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

// Package forwarder exposes the event platform forwarder for netflow.
package forwarder

import (
	"github.com/DataDog/datadog-agent/comp/forwarder/eventplatform"
)

// team: ndm-core

// Component is the component type.
type Component interface {
	eventplatform.Forwarder
}
