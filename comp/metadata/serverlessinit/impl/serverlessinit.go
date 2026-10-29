// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !windows

// Package serverlessinitimpl implements the serverless-init metadata provider.
package serverlessinitimpl

import (
	serverlessdef "github.com/DataDog/datadog-agent/comp/metadata/serverlessinit/def"
)

// Requires defines the dependencies for the serverlessinit component.
type Requires struct{}

// Provides defines the output of the serverlessinit component.
type Provides struct {
	Comp serverlessdef.Component
}

type serverlessinitComp struct{}

// NewComponent returns a component that reports no metadata.
func NewComponent(_ Requires) Provides {
	return Provides{Comp: &serverlessinitComp{}}
}

// ReportNow does nothing.
func (c *serverlessinitComp) ReportNow() {}
