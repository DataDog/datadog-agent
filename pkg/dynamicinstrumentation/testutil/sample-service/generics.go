package main

type TypeWithGenerics[V comparable] struct {
	Value V
}

//go:noinline
func (x TypeWithGenerics[V]) Guess(value V) bool {
	return x.Value == value
}

func executeGenericFuncs() {
	x := TypeWithGenerics[string]{Value: "generics work"}
	x.Guess("generics work")

	y := TypeWithGenerics[int]{Value: 42}
	y.Guess(21)
}
