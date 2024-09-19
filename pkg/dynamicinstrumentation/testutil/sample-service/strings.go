package main

//go:noinline
func test_single_string(x string) {}

func executeStringFuncs() {
	test_single_string("abc")
}
