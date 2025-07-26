// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

// Package privateactionrunnerimpl implements the privateactionrunner component interface
package privateactionrunnerimpl

import (
	compdef "github.com/DataDog/datadog-agent/comp/def"
	privateactionrunner "github.com/DataDog/datadog-agent/comp/privateactionrunner/def"
)

// Requires defines the dependencies for the snmpscan component
type Requires struct {
	compdef.In
}

// Provides defines the output of the privateactionrunner component
type Provides struct {
	Comp privateactionrunner.Component
}

// NewComponent creates a new privateactionrunner component
func NewComponent(reqs Requires) (Provides, error) {
	provides := Provides{}
	return provides, nil
}
