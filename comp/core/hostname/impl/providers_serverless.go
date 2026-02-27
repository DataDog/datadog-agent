// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build serverless

package hostnameimpl

import (
	"context"

	hostnamedef "github.com/DataDog/datadog-agent/comp/core/hostname/def"
	pkgconfigmodel "github.com/DataDog/datadog-agent/pkg/config/model"
)

// getHostname returns empty Data for serverless — there is no meaningful hostname.
func getHostname(_ context.Context, _ pkgconfigmodel.Reader, _ string, _ bool) (hostnamedef.Data, error) {
	return hostnamedef.Data{}, nil
}
