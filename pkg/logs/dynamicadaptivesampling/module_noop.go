// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !python

package dynamicadaptivesampling

import "go.uber.org/fx"

// Module is a no-op for builds without Python support (e.g. the IoT agent).
// See module.go for the reason.
func Module() fx.Option {
	return fx.Options()
}
