// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build windows

package tags

// ResolveRuntimeArch determines the architecture of the lambda at runtime
func ResolveRuntimeArch() string {
	return X86LambdaPlatform
}
