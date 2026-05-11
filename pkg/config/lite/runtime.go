// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package lite

import "runtime"

// goosBuildtime is the actual platform GOOS at compile time. Wrapping
// runtime.GOOS in a function lets tests stub it without import cycles.
func goosBuildtime() string { return runtime.GOOS }
