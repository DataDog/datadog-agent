// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package compdef defines basic types used for components
package compdef

import (
	"context"
)

type lchFunc func(context.Context) error

// Hook represents a function pair for a component's startup and shutdown
type Hook struct {
	OnStart lchFunc
	OnStop  lchFunc
}

// Lifecycle may be added to a component's requires struct if it wants to add hooks
type Lifecycle interface {
	Append(h Hook)
}
