// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package sample contains functions that dynamic instrumentation tests against
package sample

//nolint:all
//go:noinline
func test_single_byte(x byte) {}

//nolint:all
//go:noinline
func test_single_rune(x rune) {}

//nolint:all
//go:noinline
func test_single_bool(x bool) {}

//nolint:all
//go:noinline
func test_single_int(x int) {}

//nolint:all
//go:noinline
func test_single_int8(x int8) {}

//nolint:all
//go:noinline
func test_single_int16(x int16) {}

//nolint:all
//go:noinline
func test_single_int32(x int32) {}

//nolint:all
//go:noinline
func test_single_int64(x int64) {}

//nolint:all
//go:noinline
func test_single_uint(x uint) {}

//nolint:all
//go:noinline
func test_single_uint8(x uint8) {}

//nolint:all
//go:noinline
func test_single_uint16(x uint16) {}

//nolint:all
//go:noinline
func test_single_uint32(x uint32) {}

//nolint:all
//go:noinline
func test_single_uint64(x uint64) {}

//nolint:all
//go:noinline
func test_single_float32(x float32) {}

//nolint:all
//go:noinline
func test_single_float64(x float64) {}

type typeAlias uint16

//nolint:all
//go:noinline
func test_type_alias(x typeAlias) {}

//nolint:all
func ExecuteBasicFuncs() {
	test_single_int8(-8)
	test_single_int16(-16)
	test_single_int32(-32)
	test_single_int64(-64)

	test_single_uint8(8)
	test_single_uint16(16)
	test_single_uint32(32)
	test_single_uint64(64)
	test_single_float32(1.32)
	test_single_float64(-1.646464)

	test_single_bool(true)
	test_single_byte('a')
	test_single_rune('ă')
	test_single_int(-1512)
	test_single_uint(1512)
	test_type_alias(typeAlias(3))
}
