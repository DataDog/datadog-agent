// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package sample

//nolint:all
//go:noinline
func test_single_string(x string) {}

//nolint:all
func ExecuteStringFuncs() {
	test_single_string("abc")
}
