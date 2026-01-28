// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !linux && !windows

package module

import (
	sprobe "github.com/DataDog/datadog-agent/pkg/security/probe"
	"github.com/DataDog/datadog-agent/pkg/security/proto/api"
	"github.com/DataDog/datadog-agent/pkg/security/secl/model"
)

// sshSessionPatcher stub for other platforms
type sshSessionPatcher interface {
	IsResolved() error
	PatchEvent(_ interface{})
	MaxRetry() int
}

// createSSHSessionPatcher creates a no-op patcher for other platforms
func createSSHSessionPatcher(_ *model.Event, _ *sprobe.Probe) sshSessionPatcher {
	return nil
}

func (a *APIServer) collectOSReleaseData() {}

func (a *APIServer) fillStatusPlatform(_ *api.Status) error {
	return nil
}
