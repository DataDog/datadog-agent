// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package sample

type receiver struct {
	u uint
}

//nolint:all
//go:noinline
func (r *receiver) test_pointer_method_receiver(a int) {}

//nolint:all
//go:noinline
func (r receiver) test_method_receiver(a int) {}

//nolint:all
//go:noinline
func test_struct_with_array(a structWithAnArray) {}

//nolint:all
//go:noinline
func test_struct_with_a_slice(s structWithASlice) {}

//nolint:all
//go:noinline
func test_struct(x aStruct) {}

//nolint:all
//go:noinline
func test_nonembedded_struct(x nStruct) {}

//nolint:all
//go:noinline
func test_multiple_embedded_struct(b bStruct) {}

//nolint:all
//go:noinline
func test_no_string_struct(c cStruct) {}

//nolint:all
//go:noinline
func test_struct_and_byte(w byte, x aStruct) {}

//nolint:all
//go:noinline
func test_nested_pointer(x *anotherStruct) {}

//nolint:all
//go:noinline
func test_ten_strings(x tenStrings) {}

//nolint:all
//go:noinline
func test_string_struct(t threestrings) {}

//nolint:all
//go:noinline
func test_deep_struct(t deepStruct1) {}

//nolint:all
//go:noinline
func test_empty_struct(e emptyStruct) {}

//nolint:all
//go:noinline
func test_lots_of_fields(l lotsOfFields) {}

//nolint:all
func ExecuteStructFuncs() {
	ts := threestrings{"a", "bb", "ccc"}
	test_string_struct(ts)

	n := nStruct{true, 1, 2}
	test_nonembedded_struct(n)

	s := aStruct{aBool: true, aString: "one", aNumber: 2, nested: nestedStruct{anotherInt: 3, anotherString: "four"}}
	test_struct(s)

	b := bStruct{aInt16: 42, nested: s, aBool: true, aInt32: 31}
	test_multiple_embedded_struct(b)

	ns := structWithNoStrings{aUint8: 9, aBool: true}
	cs := cStruct{aInt32: 4, aUint: 1, nested: ns}
	test_no_string_struct(cs)

	test_nested_pointer(&anotherStruct{&nestedStruct{anotherInt: 42, anotherString: "xyz"}})
	test_ten_strings(tenStrings{})
	test_struct_and_byte('a', s)
	test_struct_with_array(structWithAnArray{[5]uint8{1, 2, 3, 4, 5}})
	test_struct_with_a_slice(structWithASlice{[]uint8{1, 2, 3}})

	tenStr := tenStrings{
		first:   "one",
		second:  "two",
		third:   "three",
		fourth:  "four",
		fifth:   "five",
		sixth:   "six",
		seventh: "seven",
		eighth:  "eight",
		ninth:   "nine",
		tenth:   "ten",
	}
	test_ten_strings(tenStr)

	deep := deepStruct1{
		1, deepStruct2{
			2, deepStruct3{
				3, deepStruct4{
					4, deepStruct5{
						5, deepStruct6{
							6,
						},
					},
				},
			},
		},
	}

	test_empty_struct(emptyStruct{})
	test_deep_struct(deep)

	fields := lotsOfFields{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16, 17, 18, 19, 20, 21, 22, 23, 24, 25, 26}
	test_lots_of_fields(fields)

	rcvr := receiver{1}
	rcvr.test_method_receiver(2)

	ptrRcvr := &receiver{3}
	ptrRcvr.test_pointer_method_receiver(4)

}

type emptyStruct struct{}

type deepStruct1 struct {
	a int
	b deepStruct2
}

type deepStruct2 struct {
	c int
	d deepStruct3
}

type deepStruct3 struct {
	e int
	f deepStruct4
}

type deepStruct4 struct {
	g int
	h deepStruct5
}

type deepStruct5 struct {
	i int
	j deepStruct6
}

type deepStruct6 struct {
	k int
}

type nStruct struct {
	aBool  bool
	aInt   int
	aInt16 int16
}

type aStruct struct {
	aBool   bool
	aString string
	aNumber int
	nested  nestedStruct
}

type bStruct struct {
	aInt16 int16
	nested aStruct
	aBool  bool
	aInt32 int32
}

type cStruct struct {
	aInt32 int32
	aUint  uint
	nested structWithNoStrings
}

type structWithNoStrings struct {
	aUint8 uint8
	aBool  bool
}

type structWithAnArray struct {
	arr [5]uint8
}

type structWithASlice struct {
	slice []uint8
}

type nestedStruct struct {
	anotherInt    int
	anotherString string
}

type anotherStruct struct {
	nested *nestedStruct
}

type tenStrings struct {
	first   string
	second  string
	third   string
	fourth  string
	fifth   string
	sixth   string
	seventh string
	eighth  string
	ninth   string
	tenth   string
}

type lotsOfFields struct {
	a, b, c, d, e, f, g, h, i, j, k, l, m, n, o, p, q, r, s, t, u, v, w, x, y, z uint8
}

type threestrings struct {
	a, b, c string
}
