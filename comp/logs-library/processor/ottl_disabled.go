// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !otlp

package processor

// ottlEnabled reports whether this build links the OTTL runtime. When false,
// the compiler drops the OTTL branches of applyProcessingRules.
const ottlEnabled = false
