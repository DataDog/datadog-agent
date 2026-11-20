// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !linux && !windows

package common

import "context"

// restartServices is a no-op: outside Linux, the package hooks restart the services.
func (s *Setup) restartServices(_ context.Context, _ []packageWithVersion) error {
	return nil
}

// stopServices is a no-op: outside Linux, the package hooks stop the services.
func (s *Setup) stopServices(_ context.Context, _ []packageWithVersion) error {
	return nil
}
