// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !linux && !windows

package module

import "github.com/DataDog/datadog-agent/pkg/security/proto/api"

func (a *APIServer) collectOSReleaseData() {}

func (a *APIServer) fillStatusPlatform(_ *api.Status) error {
	return nil
}
