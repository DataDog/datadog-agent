// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !jmx

//nolint:revive // TODO(APL) Fix revive linter
package jmx

// StopJmxfetch does nothing when the agent does not ship jmx
func StopJmxfetch() {

}
