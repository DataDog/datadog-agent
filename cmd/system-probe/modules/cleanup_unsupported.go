// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !(linux && linux_bpf)

package modules

// PostRegisterCleanup is a function that will be called after modules are registered, it should run cleanup code
// that is common to most system probe modules
func PostRegisterCleanup() error {
	return nil
}
