// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

//go:build serverless

package hostnameimpl

import (
	hostnamedef "github.com/DataDog/datadog-agent/comp/core/hostname/def"
	coretelemetry "github.com/DataDog/datadog-agent/comp/core/telemetry"
	pkgconfigmodel "github.com/DataDog/datadog-agent/pkg/config/model"
)

// driftService is a no-op for serverless builds — hostname is always empty, drift is meaningless.
type driftService struct{}

// newDriftService returns a no-op drift service for serverless builds.
func newDriftService(_ pkgconfigmodel.Reader, _ coretelemetry.Component) *driftService {
	return &driftService{}
}

func (ds *driftService) start(_ hostnamedef.Data) {}
func (ds *driftService) stop()                    {}
