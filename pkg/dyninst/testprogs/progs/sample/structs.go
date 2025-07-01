// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package main

type receiver struct {
	u uint
}

type hasUnsupportedFields struct {
	b int
	c float32
	d []uint8
}

//nolint:all
//go:noinline
func testStructWithUnsupportedFields(a hasUnsupportedFields) {}

//nolint:all
//go:noinline
func (r *receiver) testPointerMethodReceiver(a int) {}

//nolint:all
//go:noinline
func (r receiver) testMethodReceiver(a int) {}

//nolint:all
//go:noinline
func testStructWithArray(a structWithAnArray) {}

//nolint:all
//go:noinline
func testStructWithASlice(s structWithASlice) {}

//nolint:all
//go:noinline
func testStructWithAnEmptySlice(s structWithASlice) {}

//nolint:all
//go:noinline
func testStructWithANilSlice(s structWithASlice) {}

//nolint:all
//go:noinline
func testPointerToStructWithASlice(s *structWithASlice) {}

//nolint:all
//go:noinline
func testPointerToStructWithAString(s *structWithAString) {}

//nolint:all
//go:noinline
func testStruct(x aStruct) {}

//nolint:all
//go:noinline
func testStructWithArrays(s structWithTwoArrays) {}

//nolint:all
//go:noinline
func testNonembeddedStruct(x nStruct) {}

//nolint:all
//go:noinline
func testMultipleEmbeddedStruct(b bStruct) {}

//nolint:all
//go:noinline
func testNoStringStruct(c cStruct) {}

//nolint:all
//go:noinline
func testStructAndByte(w byte, x aStruct) {}

//nolint:all
//go:noinline
func testNestedPointer(x *anotherStruct) {}

//nolint:all
//go:noinline
func testTenStrings(x tenStrings) {}

//nolint:all
//go:noinline
func testStringStruct(t threestrings) {}

//nolint:all
//go:noinline
func testDeepStruct(t deepStruct1) {}

//nolint:all
//go:noinline
func testEmptyStruct(e emptyStruct) {}

//nolint:all
//go:noinline
func testLotsOfFields(l lotsOfFields) {}

//nolint:all
func executeStructFuncs() {
	ts := threestrings{"a", "bb", "ccc"}
	testStringStruct(ts)

	n := nStruct{true, 1, 2}
	testNonembeddedStruct(n)

	s := aStruct{aBool: true, aString: "one", aNumber: 2, nested: nestedStruct{anotherInt: 3, anotherString: "four"}}
	testStruct(s)

	b := bStruct{aInt16: 42, nested: s, aBool: true, aInt32: 31}
	testMultipleEmbeddedStruct(b)

	ns := structWithNoStrings{aUint8: 9, aBool: true}
	cs := cStruct{aInt32: 4, aUint: 1, nested: ns}
	testNoStringStruct(cs)

	testNestedPointer(&anotherStruct{&nestedStruct{anotherInt: 42, anotherString: "xyz"}})
	testTenStrings(tenStrings{})
	testStructAndByte('a', s)
	testStructWithArray(structWithAnArray{[5]uint8{1, 2, 3, 4, 5}})
	testStructWithASlice(structWithASlice{1, []uint8{2, 3, 4}, 5})
	testStructWithAnEmptySlice(structWithASlice{9, []uint8{}, 5})
	testStructWithANilSlice(structWithASlice{9, nil, 5})
	testPointerToStructWithASlice(&structWithASlice{5, []uint8{2, 3, 4}, 5})
	testPointerToStructWithAString(&structWithAString{5, "abcdef"})

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
	testTenStrings(tenStr)

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

	testEmptyStruct(emptyStruct{})
	testDeepStruct(deep)

	fields := lotsOfFields{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16, 17, 18, 19, 20, 21, 22, 23, 24, 25, 26}
	testLotsOfFields(fields)

	rcvr := receiver{1}
	rcvr.testMethodReceiver(2)

	ptrRcvr := &receiver{3}
	ptrRcvr.testPointerMethodReceiver(4)

	sta := structWithTwoArrays{
		a: [3]uint64{1, 2, 3},
		b: 4,
		c: [5]int64{6, 7, 8, 9, 10},
	}
	testStructWithArrays(sta)

	testStructWithUnsupportedFields(hasUnsupportedFields{
		b: 1,
		c: 2.0,
		d: []uint8{3, 4, 5},
	})
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

type structWithTwoArrays struct {
	a [3]uint64
	b byte
	c [5]int64
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
	x     int
	slice []uint8
	z     uint64
}

type structWithAString struct {
	x int
	s string
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
