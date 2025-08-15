// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// package main contains functions that dynamic instrumentation tests against
package main

//nolint:all
//go:noinline
func testSingleByte(x byte) {}

//nolint:all
//go:noinline
func testSingleRune(x rune) {}

//nolint:all
//go:noinline
func testSingleBool(x bool) {}

//nolint:all
//go:noinline
func testSingleInt(x int) {}

//nolint:all
//go:noinline
func testSingleInt8(x int8) {}

//nolint:all
//go:noinline
func testSingleInt16(x int16) {}

//nolint:all
//go:noinline
func testSingleInt32(x int32) {}

//nolint:all
//go:noinline
func testSingleInt64(x int64) {}

//nolint:all
//go:noinline
func testSingleUint(x uint) {}

//nolint:all
//go:noinline
func testSingleUint8(x uint8) {}

//nolint:all
//go:noinline
func testSingleUint16(x uint16) {}

//nolint:all
//go:noinline
func testSingleUint32(x uint32) {}

//nolint:all
//go:noinline
func testSingleUint64(x uint64) {}

//nolint:all
//go:noinline
func testSingleFloat32(x float32) {}

//nolint:all
//go:noinline
func testSingleFloat64(x float64) {}

type typeAlias uint16

//nolint:all
//go:noinline
func testTypeAlias(x typeAlias) {}

//nolint:all
func executeBasicFuncs() {
	testSingleInt8(-8)
	testSingleInt16(-16)
	testSingleInt32(-32)
	testSingleInt64(-64)

	testSingleUint8(8)
	testSingleUint16(16)
	testSingleUint32(32)
	testSingleUint64(64)
	testSingleFloat32(1.32)
	testSingleFloat64(-1.646464)

	testSingleBool(true)
	testSingleByte('a')
	testSingleRune(1)
	testSingleInt(-1512)
	testSingleUint(1512)
	testTypeAlias(typeAlias(3))
}
