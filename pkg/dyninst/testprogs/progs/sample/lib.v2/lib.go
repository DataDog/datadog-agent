// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package lib_v2 is a package in a directory with a dot in its name (package
// names do not need to correspond to directory names necessarily). This dot
// gets escaped in DWARF, making this useful for our tests.
package lib_v2

var dummy int

//go:noinline
func FooV2() {
	dummy++
}
