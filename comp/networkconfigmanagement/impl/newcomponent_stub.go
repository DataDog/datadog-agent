// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build !ncm

// Package networkconfigmanagementimpl implements the networkconfigmanagement component interface
package networkconfigmanagementimpl

import (
	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	compdef "github.com/DataDog/datadog-agent/comp/def"
	"github.com/DataDog/datadog-agent/comp/networkconfigmanagement/stub"
)

// Requires defines the dependencies for the networkconfigmanagement component
type Requires struct {
	compdef.In
	Logger log.Component
}

// NewComponent creates a new networkconfigmanagement component
func NewComponent(reqs Requires) (Provides, error) {
	reqs.Logger.Debugf("NCM is disabled in this build")
	return NewProvides(stub.NewStub("network config management is disabled")), nil
}
