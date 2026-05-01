// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !python

package python

import healthplatformdef "github.com/DataDog/datadog-agent/comp/healthplatform/core/def"

// InitPython is a no-op when the build tag is not set
func InitPython(_ ...string) {}

// SetHealthPlatform is a no-op when Python support is not compiled in.
func SetHealthPlatform(_ healthplatformdef.Component) {}
