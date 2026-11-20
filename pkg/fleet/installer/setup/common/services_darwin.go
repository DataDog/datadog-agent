// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package common

import "context"

// restartServices is a no-op: the macOS package hooks restart the LaunchDaemons.
func (s *Setup) restartServices(_ context.Context, _ []packageWithVersion) error {
	return nil
}

// stopServices is a no-op: the macOS package hooks stop the LaunchDaemons.
func (s *Setup) stopServices(_ context.Context, _ []packageWithVersion) error {
	return nil
}
