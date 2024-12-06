// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package compdef defines basic types used for components
package compdef

// Shutdowner may be added to a component's dependencies so it can shutdown the application
type Shutdowner interface {
	Shutdown() error
}
