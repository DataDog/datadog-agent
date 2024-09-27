// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package flags contains flags that are only available on Windows.
package flags

// Win holds a set of flags which will be populated only during the Windows build.
var Win = struct {
	StartService bool
	StopService  bool
	Foreground   bool
}{}
