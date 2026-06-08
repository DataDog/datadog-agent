// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package dogstatsdunit contains e2e tests verifying that DogStatsD timing (ms)
// metrics carry a "millisecond" unit through the Agent pipeline, while plain
// counter and histogram metrics do not.
package dogstatsdunit
