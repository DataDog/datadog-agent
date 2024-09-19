package main

//go:noinline
func test_single_byte(x byte) {}

//go:noinline
func test_single_rune(x rune) {}

//go:noinline
func test_single_bool(x bool) {}

//go:noinline
func test_single_int(x int) {}

//go:noinline
func test_single_int8(x int8) {}

//go:noinline
func test_single_int16(x int16) {}

//go:noinline
func test_single_int32(x int32) {}

//go:noinline
func test_single_int64(x int64) {}

//go:noinline
func test_single_uint(x uint) {}

//go:noinline
func test_single_uint8(x uint8) {}

//go:noinline
func test_single_uint16(x uint16) {}

//go:noinline
func test_single_uint32(x uint32) {}

//go:noinline
func test_single_uint64(x uint64) {}

//go:noinline
func test_single_float32(x float32) {}

//go:noinline
func test_single_float64(x float64) {}

type typeAlias uint16

//go:noinline
func test_type_alias(x typeAlias) {}

func executeBasicFuncs() {
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
