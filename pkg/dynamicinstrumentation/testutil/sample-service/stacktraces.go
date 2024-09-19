package main

import "fmt"

//go:noinline
func stack_A() {
	stack_B()
}

//go:noinline
func stack_B() {
	stack_C()
}

//go:noinline
func stack_C() string {
	return fmt.Sprintf("hello %d!", 1)
}

//go:noinline
func call_inlined_func_chain() {
	inline_me_1()
}

func inline_me_1() {
	inline_me_2()
}

func inline_me_2() {
	inline_me_3()
}

func inline_me_3() {
	not_inlined()
}

//go:noinline
func not_inlined() string {
	return fmt.Sprintf("hello %d!", 42)
}

func executeStackAndInlining() {
	stack_A()
	call_inlined_func_chain()
}
